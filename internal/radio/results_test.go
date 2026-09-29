package radio

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// fullCatalog is catalog() with every section of the RESULTS page: top
// results of each kind, albums and playlists.
func fullCatalog() playback.SearchResults {
	res := catalog()
	discovery := playback.Album{ID: "al1", Title: "Discovery", Artist: "Daft Punk", Year: 2001, TrackCount: 14}
	homework := playback.Album{ID: "al2", Title: "Homework", Artist: "Daft Punk", Year: 1997, TrackCount: 16}
	essentials := playback.CatalogPlaylist{ID: "pl1", Name: "Daft Punk Essentials", Curator: "Apple Music Electronic"}
	res.Top = []playback.SearchItem{
		{Kind: playback.ItemArtist, Artist: res.Artists[0]},
		{Kind: playback.ItemSong, Song: res.Songs[0]},
		{Kind: playback.ItemAlbum, Album: discovery},
		{Kind: playback.ItemPlaylist, Playlist: essentials},
	}
	res.Albums = []playback.Album{discovery, homework}
	res.Playlists = []playback.CatalogPlaylist{essentials}
	return res
}

// openResults searches "daft", enters the first suggestion ("daft punk")
// and lets the RESULTS page load.
func openResults(t *testing.T, f *playbacktest.Fake, r *fakeRecents) Model {
	t.Helper()
	f.SearchCatalogResult = fullCatalog()
	m := searchFor(t, loadedWithRecents(t, f, r), "daft")
	m, _ = press(t, m, "down")
	m, cmd := press(t, m, "enter")
	return settle(t, m, cmd)
}

// header is a section header as a page renders it, unstyled.
func header(name string) string { return spaced(name) }

func TestSuggestionEnterOpensResultsWithSectionsInOrder(t *testing.T) {
	f := playbacktest.New()
	r := &fakeRecents{}
	m := openResults(t, f, r)
	// Room for every section.
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 50})

	if want := []viewKind{viewStations, viewSearch, viewResults}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("stack %v; want %v", stackKinds(m), want)
	}
	calls := catalogCalls(f)
	if got := calls[len(calls)-1].Args; !reflect.DeepEqual(got, []any{"daft punk", resultsLimit}) {
		t.Fatalf("RESULTS searched %v; want the suggestion with the results page size", got)
	}
	if got := r.Added(); !reflect.DeepEqual(got, []string{"daft punk"}) {
		t.Fatalf("recents added = %q; want the suggestion", got)
	}
	if m.input.Value() != "daft punk" || m.input.Focused() {
		t.Fatalf("input %q focused %v; want the suggestion kept, not taking keys", m.input.Value(), m.input.Focused())
	}

	view := plain(m)
	at := -1
	for _, s := range []string{"RESULTS", "DAFT PUNK", header("TOP RESULTS"), header("ARTISTS"), header("ALBUMS"), header("SONGS"), header("PLAYLISTS")} {
		i := strings.Index(view[at+1:], s)
		if i < 0 {
			t.Fatalf("%q missing or out of order:\n%s", s, view)
		}
		at += 1 + i
	}
	for _, row := range []string{"HOMEWORK · DAFT PUNK · 1997", "DIGITAL LOVE · DAFT PUNK", "DAFT PUNK ESSENTIALS · APPLE MUSIC ELECTRONIC"} {
		if !strings.Contains(view, row) {
			t.Errorf("view lacks row %q:\n%s", row, view)
		}
	}
	// Four top results, one artist, two albums, two songs, one playlist.
	if n := len(m.resultItems()); n != 10 {
		t.Fatalf("selectable rows = %d; want 10", n)
	}
	if m.cursor() != 0 {
		t.Fatalf("cursor = %d; want the first row", m.cursor())
	}
}

