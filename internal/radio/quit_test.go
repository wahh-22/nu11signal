package radio

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// quitQuestion is the question the quit modal asks.
const quitQuestion = "QUIT NU11SIGNAL?"

// assertAsking fails unless m shows the quit modal and cmd quits nothing.
func assertAsking(t *testing.T, m Model, cmd tea.Cmd) {
	t.Helper()
	if !m.quitAsk {
		t.Fatalf("the quit modal is not open:\n%s", plain(m))
	}
	if cmd != nil {
		t.Fatalf("opening the quit modal sent a command (%T)", run(t, cmd))
	}
	view := plain(m)
	for _, want := range []string{quitQuestion, "[ Y QUIT ]", "[ N STAY ]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the quit modal lacks %q:\n%s", want, view)
		}
	}
}

func TestQuitAsksFirst(t *testing.T) {
	for _, k := range []string{keyQuit, keyCtrlC} {
		t.Run(k, func(t *testing.T) {
			f := playbacktest.New()
			m := playingModel(t, f)
			before := len(f.Calls())
			m, cmd := press(t, m, k)
			assertAsking(t, m, cmd)
			if f.Closed() || len(f.Calls()) != before {
				t.Fatalf("opening the modal touched the player: closed %v calls %v", f.Closed(), f.Calls()[before:])
			}
		})
	}
}

// y, enter, and q or ctrl+c pressed again quit from the modal, through
// the shutdown that closes the player (see shutdown.go).
func TestTheQuitModalQuits(t *testing.T) {
	for _, open := range []string{keyQuit, keyCtrlC} {
		for _, confirm := range []string{"y", "Y", keyEnter, keyQuit, keyCtrlC} {
			t.Run(open+" then "+confirm, func(t *testing.T) {
				f := playbacktest.New()
				m := playingModel(t, f)
				m, _ = press(t, m, open)
				m, cmd := press(t, m, confirm)
				assertQuits(t, f, m, cmd)
			})
		}
	}
}

func TestTheQuitModalStays(t *testing.T) {
	for _, k := range []string{"n", "N", keyEsc} {
		t.Run(k, func(t *testing.T) {
			f := playbacktest.New()
			m := playingModel(t, f)
			view := plain(m)
			m, _ = press(t, m, keyQuit)
			m, cmd := press(t, m, k)
			if m.quitAsk || cmd != nil || f.Closed() {
				t.Fatalf("%q: modal open %v, command %v, closed %v; want it closed, staying", k, m.quitAsk, cmd != nil, f.Closed())
			}
			if got := plain(m); got != view {
				t.Fatalf("%q left a different frame:\n%s\nwant:\n%s", k, got, view)
			}
		})
	}
}

// Every other key is ignored while the modal asks: nothing reaches the
// player or the view behind it.
func TestTheQuitModalIgnoresOtherKeys(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	m, _ = press(t, m, keyQuit)
	before := len(f.Calls())
	fx := m.fx.on
	for _, k := range []string{"space", "p", "down", "up", "right", "f", "ctrl+f", "?", "s", "/", "x", "l", "a", "j", "shift+right", "tab"} {
		next, cmd := press(t, m, k)
		if cmd != nil {
			t.Fatalf("%q sent a command under the quit modal", k)
		}
		if !next.quitAsk || next.help || next.settings || next.expanded || next.fx.on != fx ||
			next.focus != m.focus || next.cursor() != m.cursor() || !slices.Equal(stackKinds(next), stackKinds(m)) {
			t.Fatalf("%q acted under the quit modal", k)
		}
		m = next
	}
	if len(f.Calls()) != before {
		t.Fatalf("keys reached the player under the quit modal: %v", f.Calls()[before:])
	}
}

// Where q is typed, it stays typed; ctrl+c alone opens the modal there.
func TestQIsTypedWhereTextIsTyped(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, keySearch, keyQuit)
	if m.quitAsk || m.input.Value() != "q" {
		t.Fatalf("on SEARCH: modal %v input %q; want q typed", m.quitAsk, m.input.Value())
	}
	m, cmd := press(t, m, keyCtrlC)
	assertAsking(t, m, cmd)
	m, _ = press(t, m, keyEsc)
	if m.quitAsk || m.input.Value() != "q" || m.top().kind != viewSearch {
		t.Fatalf("esc closing the modal acted on SEARCH: modal %v input %q stack %v", m.quitAsk, m.input.Value(), stackKinds(m))
	}
}

