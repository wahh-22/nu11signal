package radio

import (
	"fmt"
	"reflect"
	"slices"
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
			if !m.expanded || m.focus != areaPlayer {
				t.Fatalf("expanded %v focus %v; want expanded with the player focused", m.expanded, m.focus)
			}
			view := plain(m)
			top := strings.Split(view, "\n")[2]
			if !strings.Contains(top, "NOW PLAYING") || !strings.HasSuffix(top, "┐") || ansi.StringWidth(top) != 80 {
				t.Fatalf("NOW PLAYING does not span the screen: %q", top)
			}
			if strings.Contains(view, "NIGHT DRIVE") || !strings.Contains(view, "PLAYLISTS") {
				t.Fatalf("list still shown, or the nav bar is gone:\n%s", view)
			}
			if !strings.Contains(textAt(m, zoneOf(t, m, zoneExpand)), "RESTORE") {
				t.Fatalf("the expand button does not offer RESTORE:\n%s", view)
			}
			m, _ = press(t, m, k)
			if m.expanded || m.focus != areaList || !strings.Contains(plain(m), "NIGHT DRIVE") {
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
	if !m.expanded || m.focus != areaPlayer || m.control != ctlExpand {
		t.Fatalf("expanded %v focus %v control %v; want expanded, EXPAND focused", m.expanded, m.focus, m.control)
	}
	m, _ = click(t, m, zoneExpand)
	if m.expanded || m.focus != areaList {
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
	if m.expanded || m.focus != areaList || m.input.Value() != "fx" {
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
		{"right", areaPlayer, ctlPlay},
		{"right", areaPlayer, ctlNext},
		{"right", areaPlayer, ctlExpand},
		{"right", areaPlayer, ctlExpand}, // the last button stays
		{"left", areaPlayer, ctlNext},
		{"left", areaPlayer, ctlPlay},
		{"left", areaPlayer, ctlPrev},
		{"left", areaList, ctlPrev},
	}
	for i, s := range steps {
		m, _ = press(t, m, s.key)
		if m.focus != s.focus || (s.focus == areaPlayer && m.control != s.ctl) {
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
	if !strings.Contains(top, "▯ PLAYLISTS") || !strings.Contains(top, "▮ NOW PLAYING") {
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
	if m.onBar || m.focus != areaPlayer || m.control != ctlNext {
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
		if m.expanded || m.focus != areaList {
			t.Fatalf("expanded %v focus %v; want restored, list focused", m.expanded, m.focus)
		}
	})
}

func TestEscReturnsFocusToTheList(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	m, _ = press(t, m, "right", "esc")
	if m.focus != areaList {
		t.Fatalf("focus %v; want the list", m.focus)
	}
	m, _ = press(t, m, "f", "esc")
	if m.expanded || m.focus != areaList {
		t.Fatalf("esc from the expanded player: expanded %v focus %v; want restored", m.expanded, m.focus)
	}
	// The expanded player keeps the focus: left from PREV goes nowhere.
	m, _ = press(t, m, "f", "left", "left", "left", "left")
	if !m.expanded || m.focus != areaPlayer || m.control != ctlPrev {
		t.Fatalf("expanded %v focus %v control %v; want the player kept", m.expanded, m.focus, m.control)
	}
}

func TestSearchInputKeepsTheArrows(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, playingModel(t, f), "daft")
	m, _ = press(t, m, "left", "left")
	m = typeText(t, m, "X")
	if m.focus != areaList || m.input.Value() != "daXft" {
		t.Fatalf("focus %v input %q; want the arrows to move the caret", m.focus, m.input.Value())
	}
	m, _ = press(t, m, "right", "right", "right")
	if m.focus != areaList {
		t.Fatal("right at the end of the input left the input")
	}

	// With a row selected, right crosses to the player and back.
	m = searchFor(t, playingModel(t, f), "daft")
	m, _ = press(t, m, "down", "right")
	if m.focus != areaPlayer || m.input.Value() != "daft" {
		t.Fatalf("focus %v input %q; want the player, input untouched", m.focus, m.input.Value())
	}
	m, _ = press(t, m, "left", "left")
	if m.focus != areaList || m.cursor() != 0 || m.input.Value() != "daft" {
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
	if m.focus != areaPlayer || m.control != ctlNext {
		t.Fatalf("focus %v control %v; want NEXT focused", m.focus, m.control)
	}
	m, cmd = click(t, m, zoneSeek)
	settle(t, m, cmd)
	if m.focus != areaPlayer || !m.onBar {
		t.Fatalf("focus %v onBar %v; want the bar focused", m.focus, m.onBar)
	}
	m, cmd = click(t, m, rowZone(1))
	settle(t, m, cmd)
	if m.focus != areaList || m.stationCursor() != 1 {
		t.Fatalf("focus %v cursor %d; want the list on row 1", m.focus, m.stationCursor())
	}
	assertCall(t, f, "LibraryPlaylist", "pl-2")
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
	if m.focus != areaPlayer {
		t.Fatalf("focus %v; player keys moved it", m.focus)
	}
	// A list key goes back to the list.
	m, _ = press(t, m, "/")
	if m.focus != areaList || m.top().kind != viewSearch {
		t.Fatalf("focus %v view %v; want the list, on SEARCH", m.focus, m.top().kind)
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
	wide, _ := step(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	if got := footer(wide); !strings.Contains(got, "[J/K] VOL") || !strings.Contains(got, "[↑↓] MOVE") || strings.Contains(got, "J/K] MOVE") {
		t.Fatalf("wide stations footer %q; want J/K for the volume, the arrows to move", got)
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
	m, first := press(t, m, "delete")  // QUEEN
	m, second := press(t, m, "delete") // DAFT PUNK
	if want := []string{"samurai"}; !reflect.DeepEqual(m.recents, want) {
		t.Fatalf("recents %q; want %q", m.recents, want)
	}
	// The commands run in the opposite order; the store still sees them
	// in the order they were issued.
	m = settle(t, m, second)
	m = settle(t, m, first)
	if want := []string{"remove:queen", "remove:daft punk"}; !reflect.DeepEqual(r.Ops(), want) {
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

func TestReturningFromThePlayerRestoresTheSearchAsItWas(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	tests := []struct {
		name string
		keys []string // from the input to the list state left for the player
		back []string // from the player back to the list
	}{
		{"row by arrows", []string{"down", "right"}, []string{"left", "left"}},
		{"row by esc", []string{"down", "right"}, []string{"esc"}},
		{"row by expand", []string{"down", "ctrl+f"}, []string{"ctrl+f"}},
		{"input by expand", []string{"ctrl+f"}, []string{"ctrl+f"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := searchFor(t, playingModel(t, f), "daft")
			m, _ = press(t, m, tt.keys[:len(tt.keys)-1]...)
			cursor, focused := m.cursor(), m.input.Focused()
			m, _ = press(t, m, tt.keys[len(tt.keys)-1])
			if m.focus != areaPlayer || m.input.Focused() {
				t.Fatalf("focus %v input focused %v; want the player alone", m.focus, m.input.Focused())
			}
			m, _ = press(t, m, tt.back...)
			if m.focus != areaList || m.cursor() != cursor || m.input.Focused() != focused || m.input.Value() != "daft" {
				t.Fatalf("back: focus %v cursor %d focused %v input %q; want the list, cursor %d, focused %v",
					m.focus, m.cursor(), m.input.Focused(), m.input.Value(), cursor, focused)
			}
		})
	}
}

func TestExpandKeepsTheFocusedControl(t *testing.T) {
	// Expanding focuses the player on the control it already had (PLAY
	// from the list); the EXPAND button is that control when clicked or
	// pressed. Restoring always gives the focus back to the list.
	tests := []struct {
		name   string
		expand func(t *testing.T, m Model) Model
		want   playerControl
	}{
		{"key from the list", func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "f")
			return m
		}, ctlPlay},
		{"key on NEXT", func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "right", "right", "f")
			return m
		}, ctlNext},
		{"key on the bar", func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "right", "up", "ctrl+f")
			return m
		}, ctlPlay},
		{"enter on EXPAND", func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "right", "right", "right", "enter")
			return m
		}, ctlExpand},
		{"button from the list", func(t *testing.T, m Model) Model {
			m, _ = click(t, m, zoneExpand)
			return m
		}, ctlExpand},
		{"button on NEXT", func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "right", "right")
			m, _ = click(t, m, zoneExpand)
			return m
		}, ctlExpand},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.expand(t, playingModel(t, playbacktest.New()))
			if !m.expanded || m.focus != areaPlayer || m.control != tt.want {
				t.Fatalf("expanded %v focus %v control %v; want expanded on %v", m.expanded, m.focus, m.control, tt.want)
			}
			for _, restore := range []string{"key", "button"} {
				r := m
				if restore == "key" {
					r, _ = press(t, r, "f")
				} else {
					r, _ = click(t, r, zoneExpand)
				}
				if r.expanded || r.focus != areaList || r.onBar {
					t.Fatalf("restore by %s: expanded %v focus %v onBar %v; want the list", restore, r.expanded, r.focus, r.onBar)
				}
			}
		})
	}
}

