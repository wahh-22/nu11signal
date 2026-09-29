package radio

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/history"
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// playingModel is the stations view at 80x24 with a song playing.
func playingModel(t *testing.T, f *playbacktest.Fake) Model {
	t.Helper()
	m := loaded(t, f, newClock())
	m, _ = step(t, m, stateMsg{state: playing(time.Minute, 3*time.Minute)})
	return m
}

// listPanelEdge is the width of the list panel as drawn: the cells up to
// and including the "┐" closing its top edge.
func listPanelEdge(t *testing.T, m Model) int {
	t.Helper()
	top := strings.Split(plain(m), "\n")[2]
	i := strings.Index(top, "┐")
	if i < 0 {
		t.Fatalf("no panel top edge in %q", top)
	}
	return ansi.StringWidth(top[:i]) + 1
}

func TestListPanelKeepsItsMaximumWidthInEveryView(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{60, 16}, {80, 24}, {120, 40}, {200, 50}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			f := playbacktest.New()
			r := &fakeRecents{terms: []string{"queen"}}
			views := map[string]func() Model{
				"stations": func() Model { return loadedWithRecents(t, f, r) },
				"search recent": func() Model {
					m, _ := press(t, loadedWithRecents(t, f, r), "/")
					return m
				},
				"search results": func() Model {
					f.SearchCatalogResult = catalog()
					return searchFor(t, loadedWithRecents(t, f, r), "daft")
				},
				"results page": func() Model { return openResults(t, f, r) },
			}
			want := listPanelWidthFor(sz.w)
			for name, open := range views {
				m, _ := step(t, open(), tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
				if got := listPanelEdge(t, m); got != want {
					t.Errorf("%s: list panel is %d cells; want %d", name, got, want)
				}
			}
			if right := sz.w - want - 1; right < playerMinWidth {
				t.Errorf("NOW PLAYING is %d cells; want at least %d", right, playerMinWidth)
			}
		})
	}
}

func TestSearchInputSpansTheListPanel(t *testing.T) {
	m, _ := press(t, loaded(t, playbacktest.New(), newClock()), "/")
	if got, want := m.input.Width(), listPanelWidthFor(80)-2-3; got != want {
		t.Fatalf("input width %d; want %d", got, want)
	}
}

func TestExpandKeysToggleTheFullWidthPlayer(t *testing.T) {
	for _, k := range []string{"f", "ctrl+f"} {
		t.Run(k, func(t *testing.T) {
			m := playingModel(t, playbacktest.New())
			m, _ = press(t, m, k)
			if !m.expanded || m.focus != focusPlayer {
				t.Fatalf("expanded %v focus %v; want expanded with the player focused", m.expanded, m.focus)
			}
			view := plain(m)
			top := strings.Split(view, "\n")[2]
			if !strings.Contains(top, "NOW PLAYING") || !strings.HasSuffix(top, "┐") || ansi.StringWidth(top) != 80 {
				t.Fatalf("NOW PLAYING does not span the screen: %q", top)
			}
			if strings.Contains(view, "NIGHT DRIVE") || !strings.Contains(view, "STATIONS") {
				t.Fatalf("list still shown, or the nav bar is gone:\n%s", view)
			}
			if !strings.Contains(textAt(m, zoneOf(t, m, zoneExpand)), "RESTORE") {
				t.Fatalf("the expand button does not offer RESTORE:\n%s", view)
			}
			m, _ = press(t, m, k)
			if m.expanded || m.focus != focusList || !strings.Contains(plain(m), "NIGHT DRIVE") {
				t.Fatalf("expanded %v focus %v; want the list back and focused:\n%s", m.expanded, m.focus, plain(m))
			}
		})
	}
}

func TestExpandButtonTogglesThePlayer(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	if got := textAt(m, zoneOf(t, m, zoneExpand)); !strings.Contains(got, "⤢") {
		t.Fatalf("expand zone covers %q", got)
	}
	m, _ = click(t, m, zoneExpand)
	if !m.expanded || m.focus != focusPlayer || m.control != ctlExpand {
		t.Fatalf("expanded %v focus %v control %v; want expanded, EXPAND focused", m.expanded, m.focus, m.control)
	}
	m, _ = click(t, m, zoneExpand)
	if m.expanded || m.focus != focusList {
		t.Fatalf("expanded %v focus %v; want restored with the list focused", m.expanded, m.focus)
	}
}