// The modal draws over every screen, the overlays, the screen while
// access links and the auth error screen included (over the boot splash
// q skips the boot first, see TestAKeyOrAClickSkipsTheBoot), and closing it gives that screen
// back as it was.
func TestTheQuitModalOpensOverEveryView(t *testing.T) {
	denied := func(t *testing.T) Model {
		f := playbacktest.New()
		f.AuthStatus = playback.AuthDenied
		m := newModel(t, f, newClock())
		m, _ = step(t, m, run(t, m.authorizeCmd()))
		if m.auth != authFailed {
			t.Fatalf("auth %v; want the error screen", m.auth)
		}
		return m
	}
	for _, tt := range []struct {
		name  string
		model func(t *testing.T) Model
		keys  []string
		under string
	}{
		{"full", func(t *testing.T) Model { return playingModel(t, playbacktest.New()) }, []string{keyQuit}, "▮ PLAYLISTS"},
		{"compact", func(t *testing.T) Model {
			m, _ := step(t, playingModel(t, playbacktest.New()), tea.WindowSizeMsg{Width: 50, Height: 20})
			return m
		}, []string{keyQuit}, "NU11SIGNAL"},
		{"expanded", func(t *testing.T) Model { return playingModel(t, playbacktest.New()) }, []string{keyExpandAlt, keyQuit}, "NOW PLAYING"},
		{"player focus", func(t *testing.T) Model { return playingModel(t, playbacktest.New()) }, []string{keyRight, keyQuit}, "NOW PLAYING"},
		{"tabs focus", func(t *testing.T) Model { return playingModel(t, playbacktest.New()) }, []string{keyUp, keyUp, keyQuit}, "PLAYLISTS"},
		{"keys", func(t *testing.T) Model { return playingModel(t, playbacktest.New()) }, []string{keyHelp, keyCtrlC}, "▮ KEYS"},
		{"settings", func(t *testing.T) Model { return playingModel(t, playbacktest.New()) }, []string{keySettings, keyCtrlC}, "▮ SETTINGS"},
		{"linking", func(t *testing.T) Model { return newModel(t, playbacktest.New(), newClock()) }, []string{keyQuit}, "LINKING"},
		{"auth error", denied, []string{keyQuit}, "AUTH // ERROR"},
		{"auth error esc", denied, []string{keyEsc}, "AUTH // ERROR"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model(t)
			m, _ = press(t, m, tt.keys[:len(tt.keys)-1]...)
			view := plain(m)
			m, cmd := press(t, m, tt.keys[len(tt.keys)-1])
			assertAsking(t, m, cmd)
			if !strings.Contains(plain(m), tt.under) {
				t.Fatalf("the modal hid %q behind it:\n%s", tt.under, plain(m))
			}
			for _, l := range strings.Split(plain(m), "\n") {
				if ansi.StringWidth(l) > m.width {
					t.Fatalf("line wider than %d: %q", m.width, l)
				}
			}
			m, _ = press(t, m, "n")
			if m.quitAsk || plain(m) != view {
				t.Fatalf("closing the modal did not give the screen back:\n%s\nwant:\n%s", plain(m), view)
			}
		})
	}
}

// On the tiny layout there is no room for the panel: the question
// replaces the status tag, and the keys work the same.
func TestTheQuitModalOnTheTinyLayout(t *testing.T) {
	f := playbacktest.New()
	m, _ := step(t, playingModel(t, f), tea.WindowSizeMsg{Width: 18, Height: 4})
	m, cmd := press(t, m, keyQuit)
	if !m.quitAsk || cmd != nil {
		t.Fatalf("q on the tiny layout: modal %v command %v", m.quitAsk, cmd != nil)
	}
	if !strings.Contains(plain(m), "QUIT? Y/N") {
		t.Fatalf("tiny layout does not ask:\n%s", plain(m))
	}
	m, cmd = press(t, m, "y")
	assertQuits(t, f, m, cmd)
}

func TestAClickOutsideTheQuitModalClosesIt(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	m, _ = press(t, m, keyQuit)
	panelZone := zoneOf(t, m, zoneQuitPanel)
	// Inside the panel, off its buttons: nothing.
	m, cmd := step(t, m, tea.MouseClickMsg{X: panelZone.x, Y: panelZone.y, Button: tea.MouseLeft})
	if !m.quitAsk || cmd != nil {
		t.Fatalf("a click on the panel: modal %v command %v; want still asking", m.quitAsk, cmd != nil)
	}
	// The wheel and other buttons: nothing.
	m, cmd = step(t, m, tea.MouseWheelMsg{X: 0, Y: 5, Button: tea.MouseWheelDown})
	if !m.quitAsk || cmd != nil {
		t.Fatal("the wheel acted under the quit modal")
	}
	// The modal hides every zone behind it.
	_, zs := m.layout()
	for _, z := range zs {
		if z.id != zoneQuitPanel && z.id != zoneQuitYes && z.id != zoneQuitNo {
			t.Fatalf("zone %v is clickable under the quit modal", z)
		}
	}
	// On a row of the list behind it: closes, the row untouched.
	before := len(f.Calls())
	m, cmd = step(t, m, tea.MouseClickMsg{X: 4, Y: 4, Button: tea.MouseLeft})
	if m.quitAsk || cmd != nil || len(f.Calls()) != before || len(m.stack) != 1 {
		t.Fatalf("a click outside: modal %v command %v calls %v stack %v; want only closed", m.quitAsk, cmd != nil, f.Calls()[before:], stackKinds(m))
	}
}

