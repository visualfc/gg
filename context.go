// Package gg provides a simple API for rendering 2D graphics in pure Go.
package gg

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"math"
	"strings"

	"github.com/goki/freetype/truetype"
	"github.com/golang/freetype/raster"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
)

type LineCap int

const (
	LineCapRound LineCap = iota
	LineCapButt
	LineCapSquare
)

type LineJoin int

const (
	LineJoinRound LineJoin = iota
	LineJoinBevel
)

type FillRule int

const (
	FillRuleWinding FillRule = iota
	FillRuleEvenOdd
)

type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

var (
	defaultFillStyle   = NewSolidPattern(color.White)
	defaultStrokeStyle = NewSolidPattern(color.Black)
)

type Context struct {
	width         int
	height        int
	rasterizer    *raster.Rasterizer
	im            *image.RGBA
	mask          *image.Alpha
	clipRect      image.Rectangle
	transformer   draw.Transformer
	color         color.Color
	fillPattern   Pattern
	strokePattern Pattern
	strokePath    raster.Path
	fillPath      raster.Path
	start         Point
	current       Point
	hasCurrent    bool
	dashes        []float64
	lineWidth     float64
	lineCap       LineCap
	lineJoin      LineJoin
	fillRule      FillRule
	fontFace      font.Face
	fontHeight    float64
	dpi           float64
	fontSize      float64
	fontScale     float64
	matrix        Matrix
	font          *truetype.Font
	glyphBuf      *truetype.GlyphBuf
	stack         []*Context
}

// NewContext creates a new image.RGBA with the specified width and height
// and prepares a context for rendering onto that image.
func NewContext(width, height int) *Context {
	return NewContextForRGBA(image.NewRGBA(image.Rect(0, 0, width, height)))
}

// NewContextForImage copies the specified image into a new image.RGBA
// and prepares a context for rendering onto that image.
func NewContextForImage(im image.Image) *Context {
	return NewContextForRGBA(imageToRGBA(im))
}

// NewContextForRGBA prepares a context for rendering onto the specified image.
// No copy is made.
func NewContextForRGBA(im *image.RGBA) *Context {
	w := im.Bounds().Size().X
	h := im.Bounds().Size().Y
	return &Context{
		width:         w,
		height:        h,
		rasterizer:    raster.NewRasterizer(w, h),
		im:            im,
		color:         color.Transparent,
		transformer:   draw.ApproxBiLinear,
		fillPattern:   defaultFillStyle,
		strokePattern: defaultStrokeStyle,
		lineWidth:     1,
		fillRule:      FillRuleWinding,
		fontFace:      basicfont.Face7x13,
		fontHeight:    13,
		dpi:           72,
		fontSize:      13 * 92 / 72,
		fontScale:     13 * 72 * 64 / 72,
		matrix:        Identity(),
		glyphBuf:      &truetype.GlyphBuf{},
	}
}

// GetCurrentPoint will return the current point and if there is a current point.
// The point will have been transformed by the context's transformation matrix.
func (dc *Context) GetCurrentPoint() (Point, bool) {
	if dc.hasCurrent {
		return dc.current, true
	}
	return Point{}, false
}

// Image returns the image that has been drawn by this context.
func (dc *Context) Image() image.Image {
	return dc.im
}

func (dc *Context) SubImage(r image.Rectangle) image.Image {
	return dc.im.SubImage(r)
}

func (dc *Context) CloneSubImage(r image.Rectangle) image.Image {
	r = r.Intersect(dc.im.Rect)
	// If r1 and r2 are Rectangles, r1.Intersect(r2) is not guaranteed to be inside
	// either r1 or r2 if the intersection is empty. Without explicitly checking for
	// this, the Pix[i:] expression below can panic.
	if r.Empty() {
		return &image.RGBA{}
	}
	i := dc.im.PixOffset(r.Min.X, r.Min.Y)
	pix := dc.im.Pix[i:]
	dst := make([]uint8, len(pix), len(pix))
	copy(dst, pix)
	return &image.RGBA{
		Pix:    dst,
		Stride: dc.im.Stride,
		Rect:   r,
	}
}

func (dc *Context) SetTranformer(transformer draw.Transformer) {
	dc.transformer = transformer
}

// Width returns the width of the image in pixels.
func (dc *Context) Width() int {
	return dc.width
}

// Height returns the height of the image in pixels.
func (dc *Context) Height() int {
	return dc.height
}

// SavePNG encodes the image as a PNG and writes it to disk.
func (dc *Context) SavePNG(path string) error {
	return SavePNG(path, dc.im)
}

// EncodePNG encodes the image as a PNG and writes it to the provided io.Writer.
func (dc *Context) EncodePNG(w io.Writer) error {
	return png.Encode(w, dc.im)
}

