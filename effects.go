package main

import (
	"image"
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// withAlpha returns c with its alpha replaced by a (0–1).
func withAlpha(c color.Color, a float32) color.NRGBA {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	n.A = uint8(math.Max(0, math.Min(255, float64(a)*255)))
	return n
}

// gradientTile renders a rounded square filled with a diagonal gradient,
// supersampled so the corners are smooth. size is in pixels.
func gradientTile(size int, radius float64, from, to color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4
	s := float64(size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			hit := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+.5)/ss
					py := float64(y) + (float64(sy)+.5)/ss
					cx := math.Max(radius, math.Min(px, s-radius))
					cy := math.Max(radius, math.Min(py, s-radius))
					if (px-cx)*(px-cx)+(py-cy)*(py-cy) <= radius*radius {
						hit++
					}
				}
			}
			if hit == 0 {
				continue
			}
			t := (float64(x) + float64(y)) / (2 * s)
			mix := func(a, b uint8) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }
			img.SetNRGBA(x, y, color.NRGBA{mix(from.R, to.R), mix(from.G, to.G), mix(from.B, to.B), uint8(255 * hit / (ss * ss))})
		}
	}
	return img
}

// statusDot is a small coloured dot that sends out one radar-style ring
// each time pulse is called. It is idle between pulses, so a window full of
// devices does not repaint continuously.
type statusDot struct {
	widget.BaseWidget
	dot, halo *canvas.Circle
	col       color.Color
	phase     float32 // 0→1 during a pulse; 1 when idle (ring invisible)
	anim      *fyne.Animation
}

func newStatusDot() *statusDot {
	s := &statusDot{dot: canvas.NewCircle(colFaint), halo: canvas.NewCircle(color.Transparent), col: colFaint, phase: 1}
	s.ExtendBaseWidget(s)
	return s
}

// set changes the colour.
func (s *statusDot) set(c color.Color) {
	s.col = c
	s.dot.FillColor = c
	s.dot.Refresh()
}

// pulse sends out a single ring.
func (s *statusDot) pulse() {
	s.stop()
	s.anim = fyne.NewAnimation(1400*time.Millisecond, func(v float32) {
		s.phase = v
		s.layoutHalo()
	})
	s.anim.Curve = fyne.AnimationEaseOut
	s.anim.Start()
}

func (s *statusDot) stop() {
	if s.anim != nil {
		s.anim.Stop()
		s.anim = nil
	}
	s.phase = 1
	s.layoutHalo()
}

func (s *statusDot) layoutHalo() {
	size := s.Size()
	cx, cy := size.Width/2, size.Height/2
	r := 4 + s.phase*7
	s.halo.Move(fyne.NewPos(cx-r, cy-r))
	s.halo.Resize(fyne.NewSize(2*r, 2*r))
	s.halo.FillColor = withAlpha(s.col, 0.55*(1-s.phase))
	s.halo.Refresh()
}

func (s *statusDot) CreateRenderer() fyne.WidgetRenderer { return &statusDotRenderer{s} }

type statusDotRenderer struct{ s *statusDot }

func (r *statusDotRenderer) Layout(size fyne.Size) {
	r.s.dot.Move(fyne.NewPos(size.Width/2-4, size.Height/2-4))
	r.s.dot.Resize(fyne.NewSize(8, 8))
	r.s.layoutHalo()
}
func (r *statusDotRenderer) MinSize() fyne.Size { return fyne.NewSize(22, 22) }
func (r *statusDotRenderer) Refresh()           { r.Layout(r.s.Size()) }
func (r *statusDotRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.s.halo, r.s.dot}
}
func (r *statusDotRenderer) Destroy() {}

// sparkline draws recent latency samples as a tiny line chart. Its line
// segments are created once and reused.
type sparkline struct {
	widget.BaseWidget
	values []time.Duration
	col    color.Color
	segs   []*canvas.Line
	objs   []fyne.CanvasObject
}

const sparkSamples = 24

func newSparkline() *sparkline {
	s := &sparkline{col: colOnline}
	for range sparkSamples - 1 {
		l := canvas.NewLine(color.Transparent)
		l.StrokeWidth = 1.5
		l.Hide()
		s.segs = append(s.segs, l)
		s.objs = append(s.objs, l)
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *sparkline) set(values []time.Duration, c color.Color) {
	if len(values) > sparkSamples {
		values = values[len(values)-sparkSamples:]
	}
	s.values, s.col = values, c
	s.Refresh()
}

func (s *sparkline) CreateRenderer() fyne.WidgetRenderer { return &sparkRenderer{s} }

type sparkRenderer struct{ s *sparkline }

func (r *sparkRenderer) Layout(size fyne.Size) {
	s := r.s
	v := s.values
	for i, l := range s.segs {
		l.Hidden = i >= len(v)-1
	}
	if len(v) < 2 {
		return
	}
	peak := 5 * time.Millisecond
	for _, d := range v {
		peak = max(peak, d)
	}
	step := size.Width / float32(sparkSamples-1)
	x0 := size.Width - step*float32(len(v)-1)
	pt := func(i int) fyne.Position {
		y := size.Height - 1 - float32(v[i])/float32(peak)*(size.Height-2)
		return fyne.NewPos(x0+step*float32(i), y)
	}
	for i := 1; i < len(v); i++ {
		l := s.segs[i-1]
		l.StrokeColor = withAlpha(s.col, 0.35+0.65*float32(i)/float32(len(v)))
		l.Position1, l.Position2 = pt(i-1), pt(i)
	}
}
func (r *sparkRenderer) MinSize() fyne.Size           { return fyne.NewSize(64, 18) }
func (r *sparkRenderer) Refresh()                     { r.Layout(r.s.Size()); canvas.Refresh(r.s) }
func (r *sparkRenderer) Objects() []fyne.CanvasObject { return r.s.objs }
func (r *sparkRenderer) Destroy()                     {}

// offsetLayout draws its single child shifted down by dy; animating dy gives
// a slide-in without disturbing the surrounding list layout.
type offsetLayout struct{ dy float32 }

func (l *offsetLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, l.dy))
		o.Resize(size)
	}
}

