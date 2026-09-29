package radio

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// zoneOf finds the clickable region id in the frame m renders now.
func zoneOf(t *testing.T, m Model, id string) zone {
	t.Helper()
	_, zs := m.layout()
	z, ok := zs.find(id)
	if !ok {
		t.Fatalf("no %q zone in:\n%s", id, plain(m))
	}
	return z
}

// textAt is the unstyled text the frame shows under z.
func textAt(m Model, z zone) string {
	lines := strings.Split(plain(m), "\n")
	if z.y >= len(lines) {
		return ""
	}
	return ansi.Cut(lines[z.y], z.x, z.x+z.w)
}

// click presses the left button on the first cell of zone id.
func click(t *testing.T, m Model, id string) (Model, tea.Cmd) {
	t.Helper()
	z := zoneOf(t, m, id)
	return step(t, m, tea.MouseClickMsg{X: z.x, Y: z.y, Button: tea.MouseLeft})
}

func wheel(t *testing.T, m Model, b tea.MouseButton) Model {
	t.Helper()
	m, _ = step(t, m, tea.MouseWheelMsg{X: 1, Y: 5, Button: b})
	return m
}

func TestViewEnablesMouseCellMotion(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v; want cell motion", got)
	}
}

func TestClickOnStationRowTunesIt(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	if got := textAt(m, zoneOf(t, m, rowZone(1))); !strings.Contains(got, "SAMURAI") {
		t.Fatalf("row 1 zone covers %q; want the SAMURAI row", got)
	}
	m, cmd := click(t, m, rowZone(1))
	if m.stationCursor() != 1 {
		t.Fatalf("station cursor = %d; want the clicked row", m.stationCursor())
	}
	settle(t, m, cmd)
	assertCall(t, f, "PlayPlaylist", "pl-2")
}

func TestOnlyLeftPressActs(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	z := zoneOf(t, m, rowZone(2))
	before := len(f.Calls())
	for _, msg := range []tea.Msg{
		tea.MouseClickMsg{X: z.x, Y: z.y, Button: tea.MouseRight},
		tea.MouseReleaseMsg{X: z.x, Y: z.y, Button: tea.MouseLeft},
		tea.MouseMotionMsg{X: z.x, Y: z.y},
	} {
		var cmd tea.Cmd
		m, cmd = step(t, m, msg)
		if cmd != nil || m.stationCursor() != 0 {
			t.Fatalf("%T moved the cursor to %d or issued a command", msg, m.stationCursor())
		}
	}
	if len(f.Calls()) != before {
		t.Fatalf("player calls %v; want none", f.Calls()[before:])
	}
}

func TestClickOutsideAnyZoneDoesNothing(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	// The clock in the header is no button.
	m2, cmd := step(t, m, tea.MouseClickMsg{X: 79, Y: 0, Button: tea.MouseLeft})
	if cmd != nil || !reflect.DeepEqual(stackKinds(m2), stackKinds(m)) || m2.stationCursor() != 0 {
		t.Fatal("a click on the header acted")
	}
}

func TestWheelMovesTheCursorLikeTheArrowKeys(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m = wheel(t, m, tea.MouseWheelDown)
	m = wheel(t, m, tea.MouseWheelDown)
	if m.stationCursor() != 2 {
		t.Fatalf("stations cursor = %d after two wheel downs; want 2", m.stationCursor())
	}
	m = wheel(t, m, tea.MouseWheelUp)
	if m.stationCursor() != 1 {
		t.Fatalf("stations cursor = %d after a wheel up; want 1", m.stationCursor())
	}

	f.SearchCatalogResult = catalog()
	m = searchFor(t, m, "daft")
	m = wheel(t, m, tea.MouseWheelDown)
	if m.cursor() != 0 || m.input.Value() != "daft" {
		t.Fatalf("search cursor %d input %q; want the first row, input untouched", m.cursor(), m.input.Value())
	}

	m = openResults(t, playbacktest.New(), &fakeRecents{})
	m = wheel(t, m, tea.MouseWheelDown)
	m = wheel(t, m, tea.MouseWheelDown)
	if m.cursor() != 2 {
		t.Fatalf("results cursor = %d; want 2", m.cursor())
	}
}