// SetDash sets the current dash pattern to use. Call with zero arguments to
// disable dashes. The values specify the lengths of each dash, with
// alternating on and off lengths.
func (dc *Context) SetDash(dashes ...float64) {
	dc.dashes = dashes
}

func (dc *Context) SetLineWidth(lineWidth float64) {
	dc.lineWidth = lineWidth
}

func (dc *Context) SetLineCap(lineCap LineCap) {
	dc.lineCap = lineCap
}

func (dc *Context) SetLineCapRound() {
	dc.lineCap = LineCapRound
}

func (dc *Context) SetLineCapButt() {
	dc.lineCap = LineCapButt
}

func (dc *Context) SetLineCapSquare() {
	dc.lineCap = LineCapSquare
}

func (dc *Context) SetLineJoin(lineJoin LineJoin) {
	dc.lineJoin = lineJoin
}

func (dc *Context) SetLineJoinRound() {
	dc.lineJoin = LineJoinRound
}

func (dc *Context) SetLineJoinBevel() {
	dc.lineJoin = LineJoinBevel
}

func (dc *Context) SetFillRule(fillRule FillRule) {
	dc.fillRule = fillRule
}

func (dc *Context) SetFillRuleWinding() {
	dc.fillRule = FillRuleWinding
}

func (dc *Context) SetFillRuleEvenOdd() {
	dc.fillRule = FillRuleEvenOdd
}

// Color Setters

func (dc *Context) setFillAndStrokeColor(c color.Color) {
	dc.color = c
	dc.fillPattern = NewSolidPattern(c)
	dc.strokePattern = NewSolidPattern(c)
}

// SetFillStyle sets current fill style
func (dc *Context) SetFillStyle(pattern Pattern) {
	// if pattern is SolidPattern, also change dc.color(for dc.Clear, dc.drawString)
	if fillStyle, ok := pattern.(*solidPattern); ok {
		dc.color = fillStyle.color
	}
	dc.fillPattern = pattern
}

// SetStrokeStyle sets current stroke style
func (dc *Context) SetStrokeStyle(pattern Pattern) {
	dc.strokePattern = pattern
}

// SetColor sets the current color(for both fill and stroke).
func (dc *Context) SetColor(c color.Color) {
	dc.setFillAndStrokeColor(c)
}

func (dc *Context) SetFillColor(c color.Color) {
	dc.fillPattern = NewSolidPattern(c)
}

func (dc *Context) SetStrokeColor(c color.Color) {
	dc.strokePattern = NewSolidPattern(c)
}

// SetHexColor sets the current color using a hex string. The leading pound
// sign (#) is optional. Both 3- and 6-digit variations are supported. 8 digits
// may be provided to set the alpha value as well.
func (dc *Context) SetHexColor(x string) {
	r, g, b, a := parseHexColor(x)
	dc.SetRGBA255(r, g, b, a)
}

// SetRGBA255 sets the current color. r, g, b, a values should be between 0 and
// 255, inclusive.
func (dc *Context) SetRGBA255(r, g, b, a int) {
	dc.color = color.NRGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
	dc.setFillAndStrokeColor(dc.color)
}

// SetRGB255 sets the current color. r, g, b values should be between 0 and 255,
// inclusive. Alpha will be set to 255 (fully opaque).
func (dc *Context) SetRGB255(r, g, b int) {
	dc.SetRGBA255(r, g, b, 255)
}

// SetRGBA sets the current color. r, g, b, a values should be between 0 and 1,
// inclusive.
func (dc *Context) SetRGBA(r, g, b, a float64) {
	dc.color = color.NRGBA{
		uint8(r * 255),
		uint8(g * 255),
		uint8(b * 255),
		uint8(a * 255),
	}
	dc.setFillAndStrokeColor(dc.color)
}

// SetRGB sets the current color. r, g, b values should be between 0 and 1,
// inclusive. Alpha will be set to 1 (fully opaque).
func (dc *Context) SetRGB(r, g, b float64) {
	dc.SetRGBA(r, g, b, 1)
}

// Path Manipulation

// MoveTo starts a new subpath within the current path starting at the
// specified point.
func (dc *Context) MoveTo(x, y float64) {
	if dc.hasCurrent {
		dc.fillPath.Add1(dc.start.Fixed())
	}
	x, y = dc.TransformPoint(x, y)
	p := Point{x, y}
	dc.strokePath.Start(p.Fixed())
	dc.fillPath.Start(p.Fixed())
	dc.start = p
	dc.current = p
	dc.hasCurrent = true
}

