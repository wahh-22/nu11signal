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
	m := loaded(t, f, newClock())
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
	m, _ = click(t, m, zoneTabStations)
	if want := []viewKind{viewStations}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("stations tab: stack %v; want %v", stackKinds(m), want)
	}
	m, _ = click(t, m, zoneTabSearch)
	if want := []viewKind{viewStations, viewSearch, viewArtist}; !reflect.DeepEqual(stackKinds(m), want) || m.cursor() != 1 {
		t.Fatalf("search tab: stack %v cursor %d; want the artist page restored at 1", stackKinds(m), m.cursor())
	}

	// On a page SEARCH goes back to the input, as / does.
	m, _ = click(t, m, zoneTabSearch)
	if m.top().kind != viewSearch || m.cursor() != -1 || !m.input.Focused() {
		t.Fatalf("search tab on a page: top %v cursor %d; want the input", m.top().kind, m.cursor())
	}
	// On SEARCH the tab only takes the input back.
	m, _ = press(t, m, "down")
	m, _ = click(t, m, zoneTabSearch)
	if m.top().kind != viewSearch || m.cursor() != -1 || m.input.Value() != "daft" {
		t.Fatalf("search tab on search: cursor %d input %q; want the input as it was", m.cursor(), m.input.Value())
	}
}

func TestBackButtonShowsAboveTheRootAndPops(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	if _, zs := m.layout(); hasZone(zs, zoneBack) {
		t.Fatal("the stations root offers BACK")
	}
	m = openResults(t, f, &fakeRecents{})
	if got := textAt(m, zoneOf(t, m, zoneBack)); !strings.Contains(got, "BACK") {
		t.Fatalf("back zone covers %q", got)
	}
	m, _ = click(t, m, zoneBack)
	if want := []viewKind{viewStations, viewSearch}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("back: stack %v; want %v", stackKinds(m), want)
	}
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
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 200*time.Second)})
	z := zoneOf(t, m, zoneSeek)
	if got := textAt(m, z); strings.Trim(got, "▮▯") != "" || got == "" {
		t.Fatalf("seek zone covers %q; want only the bar", got)
	}
	dx := z.w / 2
	m, cmd := step(t, m, tea.MouseClickMsg{X: z.x + dx, Y: z.y, Button: tea.MouseLeft})
	settle(t, m, cmd)
	want := 200 * time.Second * time.Duration(dx) / time.Duration(z.w)
	assertCall(t, f, "Seek", want)

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

// S2 review follow-ups.

func TestShortRecentTermOnlyFillsTheInput(t *testing.T) {
	f := playbacktest.New()
	r := &fakeRecents{terms: []string{"x", "queen"}}
	m := loadedWithRecents(t, f, r)
	m, _ = press(t, m, "/", "down")
	m, cmd := press(t, m, "enter")
	if cmd != nil {
		m = settle(t, m, cmd)
	}
	if m.top().kind != viewSearch || m.input.Value() != "x" || m.cursor() != -1 {
		t.Fatalf("top %v input %q cursor %d; want the term in the input only", m.top().kind, m.input.Value(), m.cursor())
	}
	if len(catalogCalls(f)) != 0 || len(r.Added()) != 0 {
		t.Fatalf("a too short term searched %v or was saved %v", catalogCalls(f), r.Added())
	}
	if !strings.Contains(plain(m), "KEEP TYPING") {
		t.Fatalf("no keep-typing notice:\n%s", plain(m))
	}
}

func TestEscOntoSearchKeepsResultsThatAnswerTheInput(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = fullCatalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	// Enter on the input opens RESULTS for the very term the live rows
	// answer.
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	before := len(catalogCalls(f))
	m, cmd = press(t, m, "esc")
	if cmd != nil {
		m = settle(t, m, cmd)
	}
	if calls := catalogCalls(f); len(calls) != before {
		t.Fatalf("esc searched again: %v", calls[before:])
	}
	if !strings.Contains(plain(m), "ONE MORE TIME") {
		t.Fatalf("live rows not shown:\n%s", plain(m))
	}
}

// linesOf drops the zones of a body renderer.
func linesOf(lines []string, _ zones) []string { return lines }

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
