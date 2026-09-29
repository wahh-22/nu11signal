package radio

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// hintsOf is the footer of the frame, unstyled.
func hintsOf(m Model) string {
	lines := strings.Split(plain(m), "\n")
	return lines[len(lines)-1]
}

func TestBackButtonShowsOnlyOverARootView(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	r := &fakeRecents{terms: []string{"queen"}}
	views := []struct {
		name string
		open func() Model
		back bool
	}{
		{"stations", func() Model { return loaded(t, f, newClock()) }, false},
		{"search", func() Model {
			m, _ := press(t, loaded(t, f, newClock()), "/")
			return m
		}, false},
		{"search results", func() Model { return searchFor(t, loaded(t, f, newClock()), "daft") }, false},
		{"stations over a parked branch", func() Model {
			m, _ := press(t, openResults(t, playbacktest.New(), r), "tab")
			return m
		}, false},
		{"results", func() Model { return openResults(t, playbacktest.New(), r) }, true},
		{"artist", func() Model { return openDaftPunk(t, playbacktest.New()) }, true},
		{"album", func() Model { return openFromArtist(t, playbacktest.New(), itemAlbum) }, true},
		{"song", func() Model { return openSong(t, playbacktest.New(), 1) }, true},
		{"playlist", func() Model { return openFromArtist(t, playbacktest.New(), itemPlaylist) }, true},
	}
	for _, v := range views {
		t.Run(v.name, func(t *testing.T) {
			m := v.open()
			_, zs := m.layout()
			if got := hasZone(zs, zoneBack); got != v.back {
				t.Fatalf("BACK zone %v; want %v", got, v.back)
			}
			if got := strings.Contains(strings.Split(plain(m), "\n")[1], "BACK"); got != v.back {
				t.Fatalf("nav line shows BACK %v; want %v:\n%s", got, v.back, plain(m))
			}
		})
	}
}

func TestUpPastTheTopOfTheStationsFocusesTheTabs(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, "down", "up", "up")
	if m.focus != areaList || m.stationCursor() != -1 {
		t.Fatalf("focus %v cursor %d; want the list on + NEW PLAYLIST, over the first station", m.focus, m.stationCursor())
	}
	m, _ = press(t, m, "up")
	if m.focus != areaTabs || m.tab != 0 {
		t.Fatalf("focus %v tab %d; want the PLAYLISTS tab", m.focus, m.tab)
	}
	if got := textAt(m, zoneOf(t, m, zoneTabStations)); !strings.Contains(got, "▸") {
		t.Fatalf("focused PLAYLISTS tab shows %q; want the ▸ marker", got)
	}
	if got := textAt(m, zoneOf(t, m, zoneTabSearch)); strings.Contains(got, "▸") {
		t.Fatalf("SEARCH is marked too: %q", got)
	}
	// The tabs hold the focus against more ups.
	m, _ = press(t, m, "up")
	if m.focus != areaTabs {
		t.Fatalf("focus %v; want the tabs kept", m.focus)
	}
	m, _ = press(t, m, "down")
	if m.focus != areaList || m.stationCursor() != -1 {
		t.Fatalf("down: focus %v cursor %d; want back on + NEW PLAYLIST", m.focus, m.stationCursor())
	}
	// k is the volume, not up: the list keeps the focus.
	m, _ = press(t, m, "k")
	if m.focus != areaList {
		t.Fatalf("k at the top: focus %v; want the list kept", m.focus)
	}
	m, _ = press(t, m, "up", "esc")
	if m.focus != areaList || len(m.stack) != 1 {
		t.Fatalf("esc: focus %v stack %v; want back on the list, nothing popped", m.focus, stackKinds(m))
	}
}