// LineTo adds a line segment to the current path starting at the current
// point. If there is no current point, it is equivalent to MoveTo(x, y)
func (dc *Context) LineTo(x, y float64) {
	if !dc.hasCurrent {
		dc.MoveTo(x, y)
	} else {
		x, y = dc.TransformPoint(x, y)
		p := Point{x, y}
		dc.strokePath.Add1(p.Fixed())
		dc.fillPath.Add1(p.Fixed())
		dc.current = p
	}
}

// QuadraticTo adds a quadratic bezier curve to the current path starting at
// the current point. If there is no current point, it first performs
// MoveTo(x1, y1)
func (dc *Context) QuadraticTo(x1, y1, x2, y2 float64) {
	if !dc.hasCurrent {
		dc.MoveTo(x1, y1)
	}
	x1, y1 = dc.TransformPoint(x1, y1)
	x2, y2 = dc.TransformPoint(x2, y2)
	p1 := Point{x1, y1}
	p2 := Point{x2, y2}
	dc.strokePath.Add2(p1.Fixed(), p2.Fixed())
	dc.fillPath.Add2(p1.Fixed(), p2.Fixed())
	dc.current = p2
}

// CubicTo adds a cubic bezier curve to the current path starting at the
// current point. If there is no current point, it first performs
// MoveTo(x1, y1). Because freetype/raster does not support cubic beziers,
// this is emulated with many small line segments.
func (dc *Context) CubicTo(x1, y1, x2, y2, x3, y3 float64) {
	if !dc.hasCurrent {
		dc.MoveTo(x1, y1)
	}
	x0, y0 := dc.current.X, dc.current.Y
	x1, y1 = dc.TransformPoint(x1, y1)
	x2, y2 = dc.TransformPoint(x2, y2)
	x3, y3 = dc.TransformPoint(x3, y3)
	points := CubicBezier(x0, y0, x1, y1, x2, y2, x3, y3)
	previous := dc.current.Fixed()
	for _, p := range points[1:] {
		f := p.Fixed()
		if f == previous {
			// TODO: this fixes some rendering issues but not all
			continue
		}
		previous = f
		dc.strokePath.Add1(f)
		dc.fillPath.Add1(f)
		dc.current = p
	}
}

// ClosePath adds a line segment from the current point to the beginning
// of the current subpath. If there is no current point, this is a no-op.
func (dc *Context) ClosePath() {
	if dc.hasCurrent {
		dc.strokePath.Add1(dc.start.Fixed())
		dc.fillPath.Add1(dc.start.Fixed())
		dc.current = dc.start
	}
}

// ClearPath clears the current path. There is no current point after this
// operation.
func (dc *Context) ClearPath() {
	dc.strokePath.Clear()
	dc.fillPath.Clear()
	dc.hasCurrent = false
}

// NewSubPath starts a new subpath within the current path. There is no current
// point after this operation.
func (dc *Context) NewSubPath() {
	if dc.hasCurrent {
		dc.fillPath.Add1(dc.start.Fixed())
	}
	dc.hasCurrent = false
}

// Path Drawing

func (dc *Context) capper() raster.Capper {
	switch dc.lineCap {
	case LineCapButt:
		return raster.ButtCapper
	case LineCapRound:
		return raster.RoundCapper
	case LineCapSquare:
		return raster.SquareCapper
	}
	return nil
}

func (dc *Context) joiner() raster.Joiner {
	switch dc.lineJoin {
	case LineJoinBevel:
		return raster.BevelJoiner
	case LineJoinRound:
		return raster.RoundJoiner
	}
	return nil
}

func (dc *Context) stroke(painter raster.Painter) {
	path := dc.strokePath
	if len(dc.dashes) > 0 {
		path = dashed(path, dc.dashes)
	} else {
		// TODO: this is a temporary workaround to remove tiny segments
		// that result in rendering issues
		path = rasterPath(flattenPath(path))
	}
	r := dc.rasterizer
	r.UseNonZeroWinding = true
	r.Clear()
	r.AddStroke(path, fix(dc.lineWidth), dc.capper(), dc.joiner())
	r.Rasterize(painter)
}

func (dc *Context) fill(painter raster.Painter) {
	path := dc.fillPath
	if dc.hasCurrent {
		path = make(raster.Path, len(dc.fillPath))
		copy(path, dc.fillPath)
		path.Add1(dc.start.Fixed())
	}
	r := dc.rasterizer
	r.UseNonZeroWinding = dc.fillRule == FillRuleWinding
	r.Clear()
	r.AddPath(path)
	r.Rasterize(painter)
}

