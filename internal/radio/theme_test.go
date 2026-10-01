package radio

import (
	"fmt"
	"regexp"
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

// The NIGHT CITY frame, escape codes included, was recorded before
// themes existed: the default theme draws exactly what the app drew.
func TestNightCityDrawsTheOriginalColors(t *testing.T) {
	assertGolden(t, "view_night_city_ansi_80x24.golden", themedView(t).View().Content)
}

// useTheme applies the theme named name for the rest of the test, and
// NIGHT CITY again after it: the styles are package state. A new Model
// applies NIGHT CITY, so the models are made first.
func useTheme(t *testing.T, name string) {
	t.Helper()
	th, ok := themeNamed(name)
	if !ok {
		t.Fatalf("no theme %q", name)
	}
	applyTheme(th)
	t.Cleanup(func() { applyTheme(themes[0]) })
}

// nightCityCodes are the escape code colors of the NIGHT CITY palette.
var nightCityCodes = []string{
	"255;95;87", "232;85;78", "94;246;255", "252;238;10",
	"90;30;30", "154;59;55", "14;42;47", "10;10;10",
}

func TestThemesStartWithNightCityThenBlue(t *testing.T) {
	var names []string
	for _, th := range themes {
		names = append(names, th.name)
	}
	if strings.Join(names, ",") != "NIGHT CITY,BLUE" {
		t.Fatalf("themes %v; want NIGHT CITY then BLUE", names)
	}
	if th, ok := themeNamed("blue"); !ok || th.name != "BLUE" {
		t.Fatalf("themeNamed(blue) = %v, %v; want BLUE, case aside", th.name, ok)
	}
	if _, ok := themeNamed("neon"); ok {
		t.Fatal("themeNamed(neon) found a theme")
	}
}

func TestBlueRecolorsEveryStyle(t *testing.T) {
	useTheme(t, "BLUE")
	const primary, yellow = "52;122;255", "255;210;61"
	for name, got := range map[string]string{
		"stLabel":        stLabel.Render("x"),
		"stLabelBold":    stLabelBold.Render("x"),
		"inkBright":      vizInks[inkBright].pre,
		"noise":          noiseStyles[0].Render("x"),
		"stAlertStatic":  stAlertStatic.Render("x"),
		"stText":         stText.Render("x") + "/" + blueForeground,
		"stNumber":       stNumber.Render("x") + "/" + blueOrange,
		"stHi":           stHi.Render("x") + "/" + blueCyan,
		"stAccent":       stAccent.Render("x") + "/" + blueCyan,
		"stOnAir":        stOnAir.Render("x") + "/" + blueCyan,
		"stHeading":      stHeading.Render("x") + "/" + blueViolet,
		"stButtonOn":     stButtonOn.Render("x") + "/" + blueViolet,
		"stFillEdge":     stFillEdge.Render("x") + "/" + blueViolet,
		"stAlertSign":    stAlertSign.Render("x") + "/" + blueViolet,
		"inkBody":        vizInks[inkBody].pre + "/" + blueViolet,
		"inkTip":         vizInks[inkTip].pre + "/" + blueCyan,
		"stWarn":         stWarn.Render("x") + "/" + yellow,
		"stFocus":        stFocus.Render("x") + "/" + yellow,
		"stFav":          stFav.Render("x") + "/" + bluePink,
		"stOK":           stOK.Render("x") + "/" + blueGreen,
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
	if c := inputStyles().Cursor.Color; c == nil || fmt.Sprint(c.RGBA()) != fmt.Sprint(lipgloss.Color("#FFD23D").RGBA()) {
		t.Errorf("input cursor color %v; want the BLUE focus yellow", c)
	}
}

func TestBlueLeavesNoNightCityColor(t *testing.T) {
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
		for _, code := range nightCityCodes {
			if strings.Contains(frame, code) {
				t.Errorf("%s under BLUE draws the NIGHT CITY color %s", name, code)
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

// nightCityViews are the frames of the views beyond the main one, escape
// codes included, under the default theme, joined in name order.
func nightCityViews(t *testing.T) string {
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

// The NIGHT CITY frames of the other views, recorded before the finer
// theme roles existed: splitting the roles changed none of their bytes.
func TestNightCityKeepsItsColorsInEveryView(t *testing.T) {
	assertGolden(t, "night_city_ansi_views_80x24.golden", nightCityViews(t))
}

// BLUE colors of the roles NIGHT CITY draws with its few colors.
const (
	blueForeground = "219;233;255"
	blueViolet     = "124;92;255"
	blueOrange     = "255;159;28"
	blueGreen      = "77;255;136"
	bluePink       = "255;61;129"
	blueCyan       = "92;225;255"
)

// blueModels are the views of TestBlueLeavesNoNightCityColor, drawn under
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

// BLUE draws the rest of the gentleman-blue palette through the finer
// roles: section headings in violet, times and percentages in orange,
// AUTH OK and the signal bars in green, names in the foreground.
func TestBlueDrawsItsFinerRoles(t *testing.T) {
	frames := blueModels(t)
	for _, tt := range []struct{ view, role, code, text string }{
		{"view", "times", blueOrange, "01:23"},
		{"view", "clock", blueOrange, `\d\d:\d\d:\d\d`},
		{"view", "AUTH OK", blueGreen, "AUTH OK"},
		{"view", "signal bars", blueGreen, "▂▄▆█"},
		{"view", "station names", blueForeground, ""},
		{"help", "group headings", blueViolet, "▞ [A-Z]"},
		{"settings", "THEMES heading", blueViolet, "▞ THEMES"},
		{"expanded", "times", blueOrange, "01:23"},
		{"no signal", "the sign", blueViolet, "  N O   S I G N A L"},
	} {
		frame := frames[tt.view]
		want := regexp.QuoteMeta(tt.code+"m") + tt.text
		if !regexp.MustCompile(want).MatchString(frame) {
			t.Errorf("%s: %s not drawn in %s (%q)", tt.view, tt.role, tt.code, want)
		}
	}
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