// QUIT and STAY are HUD bracket buttons like the player's transport:
// QUIT, enter's action, filled as the primary one, STAY in cyan brackets;
// both are clickable.
func TestTheQuitModalButtons(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	m, _ = press(t, m, keyQuit)
	yes, no := zoneOf(t, m, zoneQuitYes), zoneOf(t, m, zoneQuitNo)
	if got := textAt(m, yes); got != "[ Y QUIT ]" {
		t.Fatalf("QUIT button reads %q", got)
	}
	if got := textAt(m, no); got != "[ N STAY ]" {
		t.Fatalf("STAY button reads %q", got)
	}
	if yes.y != no.y || no.x <= yes.x+yes.w {
		t.Fatalf("QUIT %v and STAY %v are not side by side", yes, no)
	}
	screen := m.render()
	if want := stButtonOn.Render("[ Y QUIT ]"); !strings.Contains(screen, want) {
		t.Fatalf("QUIT is not the filled primary button %q:\n%s", want, screen)
	}
	if want := stHi.Render("[ N STAY ]"); !strings.Contains(screen, want) {
		t.Fatalf("STAY is not a cyan bracket button %q:\n%s", want, screen)
	}
	stay, cmd := click(t, m, zoneQuitNo)
	if stay.quitAsk || cmd != nil || f.Closed() {
		t.Fatalf("STAY: modal %v command %v closed %v", stay.quitAsk, cmd != nil, f.Closed())
	}
	m, cmd = click(t, m, zoneQuitYes)
	assertQuits(t, f, m, cmd)
}

// Too narrow for both side by side, the buttons stack, still whole and
// clickable.
func TestTheQuitModalStacksItsButtonsWhenNarrow(t *testing.T) {
	m, _ := step(t, playingModel(t, playbacktest.New()), tea.WindowSizeMsg{Width: 22, Height: 20})
	m, _ = press(t, m, keyQuit)
	yes, no := zoneOf(t, m, zoneQuitYes), zoneOf(t, m, zoneQuitNo)
	if no.y <= yes.y || textAt(m, yes) != "[ Y QUIT ]" || textAt(m, no) != "[ N STAY ]" {
		t.Fatalf("narrow modal: QUIT %v %q, STAY %v %q; want stacked whole\n%s", yes, textAt(m, yes), no, textAt(m, no), plain(m))
	}
}

func TestTheFooterShowsTheQuitModalKeys(t *testing.T) {
	for _, keys := range [][]string{{keyQuit}, {keyHelp, keyCtrlC}, {keySettings, keyCtrlC}, {keySearch, keyCtrlC}} {
		m := playingModel(t, playbacktest.New())
		m, _ = press(t, m, keys...)
		if got := strings.TrimRight(hintsOf(m), " "); got != "[Y/ENTER] QUIT  [N/ESC] STAY" {
			t.Errorf("%v: footer %q; want the modal's keys", keys, got)
		}
	}
}

// The modal scrambles in like the overlays, only inside its frame, and
// closing it scrambles the screen back in.
func TestTheQuitModalIntros(t *testing.T) {
	c := newClock()
	m := introModel(t, c)
	seq := m.intro.seq
	m, _ = press(t, m, keyQuit)
	if m.intro.seq != seq+1 {
		t.Fatalf("opening the quit modal started %d intros; want 1", m.intro.seq-seq)
	}
	base, zs := m.baseLayout()
	scrambled := scrambledAt(base, first(m.layout()))
	if len(scrambled) == 0 {
		t.Fatal("the quit modal opened without an intro")
	}
	var rows []int
	for _, z := range zs {
		if z.id == zoneQuitPanel {
			rows = append(rows, z.y)
		}
	}
	for y := range scrambled {
		if len(rows) < 3 || y <= rows[0] || y >= rows[len(rows)-1] {
			t.Errorf("row %d scrambled; only the modal's inside (%v) may", y, rows)
		}
	}
	c.advance(introDur)
	seq = m.intro.seq
	m, _ = press(t, m, keyEsc)
	if m.intro.seq != seq+1 || len(scrambledAt(first(m.baseLayout()), first(m.layout()))) == 0 {
		t.Fatal("closing the quit modal did not intro the screen")
	}

	// Over SEARCH, typing: ctrl+c still intros the modal.
	c = newClock()
	m = introModel(t, c)
	m, _ = press(t, m, keySearch)
	c.advance(introDur)
	seq = m.intro.seq
	m, _ = press(t, m, keyCtrlC)
	if m.intro.seq != seq+1 {
		t.Fatal("ctrl+c over SEARCH opened the modal without an intro")
	}
}

func TestTheQuitModalOpensWithoutAnIntroWhenCalm(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, keyQuit)
	if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
		t.Fatal("the quit modal drew an intro with the effects off")
	}
}

func TestQuitGolden80x24(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	for range 12 {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	m, _ = press(t, m, keyQuit)
	assertGolden(t, "quit_80x24.golden", ansi.Strip(m.View().Content))
}
