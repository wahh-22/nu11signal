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

// longNotes wraps to more lines than the collapsed ABOUT shows at any
// width the tests use.
const longNotes = "Thomas Bangalter and Guy-Manuel de Homem-Christo met at school in Paris. " +
	"Their first band, Darlin', was dismissed by a critic as daft punky thrash, and they kept the insult. " +
	"Homework and Discovery turned French house into a global language. " +
	"Final transmission: the robots never took their helmets off."

func artistDetail() playback.ArtistDetail {
	album := func(id, title string, year int) playback.Album {
		return playback.Album{ID: id, Title: title, Artist: "Daft Punk", Year: year, TrackCount: 14}
	}
	return playback.ArtistDetail{
		Artist: playback.Artist{ID: "a1", Name: "Daft Punk", Genres: []string{"Electronic"}},
		TopSongs: []playback.Song{
			{ID: "s1", Title: "One More Time", Artist: "Daft Punk", Album: "Discovery"},
			{ID: "s2", Title: "Get Lucky", Artist: "Daft Punk", Album: "Random Access Memories"},
			{ID: "s3", Title: "Around the World", Artist: "Daft Punk", Album: "Homework"},
		},
		EssentialAlbums: []playback.Album{album("al1", "Discovery", 2001)},
		Albums:          []playback.Album{album("al1", "Discovery", 2001), album("al2", "Homework", 1997)},
		Singles:         []playback.Album{album("sg1", "Get Lucky (Radio Edit)", 2013)},
		Playlists:       []playback.CatalogPlaylist{{ID: "pl1", Name: "Daft Punk Essentials", Curator: "Apple Music Electronic"}},
		About:           playback.ArtistAbout{Notes: longNotes, Genre: "Electronic", Origin: "Paris, France", Formed: "1993"},
	}
}

// openDaftPunk searches "daft", opens the artist row and lets the page load.
func openDaftPunk(t *testing.T, f *playbacktest.Fake) Model {
	t.Helper()
	f.SearchCatalogResult = catalog()
	f.ArtistResult = artistDetail()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "down", "down", "down")
	m, cmd := press(t, m, "enter")
	return settle(t, m, cmd)
}

func artistCalls(f *playbacktest.Fake) []playbacktest.Call {
	var out []playbacktest.Call
	for _, c := range f.Calls() {
		if c.Method == "Artist" {
			out = append(out, c)
		}
	}
	return out
}

func TestSearchEnterOnArtistOpensItsPage(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.ArtistResult = artistDetail()
	r := &fakeRecents{}
	m := searchFor(t, loadedWithRecents(t, f, r), "daft")

	m, _ = press(t, m, "down", "down", "down")
	m, cmd := press(t, m, "enter")
	if m.top().kind != viewArtist {
		t.Fatalf("top view = %v; want the artist view", m.top().kind)
	}
	if kinds := []viewKind{m.stack[0].kind, m.stack[1].kind}; !reflect.DeepEqual(kinds, []viewKind{viewStations, viewSearch}) || len(m.stack) != 3 {
		t.Fatalf("stack = %v; want stations, search, artist", m.stack)
	}
	// The name from the search row heads the page while it loads.
	if view := plain(m); !strings.Contains(view, "DAFT PUNK") || !strings.Contains(view, "DECRYPTING") {
		t.Fatalf("loading page lacks the name or the loading notice:\n%s", view)
	}
	m = settle(t, m, cmd)
	if calls := artistCalls(f); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{"a1"}) {
		t.Fatalf("Artist calls = %v; want one for a1", calls)
	}
	if got := r.Added(); !reflect.DeepEqual(got, []string{"daft"}) {
		t.Fatalf("recents added = %q; want [daft]", got)
	}
	if strings.Contains(plain(m), "DECRYPTING") {
		t.Fatalf("loaded page still loading:\n%s", plain(m))
	}
}

