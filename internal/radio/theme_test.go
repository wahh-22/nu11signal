package radio

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// themedView is the frame of view_80x24.golden with its escape codes:
// the stripped goldens cannot tell one theme from another.
func themedView(t *testing.T) Model {
	t.Helper()
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	for range 12 {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	return m
}

// The REDSHIFT frame, escape codes included, was recorded before
// themes existed: the default theme draws exactly what the app drew.
func TestRedshiftDrawsTheOriginalColors(t *testing.T) {
	assertGolden(t, "view_redshift_ansi_80x24.golden", themedView(t).View().Content)
}

// useTheme applies the theme named name for the rest of the test, and
// REDSHIFT again after it: the styles are package state. A new Model
// applies REDSHIFT, so the models are made first.
func useTheme(t *testing.T, name string) {
	t.Helper()
	th, ok := themeNamed(name)
	if !ok {
		t.Fatalf("no theme %q", name)
	}
	applyTheme(th)
	t.Cleanup(func() { applyTheme(themes[0]) })
}

// redshiftCodes are the escape code colors of the REDSHIFT palette.
var redshiftCodes = []string{
	"255;95;87", "232;85;78", "94;246;255", "252;238;10",
	"90;30;30", "154;59;55", "14;42;47", "10;10;10",
}

func TestThemesStartWithRedshiftThenBlue(t *testing.T) {
	var names []string
	for _, th := range themes {
		names = append(names, th.name)
	}
	if strings.Join(names, ",") != "REDSHIFT,BLUE,MATRIX,ROSE,NEON ROSE" {
		t.Fatalf("themes %v; want REDSHIFT, BLUE, MATRIX, ROSE, NEON ROSE", names)
	}
	if th, ok := themeNamed("blue"); !ok || th.name != "BLUE" {
		t.Fatalf("themeNamed(blue) = %v, %v; want BLUE, case aside", th.name, ok)
	}
	if _, ok := themeNamed("neon"); ok {
		t.Fatal("themeNamed(neon) found a theme")
	}
}

func TestGentlemanSemanticRolesAndANSI(t *testing.T) {
	cases := []struct {
		name, lookup, label, number, hi, heading, warn, ok, fill, ink, selected string
	}{
		{"ROSE", " rose ", "#F6EFF3", "#F2B86D", "#F095C8", "#F2B86D", "#F2B86D", "#B4E7C7", "#FFB1DD", "#060407", "#28121E"},
		{"NEON ROSE", " neon rose ", "#F6EFF3", "#F2B86D", "#F43888", "#F2B86D", "#F2B86D", "#D2CBD0", "#FF4F9A", "#060407", "#28121E"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			th, ok := themeNamed(tc.lookup)
			if !ok || th.name != tc.name {
				t.Fatalf("lookup %q: %q, %v", tc.lookup, th.name, ok)
			}
			for role, pair := range map[string][2]string{
				"label": {th.label, tc.label}, "number": {th.number, tc.number}, "hi": {th.hi, tc.hi},
				"heading": {th.heading, tc.heading}, "warn": {th.warn, tc.warn},
				"ok": {th.ok, tc.ok}, "fill": {th.fill, tc.fill},
				"ink": {th.ink, tc.ink}, "selectBg": {th.selectBg, tc.selected},
			} {
				if pair[0] != pair[1] {
					t.Errorf("%s = %s; want %s", role, pair[0], pair[1])
				}
			}
			applyTheme(th)
			t.Cleanup(func() { applyTheme(themes[0]) })
			for role, pair := range map[string][2]string{
				"number":    {stNumber.Render("x"), tc.number},
				"highlight": {stHi.Render("x"), tc.hi},
				"heading":   {stHeading.Render("x"), tc.heading},
				"warning":   {stWarn.Render("x"), tc.warn},
				"success":   {stOK.Render("x"), tc.ok},
				"button":    {stButtonOn.Render("x"), tc.fill},
				"selection": {stSelected.Render("x"), tc.selected},
			} {
				var r, g, b int
				fmt.Sscanf(pair[1], "#%02x%02x%02x", &r, &g, &b)
				code := fmt.Sprintf("%d;%d;%d", r, g, b)
				if !strings.Contains(pair[0], code) {
					t.Errorf("%s ANSI %q lacks %s", role, pair[0], code)
				}
			}
			if strings.Contains(stText.Render("x"), "\x1b[48;") {
				t.Error("ordinary text changes terminal background")
			}
		})
	}
}