func TestResultsLeaveOutEmptySections(t *testing.T) {
	f := playbacktest.New()
	m := openResults(t, f, &fakeRecents{})
	f.SearchCatalogResult = playback.SearchResults{Songs: catalog().Songs}
	m, cmd := press(t, m, "esc", "enter")
	m = settle(t, m, cmd)

	view := plain(m)
	if !strings.Contains(view, header("SONGS")) {
		t.Fatalf("songs section missing:\n%s", view)
	}
	for _, s := range []string{"TOP RESULTS", "ARTISTS", "ALBUMS", "PLAYLISTS"} {
		if strings.Contains(view, header(s)) {
			t.Errorf("empty section %s shown:\n%s", s, view)
		}
	}

	f.SearchCatalogResult = playback.SearchResults{}
	m, cmd = press(t, m, "esc", "enter")
	m = settle(t, m, cmd)
	if !strings.Contains(plain(m), "NO DATA ON FILE") {
		t.Fatalf("empty results lack their notice:\n%s", plain(m))
	}
}

func TestRecentTermAndInputEnterOpenResults(t *testing.T) {
	t.Run("recent term", func(t *testing.T) {
		f := playbacktest.New()
		f.SearchCatalogResult = fullCatalog()
		r := &fakeRecents{terms: []string{"queen", "abba"}}
		m := loadedWithRecents(t, f, r)
		m, _ = press(t, m, "/", "down", "down")
		m, cmd := press(t, m, "enter")
		m = settle(t, m, cmd)
		if m.top().kind != viewResults || m.input.Value() != "abba" {
			t.Fatalf("top %v input %q; want RESULTS for the recent term", m.top().kind, m.input.Value())
		}
		if calls := catalogCalls(f); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{"abba", resultsLimit}) {
			t.Fatalf("SearchCatalog calls = %v; want one RESULTS search for abba", calls)
		}
		if got := r.Added(); !reflect.DeepEqual(got, []string{"abba"}) {
			t.Fatalf("recents added = %q; want the term moved to the front", got)
		}
	})
	t.Run("input", func(t *testing.T) {
		f := playbacktest.New()
		f.SearchCatalogResult = fullCatalog()
		r := &fakeRecents{}
		m := loadedWithRecents(t, f, r)
		m, _ = press(t, m, "/")
		m = typeText(t, m, "queen")
		m, cmd := press(t, m, "enter")
		m = settle(t, m, cmd)
		if m.top().kind != viewResults || !strings.Contains(plain(m), "QUEEN") {
			t.Fatalf("enter on the input did not open RESULTS for the term:\n%s", plain(m))
		}
		if f.Closed() {
			t.Fatal("typing q in the search input quit the player")
		}
		if calls := catalogCalls(f); len(calls) != 1 || calls[0].Args[0] != "queen" {
			t.Fatalf("SearchCatalog calls = %v; want one, without waiting for the debounce", calls)
		}
		if got := r.Added(); !reflect.DeepEqual(got, []string{"queen"}) {
			t.Fatalf("recents added = %q; want [queen]", got)
		}
	})
}