// StrokePreserve strokes the current path with the current color, line width,
// line cap, line join and dash settings. The path is preserved after this
// operation.
func (dc *Context) StrokePreserve() {
	var painter raster.Painter
	if dc.mask == nil {
		if pattern, ok := dc.strokePattern.(*solidPattern); ok {
			// with a nil mask and a solid color pattern, we can be more efficient
			// TODO: refactor so we don't have to do this type assertion stuff?
			p := raster.NewRGBAPainter(dc.im)
			p.SetColor(pattern.color)
			painter = p
		}
	}
	if painter == nil {
		painter = newPatternPainter(dc.im, dc.mask, dc.strokePattern, dc.matrix)
	}
	dc.stroke(painter)
}

// Stroke strokes the current path with the current color, line width,
// line cap, line join and dash settings. The path is cleared after this
// operation.
func (dc *Context) Stroke() {
	dc.StrokePreserve()
	dc.ClearPath()
}

func (dc *Context) FillStroke() {
	dc.FillPreserve()
	dc.Stroke()
}

// FillPreserve fills the current path with the current color. Open subpaths
// are implicity closed. The path is preserved after this operation.
func (dc *Context) FillPreserve() {
	var painter raster.Painter
	if dc.mask == nil {
		if pattern, ok := dc.fillPattern.(*solidPattern); ok {
			// with a nil mask and a solid color pattern, we can be more efficient
			// TODO: refactor so we don't have to do this type assertion stuff?
			p := raster.NewRGBAPainter(dc.im)
			p.SetColor(pattern.color)
			painter = p
		}
	}
	if painter == nil {
		painter = newPatternPainter(dc.im, dc.mask, dc.fillPattern, dc.matrix)
	}
	dc.fill(painter)
}

// Fill fills the current path with the current color. Open subpaths
// are implicity closed. The path is cleared after this operation.
func (dc *Context) Fill() {
	dc.FillPreserve()
	dc.ClearPath()
}

type AlphaOverPainter struct {
	Image *image.Alpha
	rect  image.Rectangle
}

// Paint satisfies the Painter interface.
func (r *AlphaOverPainter) Paint(ss []raster.Span, done bool) {
	b := r.Image.Bounds()
	for _, s := range ss {
		if s.Y < b.Min.Y {
			continue
		}
		if s.Y >= b.Max.Y {
			return
		}
		if s.X0 < b.Min.X {
			s.X0 = b.Min.X
		}
		if s.X1 > b.Max.X {
			s.X1 = b.Max.X
		}
		if s.X0 >= s.X1 {
			continue
		}
		if r.rect.Min.X > s.X0 {
			r.rect.Min.X = s.X0
		}
		if r.rect.Min.Y > s.Y {
			r.rect.Min.Y = s.Y
		}
		if r.rect.Max.Y < s.Y {
			r.rect.Max.Y = s.Y
		}
		if r.rect.Max.X < s.X1 {
			r.rect.Max.X = s.X1
		}

		base := (s.Y-r.Image.Rect.Min.Y)*r.Image.Stride - r.Image.Rect.Min.X
		p := r.Image.Pix[base+s.X0 : base+s.X1]
		a := int(s.Alpha >> 8)
		for i, c := range p {
			v := int(c)
			p[i] = uint8((v*255 + (255-v)*a) / 255)
		}
	}
}

// NewAlphaOverPainter creates a new AlphaOverPainter for the given image.
func NewAlphaOverPainter(m *image.Alpha) *AlphaOverPainter {
	return &AlphaOverPainter{m, image.Rectangle{image.Point{1e9, 1e9}, image.Point{-1e9, -1e9}}}
}

// ClipPreserve updates the clipping region by intersecting the current
// clipping region with the current path as it would be filled by dc.Fill().
// The path is preserved after this operation.
func (dc *Context) ClipPreserve() {
	clip := image.NewAlpha(image.Rect(0, 0, dc.width, dc.height))
	painter := NewAlphaOverPainter(clip)
	dc.fill(painter)
	if dc.mask == nil {
		dc.mask = clip
	} else {
		mask := image.NewAlpha(image.Rect(0, 0, dc.width, dc.height))
		draw.DrawMask(mask, mask.Bounds(), clip, image.ZP, dc.mask, image.ZP, draw.Over)
		dc.mask = mask
	}
	dc.clipRect = painter.rect
}

// SetMask allows you to directly set the *image.Alpha to be used as a clipping
// mask. It must be the same size as the context, else an error is returned
// and the mask is unchanged.
func (dc *Context) SetMask(mask *image.Alpha) error {
	if mask.Bounds().Size() != dc.im.Bounds().Size() {
		return errors.New("mask size must match context size")
	}
	dc.mask = mask
	return nil
}

// AsMask returns an *image.Alpha representing the alpha channel of this
// context. This can be useful for advanced clipping operations where you first
// render the mask geometry and then use it as a mask.
func (dc *Context) AsMask() *image.Alpha {
	mask := image.NewAlpha(dc.im.Bounds())
	draw.Draw(mask, dc.im.Bounds(), dc.im, image.ZP, draw.Src)
	return mask
}