func (l *offsetLayout) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

// slideIn wraps obj so it glides up into place.
func slideIn(obj fyne.CanvasObject) *fyne.Container {
	l := &offsetLayout{dy: 26}
	c := container.New(l, obj)
	a := fyne.NewAnimation(550*time.Millisecond, func(v float32) {
		l.dy = 26 * (1 - v)
		c.Refresh()
	})
	a.Curve = fyne.AnimationEaseOut
	a.Start()
	return c
}

// glowButton is a gradient pill with a coloured glow that pulses while busy.
type glowButton struct {
	widget.BaseWidget
	text    string
	onTap   func()
	busy    bool
	bg      *canvas.Shader
	glow    *canvas.Rectangle
	label   *canvas.Text
	icon    *canvas.Image
	hoverA  *fyne.Animation
	pulseA  *fyne.Animation
	rippleA *fyne.Animation
}

func newGlowButton(text string, icon fyne.Resource, onTap func()) *glowButton {
	b := &glowButton{text: text, onTap: onTap}
	b.icon = canvas.NewImageFromResource(theme.NewColoredResource(icon, theme.ColorNameForeground))
	b.icon.FillMode = canvas.ImageFillContain
	b.ExtendBaseWidget(b)
	return b
}

func (b *glowButton) CreateRenderer() fyne.WidgetRenderer {
	b.bg = newGradientRect(10, colAccent, colAccent2)
	b.glow = canvas.NewRectangle(color.Transparent)
	b.glow.CornerRadius = 10
	b.glow.Shadow = canvas.Shadow{Color: withAlpha(colAccent, 0.55), BlurRadius: 18, Offset: fyne.NewPos(0, 3)}
	b.label = canvas.NewText(b.text, colText)
	b.label.TextStyle = fyne.TextStyle{Bold: true}
	b.label.TextSize = 14
	content := container.New(layout.NewCustomPaddedLayout(8, 8, 12, 14),
		container.NewHBox(container.NewGridWrap(fyne.NewSize(16, 16), b.icon), b.label))
	return widget.NewSimpleRenderer(container.New(shaderStack{}, b.glow, b.bg, content))
}

// setBusy shows text and dims the button, pulsing its glow until done.
func (b *glowButton) setBusy(busy bool, text string) {
	b.busy = busy
	b.text = text
	if b.label == nil {
		return
	}
	b.label.Text = text
	b.label.Refresh()
	if b.pulseA != nil {
		b.pulseA.Stop()
		b.pulseA = nil
	}
	b.bg.Uniforms["alpha"] = 1
	if busy {
		b.bg.Uniforms["alpha"] = 0.55
		b.pulseA = fyne.NewAnimation(900*time.Millisecond, func(v float32) {
			b.glow.Shadow.Color = withAlpha(colAccent2, 0.25+0.5*v)
			b.glow.Shadow.BlurRadius = 14 + 10*v
			b.glow.Refresh()
		})
		b.pulseA.AutoReverse = true
		b.pulseA.RepeatCount = fyne.AnimationRepeatForever
		b.pulseA.Curve = fyne.AnimationEaseInOut
		b.pulseA.Start()
	} else {
		b.glow.Shadow.Color = withAlpha(colAccent, 0.55)
		b.glow.Shadow.BlurRadius = 18
		b.glow.Refresh()
	}
	b.bg.Refresh()
}

// Tapped sends a ripple out from the press point, then runs the action.
func (b *glowButton) Tapped(ev *fyne.PointEvent) {
	if b.busy || b.onTap == nil {
		return
	}
	b.bg.Uniforms["rx"], b.bg.Uniforms["ry"] = ev.Position.X, ev.Position.Y
	b.bg.Uniforms["ripple"] = 0
	animateUniform(&b.rippleA, b.bg, "ripple", 1, 650*time.Millisecond)
	b.onTap()
}

func (b *glowButton) MouseIn(*desktop.MouseEvent) {
	animateUniform(&b.hoverA, b.bg, "hover", 1, 150*time.Millisecond)
}
func (b *glowButton) MouseMoved(*desktop.MouseEvent) {}
func (b *glowButton) MouseOut() {
	animateUniform(&b.hoverA, b.bg, "hover", 0, 250*time.Millisecond)
}
func (b *glowButton) Cursor() desktop.Cursor { return desktop.PointerCursor }