func TestNavTabsSwitchBetweenStationsAndTheSearchBranch(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.ArtistResult = artistDetail()
	c := newClock()
	m := loaded(t, f, c)
	if got := textAt(m, zoneOf(t, m, zoneTabSearch)); !strings.Contains(got, "SEARCH") {
		t.Fatalf("search tab covers %q", got)
	}
	if got := textAt(m, zoneOf(t, m, zoneTabStations)); !strings.Contains(got, "STATIONS") {
		t.Fatalf("stations tab covers %q", got)
	}

	m, cmd := click(t, m, zoneTabSearch)
	if m.top().kind != viewSearch || !m.input.Focused() || cmd == nil {
		t.Fatalf("search tab: top %v focused %v; want a fresh focused search", m.top().kind, m.input.Focused())
	}

	// Open an artist, leave with the stations tab, come back with SEARCH.
	m = typeText(t, m, "daft")
	m, cmd = step(t, m, searchDebounceMsg{seq: m.search.seq})
	m, _ = step(t, m, run(t, cmd))
	m, _ = press(t, m, "down", "down", "down")
	m, cmd = press(t, m, "enter")
	m = settle(t, m, cmd)
	m, _ = press(t, m, "down")
	c.advance(doubleClickGuard)
	m, _ = click(t, m, zoneTabStations)
	if want := []viewKind{viewStations}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("stations tab: stack %v; want %v", stackKinds(m), want)
	}
	c.advance(doubleClickGuard)
	m, _ = click(t, m, zoneTabSearch)
	if want := []viewKind{viewStations, viewSearch, viewArtist}; !reflect.DeepEqual(stackKinds(m), want) || m.cursor() != 1 {
		t.Fatalf("search tab: stack %v cursor %d; want the artist page restored at 1", stackKinds(m), m.cursor())
	}

	// On a page SEARCH goes back to the input, as / does.
	c.advance(doubleClickGuard)
	m, _ = click(t, m, zoneTabSearch)
	if m.top().kind != viewSearch || m.cursor() != -1 || !m.input.Focused() {
		t.Fatalf("search tab on a page: top %v cursor %d; want the input", m.top().kind, m.cursor())
	}
	// On SEARCH the tab only takes the input back.
	m, _ = press(t, m, "down")
	c.advance(doubleClickGuard)
	m, _ = click(t, m, zoneTabSearch)
	if m.top().kind != viewSearch || m.cursor() != -1 || m.input.Value() != "daft" {
		t.Fatalf("search tab on search: cursor %d input %q; want the input as it was", m.cursor(), m.input.Value())
	}
}

func TestBackButtonShowsAboveTheRootAndPops(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = fullCatalog()
	c := newClock()
	m := loaded(t, f, c)
	if _, zs := m.layout(); hasZone(zs, zoneBack) {
		t.Fatal("the stations root offers BACK")
	}
	m = searchFor(t, m, "daft")
	m, cmd := press(t, m, "down", "enter")
	m = settle(t, m, cmd)
	if got := textAt(m, zoneOf(t, m, zoneBack)); !strings.Contains(got, "BACK") {
		t.Fatalf("back zone covers %q", got)
	}
	m, _ = click(t, m, zoneBack)
	if want := []viewKind{viewStations, viewSearch}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("back: stack %v; want %v", stackKinds(m), want)
	}
	// A separate click, not the second press of a double click.
	c.advance(doubleClickGuard)
	m, _ = click(t, m, zoneBack)
	if want := []viewKind{viewStations}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("back from search: stack %v; want %v", stackKinds(m), want)
	}
}

func hasZone(zs zones, id string) bool {
	_, ok := zs.find(id)
	return ok
}

func TestTransportButtonsDriveThePlayer(t *testing.T) {
	tests := []struct {
		name   string
		status playback.Status
		button string
		label  string
		method string
	}{
		{"pause", playback.StatusPlaying, zonePlay, "PAUSE", "Pause"},
		{"resume", playback.StatusPaused, zonePlay, "PLAY", "Resume"},
		{"next", playback.StatusPlaying, zoneNext, "NEXT", "Next"},
		{"previous", playback.StatusPlaying, zonePrev, "PREV", "Previous"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			m := loaded(t, f, newClock())
			s := playing(83*time.Second, 225*time.Second)
			s.Status = tt.status
			m, _ = step(t, m, stateMsg{state: s})
			// Typing never swallows a click: the buttons work over SEARCH.
			m, _ = press(t, m, "/")
			if got := textAt(m, zoneOf(t, m, tt.button)); !strings.Contains(got, tt.label) {
				t.Fatalf("%s zone covers %q; want %s", tt.button, got, tt.label)
			}
			m, cmd := click(t, m, tt.button)
			settle(t, m, cmd)
			assertCall(t, f, tt.method)
			if m.input.Value() != "" {
				t.Fatalf("input %q; the click typed", m.input.Value())
			}
		})
	}
}

