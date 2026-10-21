// Command makeicon draws Icon.png: a glossy rounded tile in the JetFind
// blue→purple palette with a bold white "JF", styled after FuMachine's icon.
//
//	go run ./tools/makeicon Icon.png
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	out    = 1024
	ss     = 2 // supersampling factor
	S      = out * ss
	margin = 100 * ss // macOS icon grid: 824pt tile inside 1024
	radius = 185 * ss
)

var (
	accent  = [3]float64{0x2a, 0x6d, 0xf4}
	accent2 = [3]float64{0x7b, 0x4d, 0xff}
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: makeicon OUT.png")
	}
	img := image.NewNRGBA(image.Rect(0, 0, S, S))
	x0, y0, x1, y1 := float64(margin), float64(margin), float64(S-margin), float64(S-margin)

	// soft drop shadow under the tile
	shadow := image.NewAlpha(image.Rect(0, 0, S, S))
	fillMask(shadow, func(x, y float64) float64 {
		return coverage(x, y-14*ss, x0+6*ss, y0+6*ss, x1-6*ss, y1-6*ss, radius) * 0.55
	})
	blurAlpha(shadow, 18*ss)
	draw.DrawMask(img, img.Bounds(), image.NewUniform(color.Black), image.Point{}, shadow, image.Point{}, draw.Over)

	// tile: darker rim, then the body inset by the rim width
	rim := 7.0 * ss
	tile := image.NewNRGBA(img.Bounds())
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			fx, fy := float64(x)+.5, float64(y)+.5
			outer := coverage(fx, fy, x0, y0, x1, y1, radius)
			if outer == 0 {
				continue
			}
			inner := coverage(fx, fy, x0+rim, y0+rim, x1-rim, y1-rim, radius-rim)
			u, v := (fx-x0)/(x1-x0), (fy-y0)/(y1-y0)
			base := mix(accent, accent2, clamp(u*0.55+v*0.45))
			// vertical shading: lighter top, deeper bottom
			shade := 1.22 - 0.45*v
			body := scale(base, shade)
			// gloss: bright cap over the top, bounded by a wide ellipse
			ex, ey := (u-0.5)/0.95, (v+0.35)/0.85
			if e := ex*ex + ey*ey; e < 1 {
				g := 0.16 + 0.10*(1-v/0.5)
				body = lerp(body, [3]float64{255, 255, 255}, g*smoothstep(1, 0.97, e))
			}
			c := lerp(scale(base, 0.68), body, inner)
			// thin highlight just inside the top rim
			if inner > 0 && fy < y0+rim+10*ss {
				c = lerp(c, [3]float64{255, 255, 255}, 0.18*inner*smoothstep(y0+rim+10*ss, y0+rim, fy))
			}
			tile.SetNRGBA(x, y, color.NRGBA{u8(c[0]), u8(c[1]), u8(c[2]), u8(255 * outer)})
		}
	}
	draw.Draw(img, img.Bounds(), tile, image.Point{}, draw.Over)

	// "JF" in bold white with a soft shadow
	text := textMask("JF", 470*ss)
	tb := text.Bounds()
	tx := (S-tb.Dx())/2 - tb.Min.X
	ty := (S-tb.Dy())/2 - tb.Min.Y - 4*ss
	glyphs := image.NewAlpha(img.Bounds())
	draw.Draw(glyphs, tb.Add(image.Pt(tx, ty)), text, tb.Min, draw.Src)
	gshadow := image.NewAlpha(img.Bounds())
	draw.Draw(gshadow, glyphs.Bounds().Add(image.Pt(0, 12*ss)), glyphs, image.Point{}, draw.Src)
	scaleAlpha(gshadow, 0.38)
	blurAlpha(gshadow, 10*ss)
	draw.DrawMask(img, img.Bounds(), image.NewUniform(color.NRGBA{0x10, 0x10, 0x40, 255}), image.Point{}, gshadow, image.Point{}, draw.Over)
	draw.DrawMask(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, glyphs, image.Point{}, draw.Over)

	final := image.NewNRGBA(image.Rect(0, 0, out, out))
	xdraw.CatmullRom.Scale(final, final.Bounds(), img, img.Bounds(), xdraw.Src, nil)
	f, err := os.Create(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, final); err != nil {
		log.Fatal(err)
	}
}