func TestTabsWalkAndActivate(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, "up", "up", "left")
	if m.focus != areaTabs || m.tab != 0 {
		t.Fatalf("left from the first tab: focus %v tab %d; want it kept", m.focus, m.tab)
	}
	m, _ = press(t, m, "right", "right")
	if m.tab != 1 {
		t.Fatalf("tab %d; want SEARCH, the last tab over the stations", m.tab)
	}
	if got := textAt(m, zoneOf(t, m, zoneTabSearch)); !strings.Contains(got, "▸") {
		t.Fatalf("focused SEARCH tab shows %q; want the ▸ marker", got)
	}
	m, _ = press(t, m, "enter")
	if m.top().kind != viewSearch || m.focus != areaList || !m.input.Focused() {
		t.Fatalf("enter on SEARCH: top %v focus %v input %v; want SEARCH with the input", m.top().kind, m.focus, m.input.Focused())
	}

	// On a page BACK is a tab too; enter on it goes back as esc does.
	m = openResults(t, playbacktest.New(), &fakeRecents{})
	m, _ = press(t, m, "up")
	if m.focus != areaTabs || m.tab != 1 {
		t.Fatalf("focus %v tab %d; want the lit SEARCH tab", m.focus, m.tab)
	}
	m, _ = press(t, m, "right", "right")
	if m.tab != 2 {
		t.Fatalf("tab %d; want BACK", m.tab)
	}
	m, _ = press(t, m, "enter")
	if !slices.Equal(stackKinds(m), []viewKind{viewStations, viewSearch}) || m.focus != areaList {
		t.Fatalf("enter on BACK: stack %v focus %v; want SEARCH with the list focused", stackKinds(m), m.focus)
	}

	// Enter on PLAYLISTS parks the search branch, as its click does.
	m = openResults(t, playbacktest.New(), &fakeRecents{})
	m, _ = press(t, m, "up", "left", "enter")
	if len(m.stack) != 1 || len(m.parked) == 0 || m.focus != areaList {
		t.Fatalf("enter on PLAYLISTS: stack %v parked %d focus %v; want the stations", stackKinds(m), len(m.parked), m.focus)
	}
}

func TestUpFromTheSearchInputFocusesTheTabs(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "down", "up")
	if m.focus != areaList || m.cursor() != -1 || !m.input.Focused() {
		t.Fatalf("up from the first row: focus %v cursor %d; want the input", m.focus, m.cursor())
	}
	m, _ = press(t, m, "up")
	if m.focus != areaTabs || m.tab != 1 || m.input.Focused() {
		t.Fatalf("up from the input: focus %v tab %d input focused %v; want the SEARCH tab, input blurred", m.focus, m.tab, m.input.Focused())
	}
	if got := hintsOf(m); !strings.Contains(got, "[←→]") || !strings.Contains(got, "[CTRL+C] QUIT") {
		t.Fatalf("tabs footer %q; want ←→ and CTRL+C to quit", got)
	}
	m, _ = press(t, m, "down")
	if m.focus != areaList || m.cursor() != -1 || !m.input.Focused() || m.input.Value() != "daft" {
		t.Fatalf("down: focus %v cursor %d input %q; want the input back", m.focus, m.cursor(), m.input.Value())
	}
	// A letter on the tabs goes back to the input and is typed.
	m, _ = press(t, m, "up")
	m = typeText(t, m, "y")
	if m.focus != areaList || m.input.Value() != "dafty" {
		t.Fatalf("typing on the tabs: focus %v input %q; want it typed in the input", m.focus, m.input.Value())
	}
}

func TestUpPastTheTopOfAPageFocusesTheTabs(t *testing.T) {
	m := openResults(t, playbacktest.New(), &fakeRecents{})
	m, _ = press(t, m, "down", "up")
	if m.focus != areaList || m.cursor() != 0 {
		t.Fatalf("focus %v cursor %d; want the first row", m.focus, m.cursor())
	}
	m, _ = press(t, m, "up")
	if m.focus != areaTabs {
		t.Fatalf("focus %v; want the tabs", m.focus)
	}
	m, _ = press(t, m, "down")
	if m.focus != areaList || m.cursor() != 0 || m.top().kind != viewResults {
		t.Fatalf("down: focus %v cursor %d top %v; want the first row", m.focus, m.cursor(), m.top().kind)
	}
}