func TestArtistPageShowsSectionsInAppleMusicOrder(t *testing.T) {
	f := playbacktest.New()
	m := openDaftPunk(t, f)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 160, Height: 60})

	view := plain(m)
	order := []string{
		"T O P   S O N G S", "ONE MORE TIME · DISCOVERY",
		"E S S E N T I A L   A L B U M S", "DISCOVERY · 2001",
		"A L B U M S", "HOMEWORK · 1997",
		"A R T I S T   P L A Y L I S T S", "DAFT PUNK ESSENTIALS · APPLE MUSIC ELECTRONIC",
		"S I N G L E S   &   E P S", "GET LUCKY (RADIO EDIT) · 2013",
		"A B O U T", "Thomas Bangalter", "▸ MORE",
		"FROM", "PARIS, FRANCE", "FORMED", "1993", "GENRE", "ELECTRONIC",
	}
	last := -1
	for _, want := range order {
		i := strings.Index(view[last+1:], want)
		if i < 0 {
			t.Fatalf("view missing %q after position %d:\n%s", want, last, view)
		}
		last += 1 + i
	}
	if strings.Contains(view, "C O M P I L A T I O N S") {
		t.Errorf("the empty compilations section is shown:\n%s", view)
	}
}

func TestArtistPageSkipsEmptySectionsAndAboutFacts(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.ArtistResult = playback.ArtistDetail{
		Artist:       playback.Artist{ID: "a1", Name: "Daft Punk"},
		Compilations: []playback.Album{{ID: "c1", Title: "Musique"}},
		About:        playback.ArtistAbout{Genre: "Electronic"},
	}
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "down", "down", "down")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)

	view := plain(m)
	for _, want := range []string{"C O M P I L A T I O N S", "MUSIQUE", "A B O U T", "GENRE"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	// An album without a year shows no " · 0".
	for _, absent := range []string{"T O P   S O N G S", "S I N G L E S", "FROM", "FORMED", "▸ MORE", "MUSIQUE ·"} {
		if strings.Contains(view, absent) {
			t.Errorf("view shows %q:\n%s", absent, view)
		}
	}
}

func TestArtistPageEmptyShowsNoData(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.ArtistResult = playback.ArtistDetail{Artist: playback.Artist{ID: "a1", Name: "Daft Punk"}}
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "down", "down", "down")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	if view := plain(m); !strings.Contains(view, "NO DATA ON FILE") {
		t.Fatalf("empty page lacks its notice:\n%s", view)
	}
	// Nothing to select: enter and moving do nothing.
	before := len(f.Calls())
	m, cmd = press(t, m, "down", "enter")
	if cmd != nil {
		settle(t, m, cmd)
	}
	if len(f.Calls()) != before {
		t.Fatalf("enter on an empty page called the player: %v", f.Calls()[before:])
	}
}

func TestArtistPageFailureOffersRetry(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.ArtistResult = artistDetail()
	f.MethodErr = map[string]error{"Artist": errors.New("catalog offline")}
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "down", "down", "down")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)

	view := plain(m)
	if !strings.Contains(view, "[R] RETRY") || !strings.Contains(view, "CATALOG OFFLINE") {
		t.Fatalf("failed page lacks the retry notice or the reason:\n%s", view)
	}
	f.MethodErr = nil
	m, cmd = press(t, m, "r")
	if !strings.Contains(plain(m), "DECRYPTING") {
		t.Fatalf("retry does not show loading:\n%s", plain(m))
	}
	m = settle(t, m, cmd)
	if calls := artistCalls(f); len(calls) != 2 {
		t.Fatalf("Artist calls = %d; want a retry", len(calls))
	}
	if !strings.Contains(plain(m), "ONE MORE TIME") {
		t.Fatalf("retried page not shown:\n%s", plain(m))
	}
}

func TestArtistCursorSkipsHeadersAndPlaysTopSongsFromSelection(t *testing.T) {
	f := playbacktest.New()
	m := openDaftPunk(t, f)

	if m.cursor() != 0 {
		t.Fatalf("cursor = %d; want the first top song", m.cursor())
	}
	m, _ = press(t, m, "down", "j")
	m, cmd := press(t, m, "enter")
	settle(t, m, cmd)
	assertCall(t, f, "PlaySongs", []string{"s1", "s2", "s3"}, 2)

	// Past the top songs the cursor lands on the essential album, not on
	// the blank line or the section header between them.
	m, _ = press(t, m, "down")
	items := m.artistItems()
	if it := items[m.cursor()]; it.kind != itemAlbum || it.album.ID != "al1" {
		t.Fatalf("item under cursor = %+v; want the essential album", it)
	}
	for range 30 {
		m, _ = press(t, m, "down")
	}
	if m.cursor() != len(items)-1 {
		t.Fatalf("cursor = %d past the last item; want %d", m.cursor(), len(items)-1)
	}
}