// heavyFont is the macOS font closest to FuMachine's lettering; Go Bold is
// the fallback when it is missing.
const heavyFont = "/System/Library/Fonts/Supplemental/Arial Black.ttf"

// textMask renders s as a tightly cropped alpha mask with the given size.
func textMask(s string, size float64) *image.Alpha {
	data := gobold.TTF
	if b, err := os.ReadFile(heavyFont); err == nil {
		data = b
	}
	ft, err := opentype.Parse(data)
	if err != nil {
		log.Fatal(err)
	}
	face, err := opentype.NewFace(ft, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		log.Fatal(err)
	}
	d := &font.Drawer{Face: face, Src: image.White}
	b, _ := d.BoundString(s)
	r := image.Rect(b.Min.X.Floor(), b.Min.Y.Floor(), b.Max.X.Ceil(), b.Max.Y.Ceil())
	m := image.NewAlpha(r)
	d.Dst = m
	d.Dot = fixed.Point26_6{}
	d.DrawString(s)
	return m
}

// coverage is the fraction of pixel (x, y) inside a rounded rect, using a
// signed distance with a one-pixel antialiasing ramp.
func coverage(x, y, x0, y0, x1, y1, r float64) float64 {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2, (y1-y0)/2
	qx, qy := math.Abs(x-cx)-hx+r, math.Abs(y-cy)-hy+r
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
	return clamp(0.5 - d)
}

func fillMask(m *image.Alpha, f func(x, y float64) float64) {
	b := m.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			m.SetAlpha(x, y, color.Alpha{u8(255 * f(float64(x)+.5, float64(y)+.5))})
		}
	}
}

func scaleAlpha(m *image.Alpha, k float64) {
	for i, v := range m.Pix {
		m.Pix[i] = u8(float64(v) * k)
	}
}

// blurAlpha approximates a gaussian with three box-blur passes.
func blurAlpha(m *image.Alpha, r int) {
	for range 3 {
		boxBlur(m, r, true)
		boxBlur(m, r, false)
	}
}

func boxBlur(m *image.Alpha, r int, horizontal bool) {
	b := m.Bounds()
	w, h := b.Dx(), b.Dy()
	src := append([]uint8(nil), m.Pix...)
	n, lines := w, h
	if !horizontal {
		n, lines = h, w
	}
	at := func(line, i int) int {
		if horizontal {
			return line*m.Stride + i
		}
		return i*m.Stride + line
	}
	for line := 0; line < lines; line++ {
		sum := 0
		for i := -r; i <= r; i++ {
			sum += int(src[at(line, min(max(i, 0), n-1))])
		}
		for i := 0; i < n; i++ {
			m.Pix[at(line, i)] = uint8(sum / (2*r + 1))
			sum += int(src[at(line, min(i+r+1, n-1))]) - int(src[at(line, max(i-r, 0))])
		}
	}
}

func mix(a, b [3]float64, t float64) [3]float64 { return lerp(a, b, t) }

func lerp(a, b [3]float64, t float64) [3]float64 {
	return [3]float64{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

func scale(c [3]float64, k float64) [3]float64 { return [3]float64{c[0] * k, c[1] * k, c[2] * k} }

func clamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func smoothstep(e0, e1, x float64) float64 {
	t := clamp((x - e0) / (e1 - e0))
	return t * t * (3 - 2*t)
}

func u8(v float64) uint8 { return uint8(math.Max(0, math.Min(255, math.Round(v)))) }