func TestUpFromTheProgressBarFocusesTheTabs(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	m, _ = press(t, m, "right", "right", "up") // NEXT, then the bar
	if !m.onBar {
		t.Fatal("up did not reach the progress bar")
	}
	m, _ = press(t, m, "up")
	if m.focus != areaTabs || m.tab != 0 {
		t.Fatalf("up from the bar: focus %v tab %d; want the PLAYLISTS tab", m.focus, m.tab)
	}
	if strings.Contains(textAt(m, zoneOf(t, m, zoneNext)), "▸") {
		t.Fatal("NEXT still marked with the tabs focused")
	}
	m, _ = press(t, m, "down")
	if m.focus != areaPlayer || !m.onBar || m.control != ctlNext {
		t.Fatalf("down: focus %v onBar %v control %v; want back on the bar", m.focus, m.onBar, m.control)
	}
	m, _ = press(t, m, "up", "esc")
	if m.focus != areaPlayer || !m.onBar {
		t.Fatalf("esc: focus %v onBar %v; want back on the bar", m.focus, m.onBar)
	}
}

func TestUpFromTheButtonsWithNothingToSeekFocusesTheTabs(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, "right", "up")
	if m.focus != areaTabs || m.onBar {
		t.Fatalf("focus %v onBar %v; want the tabs", m.focus, m.onBar)
	}
	m, _ = press(t, m, "down")
	if m.focus != areaPlayer || m.control != ctlPlay {
		t.Fatalf("focus %v control %v; want back on PLAY", m.focus, m.control)
	}
}

func TestWheelUpAtTheTopKeepsTheList(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	// The wheel stops on the top row, + NEW PLAYLIST.
	m = wheel(t, m, tea.MouseWheelUp)
	m = wheel(t, m, tea.MouseWheelUp)
	if m.focus != areaList || m.stationCursor() != -1 {
		t.Fatalf("focus %v cursor %d; want the list kept", m.focus, m.stationCursor())
	}
}

func TestWheelNeverClimbsToTheTabs(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	t.Run("page top", func(t *testing.T) {
		m := openResults(t, playbacktest.New(), &fakeRecents{})
		m, _ = press(t, m, "down")
		m = wheel(t, m, tea.MouseWheelUp)
		if m.focus != areaList || m.cursor() != 0 {
			t.Fatalf("focus %v cursor %d; want the first row", m.focus, m.cursor())
		}
		m = wheel(t, m, tea.MouseWheelUp)
		if m.focus != areaList || m.cursor() != 0 {
			t.Fatalf("at the top: focus %v cursor %d; want the list kept", m.focus, m.cursor())
		}
	})
	t.Run("search rows up to the input", func(t *testing.T) {
		m := searchFor(t, loaded(t, f, newClock()), "daft")
		m, _ = press(t, m, "down", "down")
		m = wheel(t, m, tea.MouseWheelUp)
		if m.cursor() != 0 {
			t.Fatalf("cursor %d; want the first row", m.cursor())
		}
		m = wheel(t, m, tea.MouseWheelUp)
		if m.focus != areaList || m.cursor() != -1 {
			t.Fatalf("focus %v cursor %d; want the input", m.focus, m.cursor())
		}
		m = wheel(t, m, tea.MouseWheelUp)
		if m.focus != areaList || m.cursor() != -1 || !m.input.Focused() {
			t.Fatalf("on the input: focus %v cursor %d input %v; want the input kept", m.focus, m.cursor(), m.input.Focused())
		}
	})
}

func TestRecentSearchesOfferNoClearAll(t *testing.T) {
	m := withThreeRecents(t, &fakeRecents{})
	if rows := m.searchRows(); len(rows) != 3 {
		t.Fatalf("rows = %+v; want the three terms only", rows)
	}
	if view := plain(m); strings.Contains(view, "CLEAR") {
		t.Fatalf("view offers CLEAR RECENT:\n%s", view)
	}
	m, _ = press(t, m, "down", "down", "down", "down")
	if m.cursor() != 2 {
		t.Fatalf("cursor %d; want the last term", m.cursor())
	}
}