func TestExpandInSearchNeverTypes(t *testing.T) {
	m, _ := press(t, playingModel(t, playbacktest.New()), "/")
	m, _ = press(t, m, "f")
	if m.expanded || m.input.Value() != "f" {
		t.Fatalf("f in the input: expanded %v input %q; want it typed", m.expanded, m.input.Value())
	}
	m, _ = press(t, m, "ctrl+f")
	if !m.expanded || m.input.Value() != "f" {
		t.Fatalf("ctrl+f: expanded %v input %q; want expanded, input kept", m.expanded, m.input.Value())
	}
	// Typing goes back to the list: the player restores and the key types.
	m, _ = press(t, m, "x")
	if m.expanded || m.focus != focusList || m.input.Value() != "fx" {
		t.Fatalf("typing: expanded %v focus %v input %q; want restored, typed", m.expanded, m.focus, m.input.Value())
	}
}

func TestCompactExpandHidesTheList(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 50, Height: 14})
	if !strings.Contains(plain(m), "NIGHT DRIVE") {
		t.Fatalf("compact view lacks the list:\n%s", plain(m))
	}
	m, _ = press(t, m, "f")
	if strings.Contains(plain(m), "NIGHT DRIVE") {
		t.Fatalf("compact expanded view still lists stations:\n%s", plain(m))
	}
	for _, id := range []string{zonePrev, zonePlay, zoneNext, zoneExpand, zoneSeek} {
		zoneOf(t, m, id)
	}
	m, _ = press(t, m, "f")
	if !strings.Contains(plain(m), "NIGHT DRIVE") {
		t.Fatalf("compact restore lacks the list:\n%s", plain(m))
	}
	// The tiny layout has nothing to expand; it still fits.
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 18, Height: 4})
	m, _ = press(t, m, "f")
	if lines := strings.Split(plain(m), "\n"); len(lines) > 4 {
		t.Fatalf("tiny expanded view is %d lines", len(lines))
	}
}

func TestArrowsWalkFromTheListThroughTheButtons(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	steps := []struct {
		key   string
		focus focusArea
		ctl   playerControl
	}{
		{"right", focusPlayer, ctlPlay},
		{"right", focusPlayer, ctlNext},
		{"right", focusPlayer, ctlExpand},
		{"right", focusPlayer, ctlExpand}, // the last button stays
		{"left", focusPlayer, ctlNext},
		{"left", focusPlayer, ctlPlay},
		{"left", focusPlayer, ctlPrev},
		{"left", focusList, ctlPrev},
	}
	for i, s := range steps {
		m, _ = press(t, m, s.key)
		if m.focus != s.focus || (s.focus == focusPlayer && m.control != s.ctl) {
			t.Fatalf("step %d (%s): focus %v control %v; want %v %v", i, s.key, m.focus, m.control, s.focus, s.ctl)
		}
	}
	if m.stationCursor() != 0 {
		t.Fatalf("walking moved the station cursor to %d", m.stationCursor())
	}
}

func TestFocusedControlIsMarked(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	m, _ = press(t, m, "right", "right") // NEXT
	if got := textAt(m, zoneOf(t, m, zoneNext)); !strings.Contains(got, "▸") {
		t.Fatalf("focused NEXT shows %q; want the ▸ marker", got)
	}
	if got := textAt(m, zoneOf(t, m, zonePlay)); strings.Contains(got, "▸") {
		t.Fatalf("PLAY is marked too: %q", got)
	}
	top := strings.Split(plain(m), "\n")[2]
	if !strings.Contains(top, "▯ STATIONS") || !strings.Contains(top, "▮ NOW PLAYING") {
		t.Fatalf("panel titles do not show the active side: %q", top)
	}
	m, _ = press(t, m, "up")
	bar := zoneOf(t, m, zoneSeek)
	if got := ansi.Cut(strings.Split(plain(m), "\n")[bar.y], bar.x-1, bar.x); got != "▸" {
		t.Fatalf("focused progress bar lacks its marker, got %q", got)
	}
}

func TestProgressBarSeeksWithTheArrows(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	m, _ = press(t, m, "right", "right", "up") // NEXT, then the bar
	if !m.onBar {
		t.Fatal("up did not reach the progress bar")
	}
	m, cmd := press(t, m, "right")
	m = settle(t, m, cmd)
	assertCall(t, f, "Seek", 70*time.Second)
	m, cmd = press(t, m, "left")
	m = settle(t, m, cmd)
	assertCall(t, f, "Seek", 60*time.Second)
	m, _ = press(t, m, "down")
	if m.onBar || m.focus != focusPlayer || m.control != ctlNext {
		t.Fatalf("down: onBar %v focus %v control %v; want back on NEXT", m.onBar, m.focus, m.control)
	}
}