func TestResultsRowsOpenTheirViews(t *testing.T) {
	tests := []struct {
		name   string
		pick   func(items []playback.SearchItem) int
		kind   viewKind
		title  string
		method string
		arg    string
		// key opens the row: enter, or the album key on a song row
		// (enter plays it, see queue_test.go).
		key string
	}{
		{"top artist", firstOf(playback.ItemArtist, 0), viewArtist, "ARTIST", "Artist", "a1", ""},
		{"top song", firstOf(playback.ItemSong, 0), viewAlbum, "SONG", "SongAlbum", "s1", keyAlbum},
		{"top album", firstOf(playback.ItemAlbum, 0), viewAlbum, "ALBUM", "Album", "al1", ""},
		{"top playlist", firstOf(playback.ItemPlaylist, 0), viewPlaylist, "PLAYLIST", "CatalogPlaylist", "pl1", ""},
		{"artist", firstOf(playback.ItemArtist, 4), viewArtist, "ARTIST", "Artist", "a1", ""},
		{"second album", firstOf(playback.ItemAlbum, 6), viewAlbum, "ALBUM", "Album", "al2", ""},
		{"second song", firstOf(playback.ItemSong, 8), viewAlbum, "SONG", "SongAlbum", "s2", keyAlbum},
		{"playlist", firstOf(playback.ItemPlaylist, 9), viewPlaylist, "PLAYLIST", "CatalogPlaylist", "pl1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			f.ArtistResult = artistDetail()
			f.AlbumResult = discovery()
			f.SongAlbumResult = discovery()
			f.CatalogPlaylistResult = essentials()
			m := openResults(t, f, &fakeRecents{})
			target := tt.pick(m.resultItems())
			for m.cursor() < target {
				m, _ = press(t, m, "down")
			}
			if tt.key == "" {
				tt.key = "enter"
			}
			m, cmd := press(t, m, tt.key)
			m = settle(t, m, cmd)

			if want := []viewKind{viewStations, viewSearch, viewResults, tt.kind}; !reflect.DeepEqual(stackKinds(m), want) {
				t.Fatalf("stack %v; want %v", stackKinds(m), want)
			}
			if title, _, _, _ := m.listView(40, 10); title != tt.title {
				t.Errorf("panel title %q; want %q", title, tt.title)
			}
			if calls := callsOf(f, tt.method); len(calls) != 1 || calls[0].Args[0] != tt.arg {
				t.Fatalf("%s calls = %v; want one for %s", tt.method, calls, tt.arg)
			}
			// esc comes back to the results as they were.
			m, _ = press(t, m, "esc")
			if m.top().kind != viewResults || m.cursor() != target {
				t.Fatalf("esc: top %v cursor %d; want RESULTS at %d", m.top().kind, m.cursor(), target)
			}
		})
	}
}

// firstOf picks the first row of kind at or after row from.
func firstOf(kind playback.SearchItemKind, from int) func([]playback.SearchItem) int {
	return func(items []playback.SearchItem) int {
		for i := from; i < len(items); i++ {
			if items[i].Kind == kind {
				return i
			}
		}
		panic(fmt.Sprintf("no %s row from %d", kind, from))
	}
}

func TestResultsEscReturnsToSearchWithTheTermKept(t *testing.T) {
	f := playbacktest.New()
	m := openResults(t, f, &fakeRecents{})
	before := len(catalogCalls(f))

	m, cmd := press(t, m, "esc")
	m = settle(t, m, cmd)
	if want := []viewKind{viewStations, viewSearch}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("stack %v; want %v", stackKinds(m), want)
	}
	if m.input.Value() != "daft punk" || !m.input.Focused() || m.cursor() != -1 {
		t.Fatalf("input %q focused %v cursor %d; want the term kept on the input", m.input.Value(), m.input.Focused(), m.cursor())
	}
	// The live results follow the new term.
	calls := catalogCalls(f)
	if len(calls) != before+1 || !reflect.DeepEqual(calls[len(calls)-1].Args, []any{"daft punk", searchLimit}) {
		t.Fatalf("SearchCatalog calls after esc = %v; want one live search for the kept term", calls[before:])
	}
	if !strings.Contains(plain(m), "ONE MORE TIME") {
		t.Fatalf("live results not shown:\n%s", plain(m))
	}
}

func TestResultsAreParkedAndRestored(t *testing.T) {
	f := playbacktest.New()
	m := openResults(t, f, &fakeRecents{})
	m, _ = press(t, m, "down", "down")
	before := len(catalogCalls(f))

	m, _ = press(t, m, "tab")
	if want := []viewKind{viewStations}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("tab left stack %v; want the stations root", stackKinds(m))
	}
	m, cmd := press(t, m, "/")
	if cmd != nil {
		m = settle(t, m, cmd)
	}
	if want := []viewKind{viewStations, viewSearch, viewResults}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("restored stack %v; want %v", stackKinds(m), want)
	}
	if m.cursor() != 2 || len(catalogCalls(f)) != before {
		t.Fatalf("cursor %d, %d new searches; want the parked page as it was", m.cursor(), len(catalogCalls(f))-before)
	}
}