func TestBarFocusFallsBackWhenTheSongStopsBeingSeekable(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	m, _ = press(t, m, "right", "right", "up") // NEXT, then the bar
	if !m.barFocused() {
		t.Fatal("up did not reach the progress bar")
	}
	live := playing(0, 0) // a stream with no length
	m, _ = step(t, m, stateMsg{state: live})
	if m.onBar || !m.focused(ctlNext) {
		t.Fatalf("onBar %v control %v; want the focus back on NEXT", m.onBar, m.control)
	}
	if got := textAt(m, zoneOf(t, m, zoneNext)); !strings.Contains(got, "▸") {
		t.Fatalf("NEXT shows %q; want the ▸ marker", got)
	}
	// A seekable song later does not bring the bar focus back.
	m, _ = step(t, m, stateMsg{state: playing(time.Minute, 3*time.Minute)})
	if m.onBar {
		t.Fatal("the bar took the focus back by itself")
	}
}

func TestRightMovesTheFocusToThePlayerFromEveryPage(t *testing.T) {
	pages := []struct {
		name string
		kind viewKind
		open func(t *testing.T, f *playbacktest.Fake) Model
	}{
		{"results", viewResults, func(t *testing.T, f *playbacktest.Fake) Model {
			return openResults(t, f, &fakeRecents{})
		}},
		{"artist", viewArtist, openDaftPunk},
		{"album", viewAlbum, func(t *testing.T, f *playbacktest.Fake) Model {
			return openFromArtist(t, f, itemAlbum)
		}},
		{"song", viewAlbum, func(t *testing.T, f *playbacktest.Fake) Model { return openSong(t, f, 0) }},
		{"playlist", viewPlaylist, func(t *testing.T, f *playbacktest.Fake) Model {
			return openFromArtist(t, f, itemPlaylist)
		}},
	}
	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			m := p.open(t, playbacktest.New())
			if m.top().kind != p.kind {
				t.Fatalf("opened %v; want %v", m.top().kind, p.kind)
			}
			cursor := m.cursor()
			m, _ = press(t, m, "right")
			if m.focus != areaPlayer || m.control != ctlPlay || m.top().kind != p.kind || m.cursor() != cursor {
				t.Fatalf("focus %v control %v top %v cursor %d; want PLAY focused over the page, cursor %d",
					m.focus, m.control, m.top().kind, m.cursor(), cursor)
			}
			m, _ = press(t, m, "left", "left")
			if m.focus != areaList || m.top().kind != p.kind || m.cursor() != cursor {
				t.Fatalf("back: focus %v top %v cursor %d; want the page as left", m.focus, m.top().kind, m.cursor())
			}
		})
	}
}