func TestClickOnTheProgressBarSeeks(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	// At 80 columns the bar is 28 cells, so a 280 s song is 10 s a cell.
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 280*time.Second)})
	z := zoneOf(t, m, zoneSeek)
	if got := textAt(m, z); strings.Trim(got, "▮▯") != "" || got == "" {
		t.Fatalf("seek zone covers %q; want only the bar", got)
	}
	if z.w != 28 {
		t.Fatalf("bar is %d cells; the expected targets assume 28", z.w)
	}
	for _, tt := range []struct {
		name string
		cell int
		want time.Duration
	}{
		{"first cell is the start", 0, 0},
		{"a middle cell", 7, 70 * time.Second},
		{"last cell is its own start, short of the end", 27, 270 * time.Second},
	} {
		m2, cmd := step(t, m, tea.MouseClickMsg{X: z.x + tt.cell, Y: z.y, Button: tea.MouseLeft})
		settle(t, m2, cmd)
		t.Run(tt.name, func(t *testing.T) { assertCall(t, f, "Seek", tt.want) })
	}

	// Nothing to seek without a duration.
	m, _ = step(t, m, stateMsg{state: playback.State{Status: playback.StatusPlaying, Title: "Live"}})
	if _, zs := m.layout(); hasZone(zs, zoneSeek) {
		t.Fatal("a song without a duration offers seeking")
	}
}

func TestClickOnSearchRowsActsLikeEnter(t *testing.T) {
	t.Run("suggestion opens its results", func(t *testing.T) {
		f := playbacktest.New()
		f.SearchCatalogResult = fullCatalog()
		m := searchFor(t, loaded(t, f, newClock()), "daft")
		if got := textAt(m, zoneOf(t, m, rowZone(0))); !strings.Contains(got, "DAFT PUNK") {
			t.Fatalf("row 0 covers %q", got)
		}
		m, cmd := click(t, m, rowZone(0))
		m = settle(t, m, cmd)
		if m.top().kind != viewResults || m.top().results.term != "daft punk" {
			t.Fatalf("top %v term %q; want RESULTS for the suggestion", m.top().kind, m.top().results.term)
		}
	})
	t.Run("recent term opens its results", func(t *testing.T) {
		f := playbacktest.New()
		r := &fakeRecents{terms: []string{"queen", "abba"}}
		m := loadedWithRecents(t, f, r)
		m, _ = press(t, m, "/")
		m, cmd := click(t, m, rowZone(1))
		m = settle(t, m, cmd)
		if m.top().kind != viewResults || m.input.Value() != "abba" {
			t.Fatalf("top %v input %q; want RESULTS for abba", m.top().kind, m.input.Value())
		}
	})
	t.Run("artist opens its page", func(t *testing.T) {
		f := playbacktest.New()
		f.SearchCatalogResult = catalog()
		f.ArtistResult = artistDetail()
		m := searchFor(t, loaded(t, f, newClock()), "daft")
		m, cmd := click(t, m, rowZone(2))
		m = settle(t, m, cmd)
		if m.top().kind != viewArtist {
			t.Fatalf("top %v; want ARTIST", m.top().kind)
		}
	})
}