func TestParkedResultsSettleTheirLoad(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = fullCatalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, load := press(t, m, "down", "enter")
	m, _ = press(t, m, "tab")
	m = settle(t, m, load)
	m, _ = press(t, m, "/")
	if m.top().kind != viewResults || m.top().results.loading || !strings.Contains(plain(m), "HOMEWORK") {
		t.Fatalf("restored results did not settle:\n%s", plain(m))
	}
}

func TestResultsFailureOffersRetry(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = fullCatalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	f.MethodErr = map[string]error{"SearchCatalog": errors.New("catalog offline")}
	m, cmd := press(t, m, "down", "enter")
	m = settle(t, m, cmd)

	if view := plain(m); !strings.Contains(view, "[R] RETRY") || !strings.Contains(view, "CATALOG OFFLINE") {
		t.Fatalf("failed results lack the retry notice or the reason:\n%s", view)
	}
	if !strings.Contains(m.status, "SEARCH FAILED") {
		t.Fatalf("status %q; want the failure", m.status)
	}
	f.MethodErr = nil
	m, cmd = press(t, m, "r")
	if !strings.Contains(plain(m), "DECRYPTING") {
		t.Fatalf("retry does not show loading:\n%s", plain(m))
	}
	m = settle(t, m, cmd)
	if calls := catalogCalls(f); !reflect.DeepEqual(calls[len(calls)-1].Args, []any{"daft punk", resultsLimit}) {
		t.Fatalf("retry searched %v; want the same term", calls[len(calls)-1].Args)
	}
	if !strings.Contains(plain(m), "HOMEWORK") {
		t.Fatalf("retried results not shown:\n%s", plain(m))
	}
}

func TestLeavingLoadingResultsCancelsThem(t *testing.T) {
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	f.SearchCatalogResult = fullCatalog()
	p := &blockingPlayer{Fake: f, ctxs: make(chan context.Context, 4)}
	m := New(p, Options{Now: newClock().now, Seed: 2077})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = press(t, m, "/")
	m = typeText(t, m, "daft")
	m, cmd := press(t, m, "enter")
	if m.top().kind != viewResults {
		t.Fatalf("top %v; want RESULTS", m.top().kind)
	}
	done := make(chan tea.Msg, 4)
	launch(cmd, done)
	var ctx context.Context
	select {
	case ctx = <-p.ctxs:
	case <-time.After(2 * time.Second):
		t.Fatal("the results search never reached the player")
	}
	m, _ = press(t, m, "esc")
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("leaving the results did not cancel their search")
	}
	for answered := false; !answered; {
		select {
		case msg := <-done:
			_, answered = msg.(resultsMsg)
			m, _ = step(t, m, msg)
		case <-time.After(2 * time.Second):
			t.Fatal("the cancelled search never answered")
		}
	}
	if strings.Contains(m.status, "FAILED") {
		t.Fatalf("a cancelled results search was reported: %q", m.status)
	}
}

func TestResultsViewFitsEverySize(t *testing.T) {
	sizes := []struct{ w, h int }{{1, 1}, {10, 3}, {30, 8}, {40, 10}, {59, 15}, {80, 24}, {160, 50}}
	for _, sz := range sizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			f := playbacktest.New()
			m := openResults(t, f, &fakeRecents{})
			// Wide characters take two cells each.
			top := m.top()
			top.results.found.Top = append(top.results.found.Top, playback.SearchItem{Kind: playback.ItemSong, Song: playback.Song{ID: "s9", Title: "初恋", Artist: "宇多田ヒカル"}})
			m.setTop(top)
			m, _ = press(t, m, "down", "down", "down", "down", "down")
			m, _ = step(t, m, tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			if len(lines) > sz.h {
				t.Errorf("rendered %d lines, want at most %d", len(lines), sz.h)
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > sz.w {
					t.Errorf("line %d is %d cells wide, want at most %d: %q", i, w, sz.w, l)
				}
			}
		})
	}
}