func TestStationsFooterKeepsTheEssentialHintsAt80Columns(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	lines := strings.Split(plain(m), "\n")
	got := strings.TrimRight(lines[len(lines)-1], " ")
	want := "[ENTER] OPEN  [/] SCAN  [SPACE] PLAY/PAUSE  [→] PLAYER  [,/.] SEEK  [Q] QUIT"
	if got != want {
		t.Fatalf("stations footer\n got %q\nwant %q", got, want)
	}
}

// TestFitHintsMatchesRestylingEveryCandidate pins fitHints, which styles
// each hint once, to the plain algorithm that restyles every candidate
// line, at every width and for every hint set.
func TestFitHintsMatchesRestylingEveryCandidate(t *testing.T) {
	join := func(hs []hint) string {
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = keyCap(h.key) + " " + stRed.Render(h.label)
		}
		return strings.Join(parts, "  ")
	}
	reference := func(hints []hint, w int) string {
		if ansi.StringWidth(join(hints)) > w {
			hints = shortHints(hints)
		}
		shown := append([]hint(nil), hints...)
		for len(shown) > 1 && ansi.StringWidth(join(shown)) > w {
			shown = append(shown[:len(shown)-2], shown[len(shown)-1])
		}
		return join(shown)
	}
	sets := map[string][]hint{
		"player": playerHints, "artist": artistHints, "results": resultsHints,
		"track": trackHints, "picker": pickerHints, "name": nameHints,
		"searchSong": searchSongHints, "search": searchHints, "recent": recentHints,
		"tabs": tabsFocusHints(false), "playerFocus": playerFocusHints(true, true),
	}
	for name, hs := range sets {
		for w := 0; w <= 160; w++ {
			if got, want := fitHints(hs, w), reference(hs, w); got != want {
				t.Fatalf("%s at %d cells:\n got %q\nwant %q", name, w, got, want)
			}
		}
	}
}