func TestPinkThemesUsePiCoreRoles(t *testing.T) {
	for _, tc := range []struct {
		name, accent, active, success string
	}{
		{"ROSE", "#F095C8", "#FFB1DD", "#B4E7C7"},
		{"NEON ROSE", "#F43888", "#FF4F9A", "#D2CBD0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th, ok := themeNamed(tc.name)
			if !ok {
				t.Fatal("missing theme")
			}
			for role, pair := range map[string][2]string{
				"ink/bg": {th.ink, "#060407"}, "text": {th.text, "#F6EFF3"},
				"selectBg/selectedBg": {th.selectBg, "#28121E"}, "muted": {th.muted, "#A78E9B"},
				"hi/accent": {th.hi, tc.accent}, "accent/activePink": {th.accent, tc.active},
				"warn/warning": {th.warn, "#F2B86D"}, "ok/success": {th.ok, tc.success},
				"logo/accent": {th.logo, tc.accent}, "logoAlt/activePink": {th.logoAlt, tc.active},
			} {
				if pair[0] != pair[1] {
					t.Errorf("%s = %s; want %s", role, pair[0], pair[1])
				}
			}
			allowed := map[string]bool{"#060407": true, "#F6EFF3": true, "#28121E": true, "#A78E9B": true,
				tc.accent: true, tc.active: true, "#F2B86D": true, tc.success: true}
			used := map[string]bool{}
			for _, c := range themeRoles(th) {
				if !allowed[c] {
					t.Errorf("unexpected radio role color %s", c)
				}
				used[c] = true
			}
			if len(used) != 8 {
				t.Errorf("used %d colors; want eight", len(used))
			}
		})
	}
}

func TestPinkThemesUseExactlyEightColors(t *testing.T) {
	for _, name := range []string{"ROSE", "NEON ROSE"} {
		t.Run(name, func(t *testing.T) {
			th, ok := themeNamed(name)
			if !ok {
				t.Fatalf("missing %s", name)
			}
			colors := map[string]bool{}
			for _, c := range themeRoles(th) {
				colors[c] = true
			}
			if len(colors) != 8 {
				t.Errorf("%s uses %d colors: %v", name, len(colors), colors)
			}
		})
	}
	for _, old := range []string{"GENTLEMAN CUTE", "GENTLEMAN SEXY"} {
		if _, ok := themeNamed(old); ok {
			t.Errorf("obsolete theme %s still accepted", old)
		}
	}
}

func TestBlueRecolorsEveryStyle(t *testing.T) {
	useTheme(t, "BLUE")
	const primary = "52;122;255"
	for name, got := range map[string]string{
		"stLabel":        stLabel.Render("x"),
		"stLabelBold":    stLabelBold.Render("x"),
		"inkBright":      vizInks[inkBright].pre,
		"noise":          noiseStyles[0].Render("x"),
		"stAlertStatic":  stAlertStatic.Render("x"),
		"stText":         stText.Render("x"),
		"stNumber":       stNumber.Render("x"),
		"stHi":           stHi.Render("x") + "/" + blueCyan,
		"stAccent":       stAccent.Render("x") + "/" + blueViolet,
		"stOnAir":        stOnAir.Render("x") + "/" + blueViolet,
		"stHeading":      stHeading.Render("x") + "/" + blueViolet,
		"stButtonOn":     stButtonOn.Render("x") + "/" + blueViolet,
		"stFillEdge":     stFillEdge.Render("x") + "/" + blueViolet,
		"stAlertSign":    stAlertSign.Render("x"),
		"inkBody":        vizInks[inkBody].pre,
		"inkTip":         vizInks[inkTip].pre + "/" + blueViolet,
		"stWarn":         stWarn.Render("x") + "/" + blueViolet,
		"stFocus":        stFocus.Render("x") + "/" + blueViolet,
		"stFav":          stFav.Render("x") + "/" + blueViolet,
		"stOK":           stOK.Render("x") + "/" + blueCyan,
		"stSelected bg":  stSelected.Render("x") + "/16;24;46",
		"stFrameDim":     stFrameDim.Render("x") + "/28;44;84",
		"stMuted":        stMuted.Render("x") + "/74;85;120",
		"stButtonOn ink": stButtonOn.Render("x") + "/5;7;15",
	} {
		want := primary
		if i := strings.LastIndex(got, "/"); i >= 0 {
			got, want = got[:i], got[i+1:]
		}
		if !strings.Contains(got, want) {
			t.Errorf("%s = %q; want the BLUE color %s", name, got, want)
		}
	}
	if c := inputStyles().Cursor.Color; c == nil || fmt.Sprint(c.RGBA()) != fmt.Sprint(lipgloss.Color("#7C5CFF").RGBA()) {
		t.Errorf("input cursor color %v; want the BLUE focus violet", c)
	}
}