func TestArtistAboutMoreTogglesFullNotes(t *testing.T) {
	f := playbacktest.New()
	m := openDaftPunk(t, f)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 60})

	if view := plain(m); strings.Contains(view, "helmets off") || !strings.Contains(view, "▸ MORE") {
		t.Fatalf("collapsed notes show everything or lack MORE:\n%s", view)
	}
	for m.artistItems()[m.cursor()].kind != itemMore {
		m, _ = press(t, m, "down")
	}
	m, _ = press(t, m, "enter")
	view := plain(m)
	if !strings.Contains(view, "helmets off.") || !strings.Contains(view, "▴ LESS") {
		t.Fatalf("expanded notes lack the end or LESS:\n%s", view)
	}
	m, _ = press(t, m, "enter")
	if strings.Contains(plain(m), "helmets off") {
		t.Fatalf("LESS did not collapse the notes:\n%s", plain(m))
	}
}

func TestArtistShortNotesNeedNoMore(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.ArtistResult = artistDetail()
	f.ArtistResult.About.Notes = "French duo."
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "down", "down", "down")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	for _, it := range m.artistItems() {
		if it.kind == itemMore {
			t.Fatal("notes that fit offer MORE")
		}
	}
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 160, Height: 60})
	if !strings.Contains(plain(m), "French duo.") {
		t.Fatalf("short notes not shown:\n%s", plain(m))
	}
}

func TestArtistEscReturnsToSearchIntact(t *testing.T) {
	f := playbacktest.New()
	m := openDaftPunk(t, f)
	searches := len(catalogCalls(f))

	m, cmd := press(t, m, "esc")
	if m.top().kind != viewSearch || len(m.stack) != 2 {
		t.Fatalf("esc left stack %v; want stations, search", m.stack)
	}
	if m.input.Value() != "daft" || m.cursor() != 2 || !m.input.Focused() {
		t.Fatalf("search came back as input %q, cursor %d, focused %v; want daft, 2, true", m.input.Value(), m.cursor(), m.input.Focused())
	}
	if cmd != nil {
		m = settle(t, m, cmd)
	}
	if !strings.Contains(plain(m), "ONE MORE TIME") || len(catalogCalls(f)) != searches {
		t.Fatalf("results not kept (searches %d -> %d):\n%s", searches, len(catalogCalls(f)), plain(m))
	}
	// Enter on the same row opens the page again.
	m, cmd = press(t, m, "enter")
	settle(t, m, cmd)
	if m.top().kind != viewArtist || len(artistCalls(f)) != 2 {
		t.Fatalf("re-entering the artist: top %v, Artist calls %d", m.top().kind, len(artistCalls(f)))
	}
}

func TestArtistTabAndSlashLeaveThePage(t *testing.T) {
	f := playbacktest.New()
	m := openDaftPunk(t, f)
	next, _ := press(t, m, "tab")
	if len(next.stack) != 1 || next.top().kind != viewStations {
		t.Fatalf("tab left stack %v; want the stations root", next.stack)
	}
	next, _ = press(t, m, "/")
	if len(next.stack) != 2 || next.top().kind != viewSearch || next.input.Value() != "daft" {
		t.Fatalf("/ left stack %v, input %q; want the search input with the term kept", next.stack, next.input.Value())
	}
	// Player keys still work on the artist page.
	if _, cmd := press(t, m, "n"); cmd == nil {
		t.Fatal("n did nothing on the artist page")
	}
}

// blockingArtistPlayer hands every Artist context to the test and blocks
// until that context is done.
type blockingArtistPlayer struct {
	*playbacktest.Fake
	ctxs chan context.Context
}

func (p *blockingArtistPlayer) Artist(ctx context.Context, _ string) (playback.ArtistDetail, error) {
	p.ctxs <- ctx
	<-ctx.Done()
	return playback.ArtistDetail{}, ctx.Err()
}

