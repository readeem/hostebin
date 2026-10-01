package theme

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var tomlString = regexp.MustCompile(`(?m)^\s*([a-z0-9_]+)\s*=\s*"([^"]*)"`)

var legacyNames = map[string]string{
	"red": "color1", "green": "color2", "yellow": "color3", "blue": "color4", "magenta": "color5",
}

var (
	darkStatus  = [3]string{"#4fd69c", "#f2c14e", "#ff6a3d"}
	lightStatus = [3]string{"#1a7f4b", "#9a6700", "#c2410c"}
)

func fromOmarchy(raw []byte) ([]byte, error) {
	p := map[string]string{}
	for _, m := range tomlString.FindAllSubmatch(raw, -1) {
		p[string(m[1])] = string(m[2])
	}
	for name, legacy := range legacyNames {
		if p[name] == "" {
			p[name] = p[legacy]
		}
	}
	canvas, okBG := parseHex(p["background"])
	ink, okFG := parseHex(p["foreground"])
	accent, okAccent := parseHex(p["accent"])
	if !okBG || !okFG || !okAccent {
		return nil, errors.New("colors.toml needs background, foreground and accent")
	}

	scheme, fallback, pole := "dark", darkStatus, rgb{1, 1, 1}
	if canvas.luminance() > ink.luminance() {
		scheme, fallback, pole = "light", lightStatus, rgb{}
	}
	status := func(name string, hue, spread float64, def string) rgb {
		if c, ok := parseHex(p[name]); ok && c.near(hue, spread) {
			return c
		}
		c, _ := parseHex(def)
		return c
	}
	ok := status("green", 125, 50, fallback[0])
	warn := status("yellow", 50, 22, fallback[1])
	bad := status("red", 0, 25, fallback[2])
	if h, s := accent.hsl(); s >= minSaturation && bad.near(h, 20) {
		bad, _ = parseHex(fallback[2])
	}
	raised := canvas.mix(ink, 0.08)
	text := func(c rgb) string {
		return c.towards(pole, func(m rgb) bool {
			chip := canvas.mix(ink, 0.04).mix(m, 0.14)
			return contrast(m, raised) >= minContrast && contrast(m, chip) >= minContrast
		}).String()
	}
	syntax := func(name string) string {
		if c, ok := parseHex(p[name]); ok {
			return text(c)
		}
		return ink.String()
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, ":root {\n  color-scheme: %s;\n", scheme)
	for _, t := range [][2]string{
		{"canvas", canvas.String()},
		{"ink", ink.String()},
		{"muted", canvas.mix(ink, 0.64).towards(ink, func(m rgb) bool { return contrast(m, raised) >= minContrast }).String()},
		{"accent", accent.towards(pole, func(m rgb) bool { return contrast(m, canvas) >= minContrast }).String()},
		{"ok", text(ok)},
		{"warn", text(warn)},
		{"bad", text(bad)},
		{"t-cm", "var(--muted)"},
		{"t-str", syntax("green")},
		{"t-num", syntax("yellow")},
		{"t-kw", syntax("magenta")},
		{"t-fn", syntax("blue")},
	} {
		fmt.Fprintf(&b, "  --%s: %s;\n", t[0], t[1])
	}
	b.WriteString("}\n")
	return b.Bytes(), nil
}

const (
	minSaturation = 0.25
	minContrast   = 4.6
)

type rgb struct{ r, g, b float64 }

func parseHex(s string) (rgb, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 8 {
		s = s[:6]
	}
	if len(s) != 6 {
		return rgb{}, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgb{}, false
	}
	return rgb{float64(n>>16&255) / 255, float64(n>>8&255) / 255, float64(n&255) / 255}, true
}

func (c rgb) String() string {
	ch := func(v float64) int { return int(math.Round(v * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", ch(c.r), ch(c.g), ch(c.b))
}

func (c rgb) luminance() float64 {
	lin := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

func contrast(a, b rgb) float64 {
	la, lb := a.luminance(), b.luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func (c rgb) mix(o rgb, t float64) rgb {
	return rgb{c.r + (o.r-c.r)*t, c.g + (o.g-c.g)*t, c.b + (o.b-c.b)*t}
}

func (c rgb) towards(pole rgb, ok func(rgb) bool) rgb {
	for i := range 21 {
		if m := c.mix(pole, float64(i)/20); ok(m) {
			return m
		}
	}
	return pole
}

func (c rgb) hsl() (hue, saturation float64) {
	hi, lo := max(c.r, c.g, c.b), min(c.r, c.g, c.b)
	d := hi - lo
	if d == 0 {
		return 0, 0
	}
	switch hi {
	case c.r:
		hue = math.Mod((c.g-c.b)/d+6, 6)
	case c.g:
		hue = (c.b-c.r)/d + 2
	default:
		hue = (c.r-c.g)/d + 4
	}
	return hue * 60, d / (1 - math.Abs(hi+lo-1))
}

func (c rgb) near(hue, spread float64) bool {
	h, s := c.hsl()
	d := math.Abs(h - hue)
	return s >= minSaturation && min(d, 360-d) <= spread
}