func TestBlueLeavesNoRedshiftColor(t *testing.T) {
	m := themedView(t)
	models := map[string]Model{"view": m}
	models["help"], _ = press(t, m, "?")
	models["settings"], _ = press(t, m, "s")
	models["expanded"], _ = press(t, m, "f")
	models["player focus"], _ = press(t, m, "right", "right")
	models["search"], _ = press(t, m, "/")
	c := newClock()
	models["no signal"] = forceBurst(t, fxModel(t, c), c, true)
	c = newClock()
	models["glitch"] = forceBurst(t, fxModel(t, c), c, false)
	useTheme(t, "BLUE")
	blue, _ := themeNamed("BLUE")
	for name, m := range models {
		// The text inputs hold copies of their styles: the Model's own
		// setTheme refreshes them, as SETTINGS does.
		frame := m.setTheme(blue).View().Content
		for _, code := range redshiftCodes {
			if strings.Contains(frame, code) {
				t.Errorf("%s under BLUE draws the REDSHIFT color %s", name, code)
			}
		}
	}
}

// The BLUE frame with its escape codes: the stripped one is the same as
// view_80x24.golden.
func TestBlueGolden80x24(t *testing.T) {
	m := themedView(t)
	useTheme(t, "BLUE")
	assertGolden(t, "view_blue_80x24.golden", m.View().Content)
}

func TestMatrixGolden80x24(t *testing.T) {
	m := themedView(t)
	useTheme(t, "MATRIX")
	assertGolden(t, "view_matrix_80x24.golden", m.View().Content)
}

func TestRoseGolden80x24(t *testing.T) {
	m := themedView(t)
	useTheme(t, "ROSE")
	assertGolden(t, "view_rose_80x24.golden", m.View().Content)
}

func TestNeonRoseGolden80x24(t *testing.T) {
	m := themedView(t)
	useTheme(t, "NEON ROSE")
	assertGolden(t, "view_neon_rose_80x24.golden", m.View().Content)
}

// redshiftViews are the frames of the views beyond the main one, escape
// codes included, under the default theme, joined in name order.
func redshiftViews(t *testing.T) string {
	t.Helper()
	m := themedView(t)
	models := map[string]Model{}
	models["help"], _ = press(t, m, "?")
	models["settings"], _ = press(t, m, "s")
	models["expanded"], _ = press(t, m, "f")
	models["player focus"], _ = press(t, m, "right", "right")
	models["search"], _ = press(t, m, "/")
	c := newClock()
	models["no signal"] = forceBurst(t, fxModel(t, c), c, true)
	c = newClock()
	models["glitch"] = forceBurst(t, fxModel(t, c), c, false)
	var b strings.Builder
	for _, name := range []string{"expanded", "glitch", "help", "no signal", "player focus", "search", "settings"} {
		fmt.Fprintf(&b, "== %s ==\n%s\n", name, models[name].View().Content)
	}
	return b.String()
}

// The REDSHIFT frames of the other views, recorded before the finer
// theme roles existed: splitting the roles changed none of their bytes.
func TestRedshiftKeepsItsColorsInEveryView(t *testing.T) {
	assertGolden(t, "redshift_ansi_views_80x24.golden", redshiftViews(t))
}

// BLUE colors of the roles REDSHIFT draws with its few colors.
const (
	blueViolet = "124;92;255"
	bluePink   = "255;61;129"
	blueCyan   = "92;225;255"
)