// pressAt presses the left button on cell (x, y).
func pressAt(t *testing.T, m Model, x, y int) (Model, tea.Cmd) {
	t.Helper()
	return step(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func TestDoubleClickOpeningAViewDoesNotActInIt(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.ArtistResult = artistDetail()
	c := newClock()
	m := searchFor(t, loaded(t, f, c), "daft")
	row := zoneOf(t, m, rowZone(2))

	// Both presses of a double click land on the artist row; the first
	// opens ARTIST, the second must not act on the ARTIST row under it.
	m, cmd := pressAt(t, m, row.x, row.y)
	m = settle(t, m, cmd)
	if m.top().kind != viewArtist {
		t.Fatalf("first press: top %v; want ARTIST", m.top().kind)
	}
	before, calls := stackKinds(m), len(f.Calls())
	c.advance(doubleClickGuard / 2)
	m, cmd = pressAt(t, m, row.x, row.y)
	if cmd != nil {
		m = settle(t, m, cmd)
	}
	if !reflect.DeepEqual(stackKinds(m), before) || m.cursor() != 0 || len(f.Calls()) != calls {
		t.Fatalf("second press acted: stack %v cursor %d calls %v", stackKinds(m), m.cursor(), f.Calls()[calls:])
	}

	// A press once the guard is over acts again.
	c.advance(doubleClickGuard)
	back := zoneOf(t, m, zoneBack)
	m, _ = pressAt(t, m, back.x, back.y)
	if m.top().kind != viewSearch {
		t.Fatalf("press after the guard: top %v; want SEARCH", m.top().kind)
	}
}

func TestDoublePressOnTransportTogglesTwice(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	play := zoneOf(t, m, zonePlay)
	for range 2 {
		var cmd tea.Cmd
		m, cmd = pressAt(t, m, play.x, play.y)
		m = settle(t, m, cmd)
	}
	if got := len(callsOf(f, "Pause")) + len(callsOf(f, "Resume")); got != 2 {
		t.Fatalf("play/pause calls %d; want both presses to toggle", got)
	}
}

func TestClickOnTheSearchInputTakesItBack(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "down", "down")
	m.input.Blur()
	m, cmd := click(t, m, zoneInput)
	if m.cursor() != -1 || !m.input.Focused() || cmd == nil {
		t.Fatalf("cursor %d focused %v; want the input selected and focused", m.cursor(), m.input.Focused())
	}
	if m.top().kind != viewSearch || len(catalogCalls(f)) != 1 {
		t.Fatal("clicking the input submitted the term")
	}
}

func TestClickOnResultRowsOpensThemAndHeadersAreInert(t *testing.T) {
	f := playbacktest.New()
	f.AlbumResult = discovery()
	m := openResults(t, f, &fakeRecents{})
	row := zoneOf(t, m, rowZone(2))
	if got := textAt(m, row); !strings.Contains(got, "DISCOVERY") {
		t.Fatalf("row 2 covers %q; want the top album", got)
	}

	// The TOP RESULTS header sits right above the first row.
	first := zoneOf(t, m, rowZone(0))
	m2, cmd := step(t, m, tea.MouseClickMsg{X: first.x + 2, Y: first.y - 1, Button: tea.MouseLeft})
	if cmd != nil || m2.top().kind != viewResults || m2.cursor() != m.cursor() {
		t.Fatal("a click on a section header acted")
	}

	m, cmd = click(t, m, rowZone(2))
	m = settle(t, m, cmd)
	if m.top().kind != viewAlbum || callsOf(f, "Album")[0].Args[0] != "al1" {
		t.Fatalf("top %v; want the album opened", m.top().kind)
	}
	m, _ = press(t, m, "esc")
	if m.cursor() != 2 {
		t.Fatalf("results cursor %d; want the clicked row selected", m.cursor())
	}
}

func TestClickOnMoreTogglesTheNotes(t *testing.T) {
	f := playbacktest.New()
	m := openDaftPunk(t, f)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 60})
	items := m.artistItems()
	more := len(items) - 1
	if items[more].kind != itemMore {
		t.Fatalf("last item %+v; want MORE", items[more])
	}
	if got := textAt(m, zoneOf(t, m, rowZone(more))); !strings.Contains(got, "MORE") {
		t.Fatalf("MORE zone covers %q", got)
	}
	m, _ = click(t, m, rowZone(more))
	if !strings.Contains(plain(m), "helmets off.") {
		t.Fatalf("MORE click did not expand the notes:\n%s", plain(m))
	}
}

func TestClickOnRetryReloadsAFailedPage(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = fullCatalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	f.MethodErr = map[string]error{"SearchCatalog": errors.New("catalog offline")}
	m, cmd := press(t, m, "down", "enter")
	m = settle(t, m, cmd)
	if got := textAt(m, zoneOf(t, m, zoneRetry)); !strings.Contains(got, "RETRY") {
		t.Fatalf("retry zone covers %q", got)
	}
	f.MethodErr = nil
	m, cmd = click(t, m, zoneRetry)
	m = settle(t, m, cmd)
	if !strings.Contains(plain(m), "ONE MORE TIME") {
		t.Fatalf("retry click did not reload:\n%s", plain(m))
	}
}

func TestClickOnRetryRescansStations(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"Playlists": errors.New("offline")}
	m := newModel(t, f, newClock())
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	f.MethodErr = nil
	f.PlaylistsResult = stations()
	m, cmd = click(t, m, zoneRetry)
	m = settle(t, m, cmd)
	if len(m.stations) != 3 {
		t.Fatalf("stations %d; want the rescan loaded", len(m.stations))
	}
}

func TestCompactLayoutKeepsTheButtonsAndTinyDropsThem(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	m, _ = press(t, m, "/")
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 50, Height: 14})
	for _, id := range []string{zoneTabStations, zoneTabSearch, zoneBack, zonePrev, zonePlay, zoneNext, zoneSeek, zoneInput} {
		zoneOf(t, m, id)
	}
	m, _ = click(t, m, zoneBack)
	if m.top().kind != viewStations {
		t.Fatalf("compact back: top %v; want the stations", m.top().kind)
	}
	if got := textAt(m, zoneOf(t, m, rowZone(2))); !strings.Contains(got, "BODY HEAT") {
		t.Fatalf("compact row 2 covers %q", got)
	}

	m, _ = step(t, m, tea.WindowSizeMsg{Width: 18, Height: 4})
	if _, zs := m.layout(); len(zs) != 0 {
		t.Fatalf("tiny layout has zones %v; want none", zs)
	}
}

