package theme

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func tokens(t *testing.T, css []byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`([a-z-]+):\s*([^;]+);`).FindAllStringSubmatch(string(css), -1) {
		out[strings.TrimPrefix(m[1], "--")] = m[2]
	}
	return out
}

func TestFromOmarchy(t *testing.T) {
	monochrome := `
accent = "#F1396D"
foreground = "#FFEAF2"
background = "#191414"
color1 = "#F1396D"
color2 = "#D93665"
color3 = "#FFB3CA"
color4 = "#FF6F9B"
color5 = "#F1396D"
`
	css, err := fromOmarchy([]byte(monochrome))
	if err != nil {
		t.Fatal(err)
	}
	got := tokens(t, css)
	if got["color-scheme"] != "dark" || got["canvas"] != "#191414" || got["accent"] != "#f1396d" {
		t.Fatalf("base tokens = %v", got)
	}
	if got["ok"] != darkStatus[0] || got["warn"] != darkStatus[1] || got["bad"] != darkStatus[2] {
		t.Errorf("pink status colours or a red equal to the accent were kept: %v", got)
	}

	light := `
mode = "light"
accent = "#1e66f5"
background = "#eff1f5"
foreground = "#4c4f69"
red = "#d20f39"
yellow = "#df8e1d"
green = "#40a02b"
`
	css, err = fromOmarchy([]byte(light))
	if err != nil {
		t.Fatal(err)
	}
	got = tokens(t, css)
	if bad, _ := parseHex(got["bad"]); got["color-scheme"] != "light" || got["bad"] == lightStatus[2] || !bad.near(0, 25) {
		t.Fatalf("light tokens = %v", got)
	}
	bg, _ := parseHex("#eff1f5")
	fg, _ := parseHex("#4c4f69")
	if accent, _ := parseHex(got["accent"]); contrast(accent, bg) < 4.5 {
		t.Errorf("accent %s is unreadable on the canvas", got["accent"])
	}
	for _, name := range []string{"muted", "ok", "warn", "bad", "t-str"} {
		c, _ := parseHex(got[name])
		if r := contrast(c, bg.mix(fg, 0.08)); r < 4.5 {
			t.Errorf("%s %s has contrast %.2f", name, got[name], r)
		}
	}

	if _, err := fromOmarchy([]byte(`accent = "#fff"`)); err == nil {
		t.Error("a palette without background and foreground was accepted")
	}
}

func TestResolve(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, name, err := Resolve(""); err != nil || name != "default" {
		t.Fatalf("Resolve without Omarchy = %q, %v", name, err)
	}
	if _, _, err := Resolve("omarchy"); err == nil {
		t.Fatal(`Resolve("omarchy") without Omarchy succeeded`)
	}

	colors := filepath.Join(home, ".local", "state", "omarchy", "current", "theme", "colors.toml")
	if err := os.MkdirAll(filepath.Dir(colors), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(bg string) {
		palette := `accent = "#7aa2f7"` + "\nforeground = \"#a9b1d6\"\nbackground = \"" + bg + "\"\n"
		if err := os.WriteFile(colors, []byte(palette), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("#1a1b26")
	src, name, err := Resolve("")
	if err != nil || name != "omarchy" {
		t.Fatalf("Resolve with Omarchy = %q, %v", name, err)
	}
	write("#000000")
	if css, _ := src(); tokens(t, css)["canvas"] != "#000000" {
		t.Errorf("theme switch not picked up: %s", css)
	}

	file := filepath.Join(home, "brand.css")
	if err := os.WriteFile(file, []byte(":root { --accent: #00ff00; }"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, _, err = Resolve(file)
	if err != nil {
		t.Fatal(err)
	}
	if css, _ := src(); !strings.HasPrefix(string(css), string(Default)) || !strings.HasSuffix(string(css), "#00ff00; }") {
		t.Errorf("file theme is not layered over the default: %s", css)
	}
	if _, _, err := Resolve(filepath.Join(home, "missing.css")); err == nil {
		t.Error("a missing theme file was accepted")
	}
}