// blueModels are the views of TestBlueLeavesNoRedshiftColor, drawn under
// BLUE.
func blueModels(t *testing.T) map[string]string {
	t.Helper()
	m := themedView(t)
	models := map[string]Model{"view": m}
	models["help"], _ = press(t, m, "?")
	models["settings"], _ = press(t, m, "s")
	models["expanded"], _ = press(t, m, "f")
	c := newClock()
	models["no signal"] = forceBurst(t, fxModel(t, c), c, true)
	useTheme(t, "BLUE")
	blue, _ := themeNamed("BLUE")
	frames := map[string]string{}
	for name, m := range models {
		frames[name] = m.setTheme(blue).View().Content
	}
	return frames
}

// BLUE is REDSHIFT recolored color for color: the same roles share a
// color in both, so it draws with as many colors as REDSHIFT, each one
// of gentleman-blue.
func TestBlueUsesAsManyColorsAsRedshift(t *testing.T) {
	night, blue := themeRoles(themes[0]), themeRoles(themes[1])
	if len(night) != len(blue) {
		t.Fatalf("%d roles in REDSHIFT, %d in BLUE", len(night), len(blue))
	}
	pairs := map[string]string{}
	for i, c := range night {
		if b, ok := pairs[c]; ok && b != blue[i] {
			t.Fatalf("role %d: REDSHIFT's %s is %s here but %s elsewhere in BLUE", i, c, blue[i], b)
		}
		pairs[c] = blue[i]
	}
	used := map[string]bool{}
	for _, b := range pairs {
		if used[b] {
			t.Fatalf("BLUE merges two REDSHIFT colors into %s", b)
		}
		used[b] = true
	}
	want := map[string]string{
		"#FF5F57": "#347AFF", "#E8554E": "#2A62CC", "#5A1E1E": "#1C2C54", "#9A3B37": "#4A5578",
		"#5EF6FF": "#5CE1FF", "#FCEE0A": "#7C5CFF", "#0A0A0A": "#05070F", "#0E2A2F": "#10182E",
	}
	if len(pairs) != len(want) {
		t.Fatalf("REDSHIFT draws %d colors; want %d", len(pairs), len(want))
	}
	for n, b := range want {
		if pairs[n] != b {
			t.Errorf("REDSHIFT %s is %s in BLUE; want %s", n, pairs[n], b)
		}
	}
}

// themeRoles lists every color of t, role by role.
func themeRoles(t theme) []string {
	return append([]string{
		t.label, t.text, t.number, t.frame, t.dim, t.muted,
		t.hi, t.accent, t.heading, t.warn, t.onAir, t.favorite, t.focus, t.ok,
		t.fill, t.ink, t.selectBg, t.alert, t.alertStatic, t.rainTip, t.rainBright, t.rainBody,
		t.logo, t.logoAlt,
	}, t.noise[:]...)
}

// The NO SIGNAL sign, its static and the burst noise follow the theme:
// nothing of them reads red under BLUE.
func TestBlueNoSignalReadsBlue(t *testing.T) {
	frame := blueModels(t)["no signal"]
	for _, code := range []string{"255;95;87", bluePink} {
		if strings.Contains(frame, code) {
			t.Errorf("NO SIGNAL under BLUE draws the red %s", code)
		}
	}
}

// MATRIX is REDSHIFT recolored in greens: as many colors, each one a
// green (its green channel above red and blue), and SETTINGS offers it.
func TestMatrixIsAllGreen(t *testing.T) {
	m, ok := themeNamed("MATRIX")
	if !ok {
		t.Fatal("no MATRIX theme")
	}
	night, green := themeRoles(themes[0]), themeRoles(m)
	pairs := map[string]string{}
	for i, c := range night {
		if b, seen := pairs[c]; seen && b != green[i] {
			t.Fatalf("role %d: REDSHIFT's %s maps to both %s and %s", i, c, b, green[i])
		}
		pairs[c] = green[i]
	}
	used := map[string]bool{}
	for _, g := range pairs {
		if used[g] {
			t.Fatalf("MATRIX merges two REDSHIFT colors into %s", g)
		}
		used[g] = true
		var r, gr, b int
		if _, err := fmt.Sscanf(g, "#%02x%02x%02x", &r, &gr, &b); err != nil {
			t.Fatalf("color %q: %v", g, err)
		}
		if gr < r || gr < b {
			t.Fatalf("MATRIX color %s is not green", g)
		}
	}
}
