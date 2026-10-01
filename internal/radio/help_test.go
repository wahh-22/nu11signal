package radio

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// keyConstants are the key names keys.go declares (every const named
// key… with a string value), read from its source so a new key cannot
// be left out of the overlay unnoticed.
func keyConstants(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "keys.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "key") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				out[name.Name] = v
			}
		}
	}
	if len(out) < 20 {
		t.Fatalf("read only %d key constants from keys.go: %v", len(out), out)
	}
	return out
}

func TestTheHelpListsEveryKey(t *testing.T) {
	var listed []string
	for _, g := range helpGroups {
		for _, e := range g.entries {
			listed = append(listed, e.keys...)
		}
	}
	for name, k := range keyConstants(t) {
		if !slices.Contains(listed, k) {
			t.Errorf("%s (%q) is not in the KEYS overlay", name, k)
		}
	}
	want := []string{"PLAYBACK", "NAVIGATION", "VIEW", "SEARCH", "APP"}
	var got []string
	for _, g := range helpGroups {
		got = append(got, g.name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("groups %v; want %v", got, want)
	}
}

func TestQuestionMarkOpensAndClosesTheHelp(t *testing.T) {
	for _, closer := range []string{"?", "esc"} {
		m := playingModel(t, playbacktest.New())
		m, _ = press(t, m, "?")
		if !m.help {
			t.Fatal("? did not open the help")
		}
		view := plain(m)
		for _, want := range []string{"KEYS", "PLAYBACK", "NAVIGATION", "PLAY / PAUSE"} {
			if !strings.Contains(view, want) {
				t.Fatalf("help overlay lacks %q:\n%s", want, view)
			}
		}
		m, _ = press(t, m, closer)
		if m.help || strings.Contains(plain(m), "NAVIGATION") {
			t.Fatalf("%s did not close the help:\n%s", closer, plain(m))
		}
		if len(m.stack) != 1 {
			t.Fatalf("%s closing the help also popped the view: stack %v", closer, stackKinds(m))
		}
	}
}

func TestTheHelpIgnoresOtherKeysButCtrlC(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	m, _ = press(t, m, "?")
	before := len(f.Calls())
	m, cmd := press(t, m, "n", "space", "down", "enter", "q")
	if cmd != nil {
		t.Fatalf("a key under the help sent a command")
	}
	if !m.help || len(m.stack) != 1 || len(f.Calls()) != before {
		t.Fatalf("keys acted under the help: help %v stack %v calls %v", m.help, stackKinds(m), f.Calls()[before:])
	}
	m, cmd = press(t, m, "ctrl+c")
	if cmd != nil || !m.quitAsk {
		t.Fatal("ctrl+c did not ask to quit under the help")
	}
	_, cmd = press(t, m, "y")
	assertQuits(t, f, cmd)
}

func TestQuestionMarkIsTypedWhereTextIsTyped(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, "/", "?")
	if m.help || m.input.Value() != "?" {
		t.Fatalf("on SEARCH: help %v input %q; want ? typed", m.help, m.input.Value())
	}
	if hints := hintsOf(m); strings.Contains(hints, "[?]") {
		t.Fatalf("SEARCH footer %q hints [?] where ? is typed", hints)
	}

	m = loaded(t, playbacktest.New(), newClock())
	m.setStationCursor(-1) // + NEW PLAYLIST
	m, _ = press(t, m, "enter", "?")
	if m.editor.mode != editName || m.help || m.nameInput.Value() != "?" {
		t.Fatalf("in the name: mode %v help %v name %q; want ? typed", m.editor.mode, m.help, m.nameInput.Value())
	}
	if hints := hintsOf(m); strings.Contains(hints, "[?]") {
		t.Fatalf("name footer %q hints [?] where ? is typed", hints)
	}
}

func TestQuestionMarkOpensTheHelpFromThePlayerAndTheTabs(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	m, _ = press(t, m, "right", "?")
	if !m.help {
		t.Fatal("? on the player did not open the help")
	}
	m, _ = press(t, m, "esc")
	if m.focus != areaPlayer {
		t.Fatalf("focus %v after closing; want the player kept", m.focus)
	}
	m, _ = press(t, m, "esc", "up", "up", "?")
	if !m.help || m.focus != areaTabs {
		t.Fatalf("? on the tabs: help %v focus %v", m.help, m.focus)
	}
}

func TestEveryFooterHintsTheKeysBeforeQuit(t *testing.T) {
	sets := map[string][]hint{
		"player": playerHints, "artist": artistHints, "results": resultsHints,
		"track": trackHints, "picker": pickerHints,
		"tabs": tabsFocusHints(false), "playerFocus": playerFocusHints(false, false),
	}
	for name, hs := range sets {
		n := len(hs)
		if n < 2 || hs[n-2] != (hint{"?", "KEYS"}) || hs[n-1].label != "QUIT" {
			t.Errorf("%s hints end %v; want [?] KEYS then QUIT", name, hs[max(n-2, 0):])
		}
	}
	typing := map[string][]hint{
		"name": nameHints, "search": searchHints, "searchSong": searchSongHints, "recent": recentHints,
		"tabs typing": tabsFocusHints(true), "playerFocus typing": playerFocusHints(false, true),
	}
	for name, hs := range typing {
		if slices.ContainsFunc(hs, func(h hint) bool { return h.key == "?" }) {
			t.Errorf("%s hints [?] where ? is typed: %v", name, hs)
		}
	}
}

func TestNarrowFootersKeepTheKeysAndQuit(t *testing.T) {
	for _, w := range []int{20, 30, 40, 59, 80} {
		m := playingModel(t, playbacktest.New())
		m, _ = step(t, m, tea.WindowSizeMsg{Width: w, Height: 24})
		got := hintsOf(m)
		if !strings.Contains(got, "[?] KEYS") || !strings.HasSuffix(strings.TrimRight(got, " "), "[Q] QUIT") {
			t.Errorf("at %d cells footer %q; want [?] KEYS and [Q] QUIT", w, got)
		}
		if ansi.StringWidth(got) > w {
			t.Errorf("at %d cells footer %q overflows", w, got)
		}
	}
}

func TestHelpGolden80x24(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	for range 12 {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	m, _ = press(t, m, "?")
	assertGolden(t, "help_80x24.golden", ansi.Strip(m.View().Content))
}

func TestHelpFitsEverySize(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{20, 5}, {40, 10}, {59, 15}, {60, 16}, {80, 24}, {160, 50}} {
		m := playingModel(t, playbacktest.New())
		m, _ = step(t, m, tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
		m, _ = press(t, m, "?")
		lines := strings.Split(plain(m), "\n")
		if len(lines) > sz.h {
			t.Errorf("%dx%d: %d lines", sz.w, sz.h, len(lines))
		}
		for i, l := range lines {
			if ansi.StringWidth(l) > sz.w {
				t.Errorf("%dx%d: line %d is %d cells: %q", sz.w, sz.h, i, ansi.StringWidth(l), l)
			}
		}
	}
}