// InvertMask inverts the alpha values in the current clipping mask such that
// a fully transparent region becomes fully opaque and vice versa.
func (dc *Context) InvertMask() {
	if dc.mask == nil {
		dc.mask = image.NewAlpha(dc.im.Bounds())
	} else {
		for i, a := range dc.mask.Pix {
			dc.mask.Pix[i] = 255 - a
		}
	}
}

// Clip updates the clipping region by intersecting the current
// clipping region with the current path as it would be filled by dc.Fill().
// The path is cleared after this operation.
func (dc *Context) Clip() {
	dc.ClipPreserve()
	dc.ClearPath()
}

func (dc *Context) ClipRect() image.Rectangle {
	return dc.clipRect
}

// ResetClip clears the clipping region.
func (dc *Context) ResetClip() {
	dc.mask = nil
	dc.clipRect = image.Rect(0, 0, 0, 0)
}

// Convenient Drawing Functions

// Clear fills the entire image with the current color.
func (dc *Context) Clear() {
	src := image.NewUniform(dc.color)
	draw.Draw(dc.im, dc.im.Bounds(), src, image.ZP, draw.Src)
}

// SetPixel sets the color of the specified pixel using the current color.
func (dc *Context) SetPixel(x, y int) {
	dc.im.Set(x, y, dc.color)
}

// DrawPoint is like DrawCircle but ensures that a circle of the specified
// size is drawn regardless of the current transformation matrix. The position
// is still transformed, but not the shape of the point.
func (dc *Context) DrawPoint(x, y, r float64) {
	dc.Push()
	tx, ty := dc.TransformPoint(x, y)
	dc.Identity()
	dc.DrawCircle(tx, ty, r)
	dc.Pop()
}

func (dc *Context) DrawLine(x1, y1, x2, y2 float64) {
	dc.MoveTo(x1, y1)
	dc.LineTo(x2, y2)
}

func (dc *Context) DrawRectangle(x, y, w, h float64) {
	dc.NewSubPath()
	dc.MoveTo(x, y)
	dc.LineTo(x+w, y)
	dc.LineTo(x+w, y+h)
	dc.LineTo(x, y+h)
	dc.ClosePath()
}

func (dc *Context) DrawRoundedRectangle(x, y, w, h, r float64) {
	x0, x1, x2, x3 := x, x+r, x+w-r, x+w
	y0, y1, y2, y3 := y, y+r, y+h-r, y+h
	dc.NewSubPath()
	dc.MoveTo(x1, y0)
	dc.LineTo(x2, y0)
	dc.DrawArc(x2, y1, r, Radians(270), Radians(360))
	dc.LineTo(x3, y2)
	dc.DrawArc(x2, y2, r, Radians(0), Radians(90))
	dc.LineTo(x1, y3)
	dc.DrawArc(x1, y2, r, Radians(90), Radians(180))
	dc.LineTo(x0, y1)
	dc.DrawArc(x1, y1, r, Radians(180), Radians(270))
	dc.ClosePath()
}

func (dc *Context) DrawEllipticalArc(x, y, rx, ry, angle1, angle2 float64) {
	const n = 16
	for i := 0; i < n; i++ {
		p1 := float64(i+0) / n
		p2 := float64(i+1) / n
		a1 := angle1 + (angle2-angle1)*p1
		a2 := angle1 + (angle2-angle1)*p2
		x0 := x + rx*math.Cos(a1)
		y0 := y + ry*math.Sin(a1)
		x1 := x + rx*math.Cos(a1+(a2-a1)/2)
		y1 := y + ry*math.Sin(a1+(a2-a1)/2)
		x2 := x + rx*math.Cos(a2)
		y2 := y + ry*math.Sin(a2)
		cx := 2*x1 - x0/2 - x2/2
		cy := 2*y1 - y0/2 - y2/2
		if i == 0 {
			if dc.hasCurrent {
				dc.LineTo(x0, y0)
			} else {
				dc.MoveTo(x0, y0)
			}
		}
		dc.QuadraticTo(cx, cy, x2, y2)
	}
}

func (dc *Context) DrawEllipse(x, y, rx, ry float64) {
	dc.NewSubPath()
	dc.DrawEllipticalArc(x, y, rx, ry, 0, 2*math.Pi)
	dc.ClosePath()
}

func (dc *Context) DrawArc(x, y, r, angle1, angle2 float64) {
	dc.DrawEllipticalArc(x, y, r, r, angle1, angle2)
}

func (dc *Context) DrawCircle(x, y, r float64) {
	dc.NewSubPath()
	dc.DrawEllipticalArc(x, y, r, r, 0, 2*math.Pi)
	dc.ClosePath()
}

