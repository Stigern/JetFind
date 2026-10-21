package main

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// GLSL bodies shared by the desktop (110) and ES (100) variants. Fyne passes
// frame (output size in px), bounds (x1,y1,x2,y2 in px, top-left origin) and
// our own float uniforms. "w" is the object width in Fyne units, set by
// shaderStack, so shaders can convert between pixels and units.
const shaderCommon = `
uniform vec2 frame;
uniform vec4 bounds;
uniform float w;

float pxScale() { return (bounds[2] - bounds[0]) / max(w, 1.0); }

// position inside the object in pixels, origin top-left
vec2 localPx() {
	return vec2(gl_FragCoord.x - bounds[0], frame.y - gl_FragCoord.y - bounds[1]);
}

vec2 sizePx() { return vec2(bounds[2] - bounds[0], bounds[3] - bounds[1]); }

float sdRound(vec2 p, vec2 b, float r) {
	vec2 q = abs(p) - b + r;
	return length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - r;
}

`

// glassBody draws a card panel: solid fill, faint top sheen, a light edge
// that fades towards the bottom-right, a brief brightening driven by
// "flash", and a press wave from (rx, ry) driven by "ripple" (0–1).
const glassBody = `
uniform float radius;
uniform float hover;
uniform float flash;
uniform float ripple;
uniform float rx; uniform float ry;

void main() {
	float s = pxScale();
	vec2 size = sizePx();
	vec2 p = localPx();
	vec2 uv = p / size;
	float d = sdRound(p - size * 0.5, size * 0.5, radius * s);
	float inside = 1.0 - smoothstep(-1.0, 0.0, d);
	if (inside <= 0.0) discard;

	vec3 dark = vec3(0.118, 0.129, 0.161);
	float da = 1.0;

	// white layer: faint fill, top sheen
	vec3 tint = vec3(1.0);
	float a = 0.03 * hover + 0.16 * flash;
	a += 0.045 * (1.0 - smoothstep(0.0, 0.5, uv.y));
	if (ripple > 0.0 && ripple < 1.0) {
		float dist = length(p - vec2(rx, ry) * s);
		float r = ripple * length(size);
		float ring = exp(-pow((dist - r) / (7.0 * s), 2.0));
		float core = 1.0 - smoothstep(0.0, max(r, 1.0), dist);
		a += (0.22 * ring + 0.06 * core) * (1.0 - ripple);
	}

	float ba = a + da * (1.0 - a);
	vec3 bc = (tint * a + dark * da * (1.0 - a)) / ba;

	float edge = 1.0 - smoothstep(0.0, 1.4 * s, abs(d + 0.7 * s));
	float light = mix(0.16, 0.05, clamp(uv.x * 0.45 + uv.y * 0.85, 0.0, 1.0));
	vec3 edgeCol = vec3(1.0);
	float ea = edge * min(1.0, light + 0.12 * hover + 0.5 * flash);

	float oa = ea + ba * (1.0 - ea);
	vec3 oc = (edgeCol * ea + bc * ba * (1.0 - ea)) / max(oa, 0.001);
	gl_FragColor = vec4(oc, oa * inside);
}
`

// gradientBody fills a rounded rect with a horizontal two-colour gradient
// and a soft top highlight; "hover" brightens it. "ripple" (0–1) drives a
// wave expanding from (rx, ry), given in units, clipped to the shape.
const gradientBody = `
uniform float radius;
uniform float hover;
uniform float r1; uniform float g1; uniform float b1;
uniform float r2; uniform float g2; uniform float b2;
uniform float alpha;
uniform float ripple;
uniform float rx; uniform float ry;

void main() {
	float s = pxScale();
	vec2 size = sizePx();
	vec2 p = localPx();
	vec2 uv = p / size;
	float d = sdRound(p - size * 0.5, size * 0.5, radius * s);
	float inside = 1.0 - smoothstep(-1.0, 0.0, d);
	if (inside <= 0.0) discard;
	vec3 c = mix(vec3(r1, g1, b1), vec3(r2, g2, b2), uv.x);
	c += 0.18 * (1.0 - smoothstep(0.0, 0.55, uv.y));
	c += 0.10 * hover;
	if (ripple > 0.0 && ripple < 1.0) {
		float dist = length(p - vec2(rx, ry) * s);
		float r = ripple * length(size);
		float ring = exp(-pow((dist - r) / (5.0 * s), 2.0));
		float core = 1.0 - smoothstep(0.0, max(r, 1.0), dist);
		c += (0.45 * ring + 0.12 * core) * (1.0 - ripple);
	}
	gl_FragColor = vec4(c, alpha * inside);
}
`

const esHeader = `#version 100
#ifdef GL_ES
# ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
# else
precision mediump float;
# endif
#endif
`

func newShader(name, body string) *canvas.Shader {
	src := "#version 110\n" + shaderCommon + body
	es := esHeader + shaderCommon + body
	s := canvas.NewShader(name, []byte(src), []byte(es))
	// Shaders sharing a name share one GL program, and uniforms missing from
	// this map keep whatever the previously drawn object set. Start every
	// animated uniform at zero so effects never leak between objects.
	s.Uniforms = map[string]float32{"w": 0, "hover": 0, "flash": 0, "ripple": 0, "rx": 0, "ry": 0}
	return s
}

func newGlass(radius float32) *canvas.Shader {
	s := newShader("jetfind-glass", glassBody)
	s.Uniforms["radius"] = radius
	return s
}

func newGradientRect(radius float32, from, to color.Color) *canvas.Shader {
	s := newShader("jetfind-gradient", gradientBody)
	f := color.NRGBAModel.Convert(from).(color.NRGBA)
	t := color.NRGBAModel.Convert(to).(color.NRGBA)
	for k, v := range map[string]uint8{"r1": f.R, "g1": f.G, "b1": f.B, "r2": t.R, "g2": t.G, "b2": t.B} {
		s.Uniforms[k] = float32(v) / 255
	}
	s.Uniforms["radius"] = radius
	s.Uniforms["alpha"] = 1
	return s
}

// shaderStack lays objects on top of each other like container.Stack and
// tells every shader child its width in units ("w" uniform).
type shaderStack struct{}

func (shaderStack) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
		if s, ok := o.(*canvas.Shader); ok {
			s.Uniforms["w"] = size.Width
			s.Refresh()
		}
	}
}

func (shaderStack) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs {
		if _, ok := o.(*canvas.Shader); ok {
			continue
		}
		m = m.Max(o.MinSize())
	}
	return m
}

// animateUniform tweens a shader uniform from its current value to target.
func animateUniform(slot **fyne.Animation, s *canvas.Shader, name string, target float32, d time.Duration) {
	if *slot != nil {
		(*slot).Stop()
	}
	from := s.Uniforms[name]
	a := fyne.NewAnimation(d, func(v float32) {
		s.Uniforms[name] = from + (target-from)*v
		s.Refresh()
	})
	a.Curve = fyne.AnimationEaseOut
	*slot = a
	a.Start()
}