func TestPlayerFocusHintsFollowTheExpandedStateAndTheInput(t *testing.T) {
	tests := []struct {
		expanded, typing bool
		expand, quit     hint
	}{
		{false, false, hint{"F", "EXPAND"}, hint{"Q", "QUIT"}},
		{true, false, hint{"F", "RESTORE"}, hint{"Q", "QUIT"}},
		{false, true, hint{"F", "EXPAND"}, hint{"CTRL+C", "QUIT"}},
		{true, true, hint{"F", "RESTORE"}, hint{"CTRL+C", "QUIT"}},
	}
	for _, tt := range tests {
		hs := playerFocusHints(tt.expanded, tt.typing)
		if !slices.Contains(hs, tt.expand) || hs[len(hs)-1] != tt.quit {
			t.Errorf("expanded %v typing %v: hints %v; want %v and %v last", tt.expanded, tt.typing, hs, tt.expand, tt.quit)
		}
	}
}

func TestReturningFromThePlayerOffSearchLeavesTheInputAlone(t *testing.T) {
	// The input took the keys when the player got the focus, but SEARCH
	// is no longer on top when the focus comes back, so the input must
	// stay blurred.
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, playingModel(t, f), "daft")
	m, _ = press(t, m, "ctrl+f")
	if m.focus != areaPlayer || m.input.Focused() {
		t.Fatalf("focus %v input focused %v; want the player alone", m.focus, m.input.Focused())
	}
	m.stack = m.stack[:1:1]
	m, _ = press(t, m, "ctrl+f")
	if m.focus != areaList || m.input.Focused() {
		t.Fatalf("back on the stations: focus %v input focused %v; want the list with the input blurred", m.focus, m.input.Focused())
	}
}