func TestZonesStayInsideTheFrame(t *testing.T) {
	f := playbacktest.New()
	for _, sz := range []struct{ w, h int }{{20, 5}, {40, 10}, {59, 15}, {60, 16}, {80, 24}, {160, 50}} {
		m := openResults(t, f, &fakeRecents{})
		m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
		m, _ = step(t, m, tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
		_, zs := m.layout()
		for _, z := range zs {
			if z.x < 0 || z.y < 0 || z.w <= 0 || z.x+z.w > sz.w || z.y >= sz.h {
				t.Errorf("%dx%d: zone %+v leaves the frame", sz.w, sz.h, z)
			}
		}
	}
}

func TestNavTabLightsTheViewShown(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	active := func(m Model) []string {
		var ids []string
		for _, b := range m.navButtons() {
			if b.active {
				ids = append(ids, b.id)
			}
		}
		return ids
	}
	if got := active(m); !reflect.DeepEqual(got, []string{zoneTabStations}) {
		t.Fatalf("active tabs on the stations %v", got)
	}
	m, _ = press(t, m, "/")
	if got := active(m); !reflect.DeepEqual(got, []string{zoneTabSearch}) {
		t.Fatalf("active tabs on search %v", got)
	}
	if on, off := (button{label: "X", active: true}).render(), (button{label: "X", tone: stRed}).render(); on == off {
		t.Fatal("the active tab looks like the others")
	}
}

func TestClickOnRecentCrossRemovesOnlyThatTerm(t *testing.T) {
	r := &fakeRecents{}
	m := withThreeRecents(t, r)
	z := zoneOf(t, m, recentDeleteZone(1))
	if got := textAt(m, z); !strings.Contains(got, "✕") {
		t.Fatalf("delete zone covers %q; want the ✕", got)
	}
	if row := zoneOf(t, m, rowZone(1)); row.y != z.y || !strings.Contains(textAt(m, row), "DAFT PUNK") {
		t.Fatalf("✕ of row 1 is not on the DAFT PUNK row")
	}
	m, cmd := click(t, m, recentDeleteZone(1))
	if want := []string{"queen", "samurai"}; !reflect.DeepEqual(m.recents, want) {
		t.Fatalf("recents = %q; want %q", m.recents, want)
	}
	if m.top().kind != viewSearch {
		t.Fatal("clicking ✕ opened the term")
	}
	settle(t, m, cmd)
	if got := r.Removed(); !reflect.DeepEqual(got, []string{"daft punk"}) {
		t.Fatalf("store Remove calls = %q; want [daft punk]", got)
	}
}

func TestClickOnRecentRowStillOpensIt(t *testing.T) {
	m := withThreeRecents(t, &fakeRecents{})
	m, _ = click(t, m, rowZone(1))
	if m.top().kind != viewResults || len(m.recents) != 3 {
		t.Fatalf("view %v, recents %q; want RESULTS with every term kept", m.top().kind, m.recents)
	}
}

func TestClickOnClearRecentClearsEveryTerm(t *testing.T) {
	for _, id := range []string{zoneClearRecents, rowZone(3)} {
		t.Run(id, func(t *testing.T) {
			r := &fakeRecents{}
			m := withThreeRecents(t, r)
			if got := textAt(m, zoneOf(t, m, id)); !strings.Contains(got, "CLEAR RECENT") {
				t.Fatalf("zone covers %q; want CLEAR RECENT", got)
			}
			m, cmd := click(t, m, id)
			if len(m.recents) != 0 || m.top().kind != viewSearch {
				t.Fatalf("recents = %q, view %v; want none, still SEARCH", m.recents, m.top().kind)
			}
			if !strings.Contains(plain(m), "RECENT CLEARED") {
				t.Fatalf("status line lacks the notice:\n%s", plain(m))
			}
			settle(t, m, cmd)
			if r.Clears() != 1 {
				t.Fatalf("store Clear calls = %d; want 1", r.Clears())
			}
			if _, zs := m.layout(); len(zs) > 0 {
				if _, ok := zs.find(zoneClearRecents); ok {
					t.Fatal("CLEAR RECENT button still shown with no terms")
				}
			}
		})
	}
}