func TestLeavingALoadingArtistPageCancelsIt(t *testing.T) {
	// tab parks the page with its load instead (see resume_test.go).
	for _, k := range []string{"esc", "/"} {
		t.Run(k, func(t *testing.T) {
			f := playbacktest.New()
			f.PlaylistsResult = stations()
			f.SearchCatalogResult = catalog()
			p := &blockingArtistPlayer{Fake: f, ctxs: make(chan context.Context, 1)}
			m := New(p, Options{Now: newClock().now, Seed: 2077})
			m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
			m = searchFor(t, m, "daft")
			m, _ = press(t, m, "down", "down", "down")
			m, cmd := press(t, m, "enter")
			done := make(chan tea.Msg, 4)
			launch(cmd, done)
			var ctx context.Context
			select {
			case ctx = <-p.ctxs:
			case <-time.After(2 * time.Second):
				t.Fatal("the artist load never reached the player")
			}
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) < defaultCallTimeout {
				t.Errorf("artist deadline in %v; want the longer artist timeout", time.Until(deadline))
			}

			m, _ = press(t, m, k)
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("leaving the page did not cancel its load")
			}
			// The cancelled answer is dropped, not reported.
			for answered := false; !answered; {
				select {
				case msg := <-done:
					_, answered = msg.(artistMsg)
					m, _ = step(t, m, msg)
				case <-time.After(2 * time.Second):
					t.Fatal("the cancelled load never answered")
				}
			}
			if strings.Contains(m.status, "ARTIST") {
				t.Fatalf("a cancelled load was reported: %q", m.status)
			}
		})
	}
}

func TestArtistAnswerForAnotherPageIsDropped(t *testing.T) {
	f := playbacktest.New()
	m := openDaftPunk(t, f)
	stale := artistMsg{seq: m.top().artist.seq - 1, detail: playback.ArtistDetail{Artist: playback.Artist{Name: "Stale Band"}}}
	m, _ = step(t, m, stale)
	if strings.Contains(plain(m), "STALE BAND") {
		t.Fatal("an answer for another page was shown")
	}
}

func TestArtistViewFitsEverySize(t *testing.T) {
	sizes := []struct{ w, h int }{{1, 1}, {10, 3}, {30, 8}, {40, 10}, {59, 15}, {60, 16}, {80, 24}, {160, 50}}
	for _, sz := range sizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			f := playbacktest.New()
			m := openDaftPunk(t, f)
			m.stack[len(m.stack)-1].artist.detail.Artist.Name = "宇多田ヒカル Daft Punk Tribute Orchestra"
			m, _ = press(t, m, "down", "down", "down", "down")
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
			if sz.w >= 60 && sz.h >= 16 && !strings.Contains(strings.Join(lines, "\n"), "NOW PLAYING") {
				t.Errorf("full layout lost NOW PLAYING:\n%s", strings.Join(lines, "\n"))
			}
		})
	}
}

func TestArtistBodyLinesFillWidth(t *testing.T) {
	f := playbacktest.New()
	m := openDaftPunk(t, f)
	for _, w := range []int{9, 30, 46} {
		for cur := range len(m.artistItems()) {
			m.stack[len(m.stack)-1].cursor = cur
			for i, row := range linesOf(m.artistBody(w, 12)) {
				if got := ansi.StringWidth(row); got != w {
					t.Errorf("w=%d cursor=%d: row %d is %d cells: %q", w, cur, i, got, ansi.Strip(row))
				}
			}
		}
	}
}

func TestArtistViewGolden(t *testing.T) {
	tests := []struct {
		name  string
		w, h  int
		setup func(t *testing.T, m Model) Model
	}{
		{"artist_80x24", 80, 24, func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "down")
			return m
		}},
		{"artist_about_120x40", 120, 40, func(t *testing.T, m Model) Model {
			for m.artistItems()[m.cursor()].kind != itemMore {
				m, _ = press(t, m, "down")
			}
			m, _ = press(t, m, "enter")
			return m
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			m := openDaftPunk(t, f)
			m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
			for i := 0; i < 12; i++ {
				m, _ = step(t, m, tickMsg{gen: m.tickGen})
			}
			m, _ = step(t, m, tea.WindowSizeMsg{Width: tt.w, Height: tt.h})
			m = tt.setup(t, m)
			assertGolden(t, tt.name+".golden", ansi.Strip(m.View().Content))
		})
	}
}