func TestProgressBarNeedsASeekableSong(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, "right", "up")
	if m.onBar {
		t.Fatal("up reached a progress bar with nothing to seek")
	}
}

func TestEnterActivatesTheFocusedButton(t *testing.T) {
	tests := []struct {
		name   string
		status playback.Status
		keys   []string
		method string
	}{
		{"pause", playback.StatusPlaying, []string{"right"}, "Pause"},
		{"resume", playback.StatusPaused, []string{"right"}, "Resume"},
		{"next", playback.StatusPlaying, []string{"right", "right"}, "Next"},
		{"previous", playback.StatusPlaying, []string{"right", "left"}, "Previous"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			m := loaded(t, f, newClock())
			s := playing(time.Minute, 3*time.Minute)
			s.Status = tt.status
			m, _ = step(t, m, stateMsg{state: s})
			m, _ = press(t, m, tt.keys...)
			m, cmd := press(t, m, "enter")
			settle(t, m, cmd)
			assertCall(t, f, tt.method)
			if len(callsOf(f, "PlayPlaylist")) != 0 {
				t.Fatal("enter also tuned the selected station")
			}
		})
	}
	t.Run("expand", func(t *testing.T) {
		m := playingModel(t, playbacktest.New())
		m, _ = press(t, m, "right", "right", "right", "enter")
		if !m.expanded || m.control != ctlExpand {
			t.Fatalf("expanded %v control %v; want expanded, still on the button", m.expanded, m.control)
		}
		m, _ = press(t, m, "enter")
		if m.expanded || m.focus != focusList {
			t.Fatalf("expanded %v focus %v; want restored, list focused", m.expanded, m.focus)
		}
	})
}

func TestEscReturnsFocusToTheList(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	m, _ = press(t, m, "right", "esc")
	if m.focus != focusList {
		t.Fatalf("focus %v; want the list", m.focus)
	}
	m, _ = press(t, m, "f", "esc")
	if m.expanded || m.focus != focusList {
		t.Fatalf("esc from the expanded player: expanded %v focus %v; want restored", m.expanded, m.focus)
	}
	// The expanded player keeps the focus: left from PREV goes nowhere.
	m, _ = press(t, m, "f", "left", "left", "left", "left")
	if !m.expanded || m.focus != focusPlayer || m.control != ctlPrev {
		t.Fatalf("expanded %v focus %v control %v; want the player kept", m.expanded, m.focus, m.control)
	}
}

func TestSearchInputKeepsTheArrows(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, playingModel(t, f), "daft")
	m, _ = press(t, m, "left", "left")
	m = typeText(t, m, "X")
	if m.focus != focusList || m.input.Value() != "daXft" {
		t.Fatalf("focus %v input %q; want the arrows to move the caret", m.focus, m.input.Value())
	}
	m, _ = press(t, m, "right", "right", "right")
	if m.focus != focusList {
		t.Fatal("right at the end of the input left the input")
	}

	// With a row selected, right crosses to the player and back.
	m = searchFor(t, playingModel(t, f), "daft")
	m, _ = press(t, m, "down", "right")
	if m.focus != focusPlayer || m.input.Value() != "daft" {
		t.Fatalf("focus %v input %q; want the player, input untouched", m.focus, m.input.Value())
	}
	m, _ = press(t, m, "left", "left")
	if m.focus != focusList || m.cursor() != 0 || m.input.Value() != "daft" {
		t.Fatalf("focus %v cursor %d input %q; want back on row 0", m.focus, m.cursor(), m.input.Value())
	}
	// Left on a row does not reach the hidden caret.
	m, _ = press(t, m, "left")
	m, _ = press(t, m, "up")
	m = typeText(t, m, "!")
	if m.input.Value() != "daft!" {
		t.Fatalf("input %q; left on a row moved the caret", m.input.Value())
	}
}

func TestSeekKeysInSearch(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, playingModel(t, f), "daft")
	m, cmd := press(t, m, "shift+right")
	settle(t, m, cmd)
	assertCall(t, f, "Seek", 70*time.Second)
	if m.input.Value() != "daft" {
		t.Fatalf("shift+right edited the input: %q", m.input.Value())
	}
	m = typeText(t, m, ",.")
	if m.input.Value() != "daft,." {
		t.Fatalf(",/. in the input: %q; want typed", m.input.Value())
	}
}

