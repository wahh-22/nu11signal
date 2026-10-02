package radio

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/config"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// savingSource is a config.Source that records what it is asked to save.
type savingSource struct {
	cfg     config.Config
	saved   []config.Config
	saveErr error
}

func (s *savingSource) Load() (config.Config, error) { return s.cfg, nil }

func (s *savingSource) Save(c config.Config) error {
	s.saved = append(s.saved, c)
	return s.saveErr
}

// settingsModel is a playing model that read src at startup; it leaves
// NIGHT CITY applied after the test.
func settingsModel(t *testing.T, src config.Source) Model {
	t.Helper()
	t.Cleanup(func() { applyTheme(themes[0]) })
	m := withConfig(t, playbacktest.New(), newClock(), src)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	return m
}

func TestSOpensAndClosesTheSettings(t *testing.T) {
	for _, closer := range []string{"s", "esc"} {
		m := settingsModel(t, &savingSource{})
		m, _ = press(t, m, "s")
		if !m.settings {
			t.Fatal("s did not open the settings")
		}
		view := plain(m)
		for _, want := range []string{"SETTINGS", "THEMES", "NIGHT CITY", "BLUE"} {
			if !strings.Contains(view, want) {
				t.Fatalf("settings overlay lacks %q:\n%s", want, view)
			}
		}
		m, _ = press(t, m, closer)
		if m.settings || strings.Contains(plain(m), "THEMES") {
			t.Fatalf("%s did not close the settings:\n%s", closer, plain(m))
		}
		if len(m.stack) != 1 {
			t.Fatalf("%s closing the settings also popped the view: stack %v", closer, stackKinds(m))
		}
	}
}

func TestTheSettingsPickATheme(t *testing.T) {
	src := &savingSource{cfg: config.Config{Visualizer: "rain"}}
	m := settingsModel(t, src)
	m, _ = press(t, m, "s", "down")
	if m.theme != "NIGHT CITY" {
		t.Fatalf("moving applied %q; want enter to apply", m.theme)
	}
	m, cmd := press(t, m, "enter")
	if m.theme != "BLUE" || !strings.Contains(stLabel.Render("x"), "52;122;255") {
		t.Fatalf("enter on BLUE: theme %q, stLabel %q", m.theme, stLabel.Render("x"))
	}
	if !m.settings {
		t.Fatal("enter closed the settings; want them open to compare")
	}
	if !strings.Contains(plain(m), "◉ BLUE") {
		t.Fatalf("the active theme is not marked:\n%s", plain(m))
	}
	m, _ = step(t, m, run(t, cmd))
	if want := (config.Config{Visualizer: "rain", Theme: "BLUE"}); len(src.saved) != 1 || !reflect.DeepEqual(src.saved[0], want) {
		t.Fatalf("saved %+v; want one save of %+v", src.saved, want)
	}
	if m.status != "" {
		t.Fatalf("a saved theme reported %q", m.status)
	}
	m, _ = press(t, m, "up", "enter")
	if m.theme != "NIGHT CITY" || !strings.Contains(stLabel.Render("x"), "255;95;87") {
		t.Fatalf("back to NIGHT CITY: theme %q, stLabel %q", m.theme, stLabel.Render("x"))
	}
}

func TestSettingsSelectGentlemanThemes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		steps int
		ansi  string
	}{
		{"GENTLEMAN CUTE", 3, "240;149;200"},
		{"GENTLEMAN SEXY", 4, "244;56;136"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := &savingSource{}
			m := settingsModel(t, src)
			m, _ = press(t, m, "s")
			if !strings.Contains(plain(m), tc.name) {
				t.Fatalf("settings lacks %s", tc.name)
			}
			for range tc.steps {
				m, _ = press(t, m, "down")
			}
			var cmd tea.Cmd
			m, cmd = press(t, m, "enter")
			if m.theme != tc.name || !strings.Contains(stHi.Render("x"), tc.ansi) {
				t.Fatalf("selected %q, highlight %q", m.theme, stHi.Render("x"))
			}
			m, _ = step(t, m, run(t, cmd))
			if len(src.saved) != 1 || src.saved[0].Theme != tc.name {
				t.Fatalf("saved %+v", src.saved)
			}
			m = settingsModel(t, &savingSource{cfg: config.Config{Theme: strings.ToLower(tc.name)}})
			if m.theme != tc.name {
				t.Fatalf("startup theme %q", m.theme)
			}
		})
	}
}

func TestAFailedThemeSaveSaysSo(t *testing.T) {
	src := &savingSource{saveErr: errors.New("disk full")}
	m := settingsModel(t, src)
	m, cmd := press(t, m, "s", "down", "enter")
	m, _ = step(t, m, run(t, cmd))
	if !strings.Contains(m.status, "SETTINGS NOT SAVED") || !strings.Contains(m.status, "disk full") {
		t.Fatalf("status %q; want the failed save reported", m.status)
	}
	if m.theme != "BLUE" {
		t.Fatalf("a failed save reverted the theme to %q", m.theme)
	}
}

func TestTheSettingsIgnoreOtherKeysButCtrlC(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	t.Cleanup(func() { applyTheme(themes[0]) })
	m, _ = press(t, m, "s")
	before := len(f.Calls())
	m, cmd := press(t, m, "n", "space", "q", "?", "/", "f")
	if cmd != nil {
		t.Fatal("a key under the settings sent a command")
	}
	if !m.settings || m.help || m.expanded || len(m.stack) != 1 || len(f.Calls()) != before {
		t.Fatalf("keys acted under the settings: settings %v help %v stack %v calls %v", m.settings, m.help, stackKinds(m), f.Calls()[before:])
	}
	m, cmd = press(t, m, "ctrl+c")
	if cmd != nil || !m.quitAsk {
		t.Fatal("ctrl+c did not ask to quit under the settings")
	}
	m, cmd = press(t, m, "y")
	assertQuits(t, f, m, cmd)
}