func (dc *Context) DrawRegularPolygon(n int, x, y, r, rotation float64) {
	angle := 2 * math.Pi / float64(n)
	rotation -= math.Pi / 2
	if n%2 == 0 {
		rotation += angle / 2
	}
	dc.NewSubPath()
	for i := 0; i < n; i++ {
		a := rotation + angle*float64(i)
		dc.LineTo(x+r*math.Cos(a), y+r*math.Sin(a))
	}
	dc.ClosePath()
}

// DrawImage draws the specified image at the specified point.
func (dc *Context) DrawImage(im image.Image, x, y int) {
	dc.DrawImageAnchored(im, x, y, 0, 0)
}

// DrawImageAnchored draws the specified image at the specified anchor point.
// The anchor point is x - w * ax, y - h * ay, where w, h is the size of the
// image. Use ax=0.5, ay=0.5 to center the image at the specified point.
func (dc *Context) DrawImageAnchored(im image.Image, x, y int, ax, ay float64) {
	s := im.Bounds().Size()
	x -= int(ax * float64(s.X))
	y -= int(ay * float64(s.Y))
	fx, fy := float64(x), float64(y)
	m := dc.matrix.Translate(fx, fy)
	s2d := f64.Aff3{m.XX, m.XY, m.X0, m.YX, m.YY, m.Y0}
	if dc.mask == nil {
		dc.transformer.Transform(dc.im, s2d, im, im.Bounds(), draw.Over, nil)
	} else {
		dc.transformer.Transform(dc.im, s2d, im, im.Bounds(), draw.Over, &draw.Options{
			DstMask:  dc.mask,
			DstMaskP: image.ZP,
		})
	}
}

// Font

func (dc *Context) SetFont(font *truetype.Font) {
	dc.font = font
}

func (dc *Context) LoadFont(path string) error {
	fontBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	f, err := truetype.Parse(fontBytes)
	if err != nil {
		return err
	}
	dc.font = f
	return nil
}

func (dc *Context) LoadFontData(ttf []byte) error {
	f, err := truetype.Parse(ttf)
	if err != nil {
		return err
	}
	dc.font = f
	return nil
}

func (dc *Context) SetFontSize(points float64) {
	if dc.font == nil {
		log.Println("must load font")
		return
	}
	dc.fontFace = truetype.NewFace(dc.font, &truetype.Options{
		Size: points,
		// Hinting: font.HintingFull,
	})
	dc.fontHeight = float64(dc.fontFace.Metrics().Height) / 64
	dc.fontSize = points
	dc.fontScale = dc.fontSize * dc.dpi * 64 / 72
}

// Text Functions

func (dc *Context) SetFontFace(fontFace font.Face) {
	dc.fontFace = fontFace
	dc.fontHeight = float64(fontFace.Metrics().Height) / 64
	dc.fontSize = dc.fontHeight * 96 / 72
	dc.fontScale = dc.fontSize * dc.dpi * 64 / 72
}

func (dc *Context) LoadFontFace(path string, points float64) error {
	face, err := LoadFontFace(path, points)
	if err == nil {
		dc.fontFace = face
		dc.fontHeight = points * 72 / 96
		dc.fontSize = points
		dc.fontScale = dc.fontSize * dc.dpi * 64 / 72
	}
	return err
}

func (dc *Context) FontHeight() float64 {
	return dc.fontHeight
}

func (dc *Context) drawString(im *image.RGBA, s string, x, y float64) {
	d := &font.Drawer{
		Dst:  im,
		Src:  image.NewUniform(dc.color),
		Face: dc.fontFace,
		Dot:  fixp(x, y),
	}
	// based on Drawer.DrawString() in golang.org/x/image/font/font.go
	prevC := rune(-1)
	for _, c := range s {
		if prevC >= 0 {
			d.Dot.X += d.Face.Kern(prevC, c)
		}
		dr, mask, maskp, advance, ok := d.Face.Glyph(d.Dot, c)
		if !ok {
			// TODO: is falling back on the U+FFFD glyph the responsibility of
			// the Drawer or the Face?
			// TODO: set prevC = '\ufffd'?
			continue
		}
		sr := dr.Sub(dr.Min)
		transformer := draw.BiLinear
		fx, fy := float64(dr.Min.X), float64(dr.Min.Y)
		m := dc.matrix.Translate(fx, fy)
		s2d := f64.Aff3{m.XX, m.XY, m.X0, m.YX, m.YY, m.Y0}
		transformer.Transform(d.Dst, s2d, d.Src, sr, draw.Over, &draw.Options{
			SrcMask:  mask,
			SrcMaskP: maskp,
		})
		d.Dot.X += advance
		prevC = c
	}
}