func TestClicksMoveTheFocus(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	m, cmd := click(t, m, zoneNext)
	settle(t, m, cmd)
	if m.focus != focusPlayer || m.control != ctlNext {
		t.Fatalf("focus %v control %v; want NEXT focused", m.focus, m.control)
	}
	m, cmd = click(t, m, zoneSeek)
	settle(t, m, cmd)
	if m.focus != focusPlayer || !m.onBar {
		t.Fatalf("focus %v onBar %v; want the bar focused", m.focus, m.onBar)
	}
	m, cmd = click(t, m, rowZone(1))
	settle(t, m, cmd)
	if m.focus != focusList || m.stationCursor() != 1 {
		t.Fatalf("focus %v cursor %d; want the list on row 1", m.focus, m.stationCursor())
	}
	assertCall(t, f, "PlayPlaylist", "pl-2")
}

func TestPlayerKeysKeepWorkingWithThePlayerFocused(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	m, _ = press(t, m, "right")
	m, cmd := press(t, m, "n")
	settle(t, m, cmd)
	assertCall(t, f, "Next")
	m, cmd = press(t, m, ".")
	settle(t, m, cmd)
	assertCall(t, f, "Seek", 70*time.Second)
	if m.focus != focusPlayer {
		t.Fatalf("focus %v; player keys moved it", m.focus)
	}
	// A list key goes back to the list.
	m, _ = press(t, m, "j")
	if m.focus != focusList || m.stationCursor() != 1 {
		t.Fatalf("focus %v cursor %d; want the list, moved", m.focus, m.stationCursor())
	}
}

func TestHintsFollowTheFocus(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	footer := func(m Model) string {
		lines := strings.Split(plain(m), "\n")
		return lines[len(lines)-1]
	}
	if got := footer(m); !strings.Contains(got, "[,/.] SEEK") || strings.Contains(got, "[←→] SEEK") {
		t.Fatalf("stations footer %q; want the new seek keys", got)
	}
	m, _ = press(t, m, "right")
	if got := footer(m); !strings.Contains(got, "[ENTER] PRESS") || !strings.Contains(got, "[ESC] LIST") {
		t.Fatalf("player footer %q", got)
	}
	m, _ = press(t, m, "f")
	if got := footer(m); !strings.Contains(got, "[F] RESTORE") {
		t.Fatalf("expanded footer %q; want F to RESTORE", got)
	}
}

func TestRecentWritesPersistInIssueOrder(t *testing.T) {
	r := &fakeRecents{}
	m := withThreeRecents(t, r)
	m, _ = press(t, m, "down")
	m, remove := press(t, m, "delete") // QUEEN
	m, _ = press(t, m, "down", "down", "down")
	m, clear := press(t, m, "enter") // CLEAR RECENT
	if len(m.recents) != 0 {
		t.Fatalf("recents %q; want cleared", m.recents)
	}
	// The commands run in the opposite order; the store still sees the
	// remove before the clear.
	m = settle(t, m, clear)
	m = settle(t, m, remove)
	if want := []string{"remove:queen", "clear"}; !reflect.DeepEqual(r.Ops(), want) {
		t.Fatalf("store ops %q; want %q", r.Ops(), want)
	}
	if strings.Contains(plain(m), "NOT SAVED") {
		t.Fatalf("a write reported a failure:\n%s", plain(m))
	}
}

func TestRecentAddThenRemoveEndsRemoved(t *testing.T) {
	store := history.NewMemory()
	if err := store.Add("queen"); err != nil {
		t.Fatal(err)
	}
	m := New(playbacktest.New(), Options{Now: newClock().now, Seed: 2077, Recents: store})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = step(t, m, run(t, m.loadRecentsCmd()))
	add := m.remember("daft")
	m, _ = press(t, m, "/", "down") // DAFT
	m, remove := press(t, m, "delete")
	// The remove runs first; it writes the add queued before it.
	settle(t, m, remove)
	if got, _ := store.Load(); !reflect.DeepEqual(got, []string{"queen"}) {
		t.Fatalf("stored %q; want [queen]", got)
	}
	if msg, ok := run(t, add).(recentSavedMsg); !ok || msg.err != nil {
		t.Fatalf("late add command returned %#v", msg)
	}
	if got, _ := store.Load(); !reflect.DeepEqual(got, []string{"queen"}) {
		t.Fatalf("stored %q after the late add command; want [queen]", got)
	}
}