func TestResultsBodyLinesFillWidth(t *testing.T) {
	f := playbacktest.New()
	m := openResults(t, f, &fakeRecents{})
	for _, w := range []int{9, 30, 70} {
		for cur := 0; cur < len(m.resultItems()); cur++ {
			m.stack[len(m.stack)-1].cursor = cur
			for i, row := range linesOf(m.resultsBody(w, 40)) {
				if got := ansi.StringWidth(row); got != w {
					t.Errorf("w=%d cursor=%d: row %d is %d cells: %q", w, cur, i, got, ansi.Strip(row))
				}
			}
		}
	}
}

func TestResultsViewGolden(t *testing.T) {
	f := playbacktest.New()
	m := openResults(t, f, &fakeRecents{})
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	for i := 0; i < 12; i++ {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	m, _ = press(t, m, "down", "down")
	assertGolden(t, "results_80x24.golden", ansi.Strip(m.View().Content))
}

// The S1 review advisories: parked pages and their loads.

func TestParkedPageFailureWaitsForTheRestore(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.MethodErr = map[string]error{"Artist": errors.New("catalog offline")}
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, load := press(t, m, "down", "down", "down", "enter")

	m, _ = press(t, m, "tab")
	m = settle(t, m, load)
	if strings.Contains(m.status, "FAILED") {
		t.Fatalf("a parked page's failure reached the stations: %q", m.status)
	}
	m, _ = press(t, m, "/")
	if !strings.Contains(m.status, "ARTIST FEED FAILED") || !strings.Contains(plain(m), "[R] RETRY") {
		t.Fatalf("restored failed page: status %q\n%s", m.status, plain(m))
	}
}

func TestParkedTrackPagesSettleTheirLoads(t *testing.T) {
	for _, kind := range []artistItemKind{itemAlbum, itemPlaylist} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			f := playbacktest.New()
			f.AlbumResult = discovery()
			f.CatalogPlaylistResult = essentials()
			m := openDaftPunk(t, f)
			for m.artistItems()[m.cursor()].kind != kind {
				m, _ = press(t, m, "down")
			}
			m, load := press(t, m, "enter")
			m, _ = press(t, m, "tab")
			m = settle(t, m, load)
			m, _ = press(t, m, "/")
			if m.top().tracks.loading || m.top().tracks.err != nil || len(m.top().tracks.tracks()) == 0 {
				t.Fatalf("restored page did not settle: %+v\n%s", m.top().tracks, plain(m))
			}
		})
	}
}

func TestParkingCancelsTheBranchItReplaces(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "/")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// An older parked branch still loading; parking again replaces it.
	m.parked = []frame{{kind: viewSearch, cursor: -1}, {kind: viewArtist, artist: artistPage{loading: true, cancel: cancel}}}
	m, _ = press(t, m, "tab")
	if ctx.Err() == nil {
		t.Fatal("the replaced parked page kept loading")
	}
	if len(m.parked) != 1 || m.parked[0].kind != viewSearch {
		t.Fatalf("parked %v; want the search just left", m.parked)
	}
}

func TestResultsTextNeverReachesTheTerminalRaw(t *testing.T) {
	const evil = "\x1b]0;pwned\x07"
	f := playbacktest.New()
	res := fullCatalog()
	res.Top[2].Album.Title += evil
	res.Albums[1].Artist += evil
	res.Playlists[0].Curator += evil
	f.SearchCatalogResult = res
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, cmd := press(t, m, "down", "enter")
	m = settle(t, m, cmd)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 50})
	if view := m.render(); strings.Contains(view, "\x07") || !strings.Contains(plain(m), "DISCOVERY]0;PWNED") {
		t.Fatalf("results page not cleaned:\n%q", plain(m))
	}
}

// Going back from RESULTS onto SEARCH keeps the live rows when they
// already answer the input.

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