// DrawString draws the specified text at the specified point.
func (dc *Context) DrawString(s string, x, y float64) {
	dc.DrawStringAnchored(s, x, y, 0, 0)
}

// DrawStringAnchored draws the specified text at the specified anchor point.
// The anchor point is x - w * ax, y - h * ay, where w, h is the size of the
// text. Use ax=0.5, ay=0.5 to center the text at the specified point.
func (dc *Context) DrawStringAnchored(s string, x, y, ax, ay float64) {
	w, h := dc.MeasureString(s)
	x -= ax * w
	y += ay * h
	if dc.mask == nil {
		dc.drawString(dc.im, s, x, y)
	} else {
		im := image.NewRGBA(image.Rect(0, 0, dc.width, dc.height))
		dc.drawString(im, s, x, y)
		draw.DrawMask(dc.im, dc.im.Bounds(), im, image.ZP, dc.mask, image.ZP, draw.Over)
	}
}

// DrawStringWrapped word-wraps the specified string to the given max width
// and then draws it at the specified anchor point using the given line
// spacing and text alignment.
func (dc *Context) DrawStringWrapped(s string, x, y, ax, ay, width, lineSpacing float64, align Align) {
	lines := dc.WordWrap(s, width)

	// sync h formula with MeasureMultilineString
	h := float64(len(lines)) * dc.fontHeight * lineSpacing
	h -= (lineSpacing - 1) * dc.fontHeight

	x -= ax * width
	y -= ay * h
	switch align {
	case AlignLeft:
		ax = 0
	case AlignCenter:
		ax = 0.5
		x += width / 2
	case AlignRight:
		ax = 1
		x += width
	}
	ay = 1
	for _, line := range lines {
		dc.DrawStringAnchored(line, x, y, ax, ay)
		y += dc.fontHeight * lineSpacing
	}
}

func (dc *Context) MeasureMultilineString(s string, lineSpacing float64) (width, height float64) {
	lines := strings.Split(s, "\n")

	// sync h formula with DrawStringWrapped
	height = float64(len(lines)) * dc.fontHeight * lineSpacing
	height -= (lineSpacing - 1) * dc.fontHeight

	d := &font.Drawer{
		Face: dc.fontFace,
	}

	// max width from lines
	for _, line := range lines {
		adv := d.MeasureString(line)
		currentWidth := float64(adv >> 6) // from gg.Context.MeasureString
		if currentWidth > width {
			width = currentWidth
		}
	}

	return width, height
}

// MeasureString returns the rendered width and height of the specified text
// given the current font face.
func (dc *Context) MeasureString(s string) (w, h float64) {
	d := &font.Drawer{
		Face: dc.fontFace,
	}
	a := d.MeasureString(s)
	return float64(a >> 6), dc.fontHeight
}

// WordWrap wraps the specified string to the given max width and current
// font face.
func (dc *Context) WordWrap(s string, w float64) []string {
	return wordWrap(dc, s, w)
}

// Transformation Matrix Operations

func (dc *Context) GetMatrix() Matrix {
	return dc.matrix
}

func (dc *Context) SetMatrix(m Matrix) {
	dc.matrix = m
}

// Identity resets the current transformation matrix to the identity matrix.
// This results in no translating, scaling, rotating, or shearing.
func (dc *Context) Identity() {
	dc.matrix = Identity()
}

// Translate updates the current matrix with a translation.
func (dc *Context) Translate(x, y float64) {
	dc.matrix = dc.matrix.Translate(x, y)
}

// Scale updates the current matrix with a scaling factor.
// Scaling occurs about the origin.
func (dc *Context) Scale(x, y float64) {
	dc.matrix = dc.matrix.Scale(x, y)
}

// ScaleAbout updates the current matrix with a scaling factor.
// Scaling occurs about the specified point.
func (dc *Context) ScaleAbout(sx, sy, x, y float64) {
	dc.Translate(x, y)
	dc.Scale(sx, sy)
	dc.Translate(-x, -y)
}

// Rotate updates the current matrix with a clockwise rotation.
// Rotation occurs about the origin. Angle is specified in radians.
func (dc *Context) Rotate(angle float64) {
	dc.matrix = dc.matrix.Rotate(angle)
}

// RotateAbout updates the current matrix with a clockwise rotation.
// Rotation occurs about the specified point. Angle is specified in radians.
func (dc *Context) RotateAbout(angle, x, y float64) {
	dc.Translate(x, y)
	dc.Rotate(angle)
	dc.Translate(-x, -y)
}

// Shear updates the current matrix with a shearing angle.
// Shearing occurs about the origin.
func (dc *Context) Shear(x, y float64) {
	dc.matrix = dc.matrix.Shear(x, y)
}