func TestSIsTypedWhereTextIsTyped(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, "/", "s")
	if m.settings || m.input.Value() != "s" {
		t.Fatalf("on SEARCH: settings %v input %q; want s typed", m.settings, m.input.Value())
	}
	if hints := hintsOf(m); strings.Contains(hints, "[S]") {
		t.Fatalf("SEARCH footer %q hints [S] where s is typed", hints)
	}
	m = loaded(t, playbacktest.New(), newClock())
	m.setStationCursor(-1) // + NEW PLAYLIST
	m, _ = press(t, m, "enter", "s")
	if m.editor.mode != editName || m.settings || m.nameInput.Value() != "s" {
		t.Fatalf("in the name: mode %v settings %v name %q; want s typed", m.editor.mode, m.settings, m.nameInput.Value())
	}
}

func TestTheHelpAndTheSettingsDoNotStack(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	m, _ = press(t, m, "?", "s")
	if !m.help || m.settings {
		t.Fatalf("s under the help: help %v settings %v; want the help kept alone", m.help, m.settings)
	}
}

func TestAClickOnAThemeAppliesIt(t *testing.T) {
	m := settingsModel(t, &savingSource{})
	m, _ = press(t, m, "s")
	lines := strings.Split(plain(m), "\n")
	y := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "BLUE") })
	if y < 0 {
		t.Fatalf("no BLUE row:\n%s", plain(m))
	}
	x := strings.Index(lines[y], "BLUE")
	m, cmd := pressAt(t, m, ansi.StringWidth(lines[y][:x]), y)
	if m.theme != "BLUE" || cmd == nil {
		t.Fatalf("click on BLUE: theme %q, save %v", m.theme, cmd != nil)
	}
	// A click off the rows closes the settings, as it closes the help.
	m, _ = pressAt(t, m, 0, 0)
	if m.settings {
		t.Fatal("a click off the rows left the settings open")
	}
}

func TestStartupAppliesTheSavedTheme(t *testing.T) {
	m := settingsModel(t, &savingSource{cfg: config.Config{Theme: "blue"}})
	if m.theme != "BLUE" || !strings.Contains(stLabel.Render("x"), "52;122;255") {
		t.Fatalf("saved blue: theme %q, stLabel %q", m.theme, stLabel.Render("x"))
	}
	applyTheme(themes[1])
	m = settingsModel(t, &savingSource{cfg: config.Config{Theme: "neon"}})
	if m.theme != "NIGHT CITY" || !strings.Contains(stLabel.Render("x"), "255;95;87") {
		t.Fatalf("unknown theme: theme %q, stLabel %q; want NIGHT CITY", m.theme, stLabel.Render("x"))
	}
	if m.status != "" {
		t.Fatalf("an unknown theme reported %q; want silence", m.status)
	}
}

func TestAThemeChosenBeforeTheSettingsLoadIsKept(t *testing.T) {
	src := &savingSource{cfg: config.Config{Theme: "NIGHT CITY"}}
	t.Cleanup(func() { applyTheme(themes[0]) })
	m := New(playbacktest.New(), Options{SkipBoot: true, Now: newClock().now, Seed: 2077, Config: src})
	load := m.loadConfigCmd()
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = press(t, m, "s", "down", "enter")
	m, _ = step(t, m, run(t, load))
	if m.theme != "BLUE" {
		t.Fatalf("the late settings file reverted the theme to %q", m.theme)
	}
}

func TestEveryFooterHintsTheSettingsBeforeTheKeys(t *testing.T) {
	sets := map[string][]hint{
		"player": playerHints, "artist": artistHints, "results": resultsHints,
		"track": trackHints, "picker": pickerHints,
		"tabs": tabsFocusHints(false), "playerFocus": playerFocusHints(false, false),
	}
	for name, hs := range sets {
		n := len(hs)
		if n < 3 || hs[n-3] != settingsHint {
			t.Errorf("%s hints end %v; want [S] SETTINGS before [?] KEYS", name, hs[max(n-3, 0):])
		}
	}
	// Where the footer has room, it shows; where it does not, it goes
	// before the keys and quit.
	m := playingModel(t, playbacktest.New())
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 200, Height: 30})
	if hints := hintsOf(m); !strings.Contains(hints, "[S] SETTINGS") {
		t.Fatalf("a 200-cell footer %q lacks [S] SETTINGS", hints)
	}
}

func TestSettingsGolden80x24(t *testing.T) {
	m := themedView(t)
	m, _ = press(t, m, "s")
	assertGolden(t, "settings_80x24.golden", ansi.Strip(m.View().Content))
}

func TestAnOlderThemeSaveFinishingLastIsDropped(t *testing.T) {
	// Saves run concurrently: the file must end with the latest choice
	// even when an older save runs after it.
	src := &savingSource{}
	m := settingsModel(t, src)
	m, older := press(t, m, "s", "down", "enter")
	m, newer := press(t, m, "up", "enter")
	m, _ = step(t, m, run(t, newer))
	m, _ = step(t, m, run(t, older))
	if n := len(src.saved); n != 1 || src.saved[0].Theme != "NIGHT CITY" {
		t.Fatalf("saved %+v; want only the latest choice, NIGHT CITY", src.saved)
	}
	if m.status != "" {
		t.Fatalf("a dropped save reported %q", m.status)
	}
}