// ShearAbout updates the current matrix with a shearing angle.
// Shearing occurs about the specified point.
func (dc *Context) ShearAbout(sx, sy, x, y float64) {
	dc.Translate(x, y)
	dc.Shear(sx, sy)
	dc.Translate(-x, -y)
}

// TransformPoint multiplies the specified point by the current matrix,
// returning a transformed position.
func (dc *Context) TransformPoint(x, y float64) (tx, ty float64) {
	return dc.matrix.TransformPoint(x, y)
}

// InvertY flips the Y axis so that Y grows from bottom to top and Y=0 is at
// the bottom of the image.
func (dc *Context) InvertY() {
	dc.Translate(0, float64(dc.height))
	dc.Scale(1, -1)
}

// Stack

// Push saves the current state of the context for later retrieval. These
// can be nested.
func (dc *Context) Push() {
	x := *dc
	dc.stack = append(dc.stack, &x)
}

// Pop restores the last saved context state from the stack.
func (dc *Context) Pop() {
	before := *dc
	s := dc.stack
	x, s := s[len(s)-1], s[:len(s)-1]
	*dc = *x
	dc.mask = before.mask
	dc.strokePath = before.strokePath
	dc.fillPath = before.fillPath
	dc.start = before.start
	dc.current = before.current
	dc.hasCurrent = before.hasCurrent
}

// p is a truetype.Point measured in FUnits and positive Y going upwards.
// The returned value is the same thing measured in floating point and positive Y
// going downwards.

func (dc *Context) drawGlyph(glyph truetype.Index, dx, dy float64) error {
	if err := dc.glyphBuf.Load(dc.font, fixed.Int26_6(dc.fontScale), glyph, font.HintingNone); err != nil {
		return err
	}
	e0 := 0
	for _, e1 := range dc.glyphBuf.Ends {
		dc.DrawContour(dc.glyphBuf.Points[e0:e1], dx, dy)
		e0 = e1
	}
	return nil
}

// CreateStringPath creates a path from the string s at x, y, and returns the string width.
// The text is placed so that the left edge of the em square of the first character of s
// and the baseline intersect at x, y. The majority of the affected pixels will be
// above and to the right of the point, but some may be below or to the left.
// For example, drawing a string that starts with a 'J' in an italic font may
// affect pixels below and left of the point.
func (dc *Context) CreateStringPath(s string, x, y float64) float64 {
	if dc.font == nil {
		log.Println("must load font")
		return 0.0
	}
	//dc.NewSubPath()
	//defer dc.ClosePath()
	startx := x
	prev, hasPrev := truetype.Index(0), false
	for _, rune := range s {
		index := dc.font.Index(rune)
		if hasPrev {
			x += fUnitsToFloat64(dc.font.Kern(fixed.Int26_6(dc.fontScale), prev, index))
		}
		err := dc.drawGlyph(index, x, y)
		if err != nil {
			log.Println(err)
			return startx - x
		}
		x += fUnitsToFloat64(dc.font.HMetric(fixed.Int26_6(dc.fontScale), index).AdvanceWidth)
		prev, hasPrev = index, true
	}
	return x - startx
}

func pointToF64Point(p truetype.Point) (x, y float64) {
	return fUnitsToFloat64(p.X), -fUnitsToFloat64(p.Y)
}

func fUnitsToFloat64(x fixed.Int26_6) float64 {
	scaled := x << 2
	return float64(scaled/256) + float64(scaled%256)/256.0
}

// DrawContour draws the given closed contour at the given sub-pixel offset.
func (dc *Context) DrawContour(ps []truetype.Point, dx, dy float64) {
	if len(ps) == 0 {
		return
	}
	startX, startY := pointToF64Point(ps[0])
	dc.MoveTo(startX+dx, startY+dy)
	q0X, q0Y, on0 := startX, startY, true
	for _, p := range ps[1:] {
		qX, qY := pointToF64Point(p)
		on := p.Flags&0x01 != 0
		if on {
			if on0 {
				dc.LineTo(qX+dx, qY+dy)
			} else {
				dc.QuadraticTo(q0X+dx, q0Y+dy, qX+dx, qY+dy)
			}
		} else {
			if on0 {
				// No-op.
			} else {
				midX := (q0X + qX) / 2
				midY := (q0Y + qY) / 2
				dc.QuadraticTo(q0X+dx, q0Y+dy, midX+dx, midY+dy)
			}
		}
		q0X, q0Y, on0 = qX, qY, on
	}
	// Close the curve.
	if on0 {
		dc.LineTo(startX+dx, startY+dy)
	} else {
		dc.QuadraticTo(q0X+dx, q0Y+dy, startX+dx, startY+dy)
	}
}
