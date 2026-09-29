package radio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// albumNotes wraps to more lines than the collapsed notes show at any width
// the tests use.
const albumNotes = "Discovery is the second studio album by Daft Punk, a tribute to the music of their childhood. " +
	"Sampling disco, funk and soft rock, it pairs house grooves with pop hooks and Leiji Matsumoto's anime. " +
	"Final track: Too Long, ten minutes of robots refusing to stop."

func discovery() playback.AlbumDetail {
	track := func(id, title string, secs, n int) playback.Track {
		return playback.Track{Song: playback.Song{ID: id, Title: title, Artist: "Daft Punk", Album: "Discovery", Duration: time.Duration(secs) * time.Second}, Number: n, Disc: 1}
	}
	return playback.AlbumDetail{
		Album: playback.Album{ID: "al1", Title: "Discovery", Artist: "Daft Punk", Year: 2001, TrackCount: 4},
		Tracks: []playback.Track{
			track("s1", "One More Time", 320, 1),
			track("s4", "Aerodynamic", 207, 2),
			track("s2", "Digital Love", 298, 3),
			track("s5", "Harder, Better, Faster, Stronger", 224, 4),
		},
		Genre:       "Electronic",
		ReleaseDate: "2001-03-07",
		RecordLabel: "Parlophone",
		Copyright:   "℗ 2001 Daft Life Ltd.",
		Notes:       albumNotes,
	}
}

func essentials() playback.PlaylistDetail {
	song := func(id, title, artist string, secs int) playback.Song {
		return playback.Song{ID: id, Title: title, Artist: artist, Duration: time.Duration(secs) * time.Second}
	}
	return playback.PlaylistDetail{
		Playlist: playback.CatalogPlaylist{ID: "pl1", Name: "Daft Punk Essentials", Curator: "Apple Music Electronic"},
		Tracks: []playback.Song{
			song("s1", "One More Time", "Daft Punk", 320),
			song("s3", "Get Lucky", "Daft Punk, Pharrell Williams & Nile Rodgers", 369),
			song("s6", "Instant Crush", "Daft Punk & Julian Casablancas", 337),
			song("s7", "Around the World", "Daft Punk", 429),
		},
		Notes: "The French duo's robot-helmeted house, from Homework to Random Access Memories.",
	}
}

// openSong searches "daft" and opens the song row at index row (0 is the
// first song), letting its album load.
func openSong(t *testing.T, f *playbacktest.Fake, row int) Model {
	t.Helper()
	f.SearchCatalogResult = catalog()
	f.SongAlbumResult = discovery()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "down", "down", "down", "down") // suggestions, artist, first song
	for range row {
		m, _ = press(t, m, "down")
	}
	m, cmd := press(t, m, "enter")
	return settle(t, m, cmd)
}

// openFromArtist opens the Daft Punk page and enters its first row of kind;
// albums answer with f.AlbumResult when set, else discovery().
func openFromArtist(t *testing.T, f *playbacktest.Fake, kind artistItemKind) Model {
	t.Helper()
	if f.AlbumResult.Album.ID == "" {
		f.AlbumResult = discovery()
	}
	f.CatalogPlaylistResult = essentials()
	m := openDaftPunk(t, f)
	for m.artistItems()[m.cursor()].kind != kind {
		m, _ = press(t, m, "down")
	}
	m, cmd := press(t, m, "enter")
	return settle(t, m, cmd)
}

func callsOf(f *playbacktest.Fake, method string) []playbacktest.Call {
	var out []playbacktest.Call
	for _, c := range f.Calls() {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

func TestSearchEnterOnSongOpensItsAlbumWithTheSongHighlighted(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.SongAlbumResult = discovery()
	r := &fakeRecents{}
	m := searchFor(t, loadedWithRecents(t, f, r), "daft")
	m, _ = press(t, m, "down", "down", "down", "down", "down") // the second song, Digital Love
	m, cmd := press(t, m, "enter")

	if m.top().kind != viewAlbum || len(m.stack) != 3 || m.stack[1].kind != viewSearch {
		t.Fatalf("stack = %v; want stations, search, album", m.stack)
	}
	if view := plain(m); !strings.Contains(view, "DECRYPTING") || !strings.Contains(view, "DIGITAL LOVE") {
		t.Fatalf("loading song page lacks the notice or the song:\n%s", view)
	}
	m = settle(t, m, cmd)
	if calls := callsOf(f, "SongAlbum"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{"s2"}) {
		t.Fatalf("SongAlbum calls = %v; want one for s2", calls)
	}
	if len(callsOf(f, "PlaySongs")) != 0 {
		t.Fatal("opening a song played it")
	}
	if got := r.Added(); !reflect.DeepEqual(got, []string{"daft"}) {
		t.Fatalf("recents added = %q; want [daft]", got)
	}
	if m.cursor() != 2 {
		t.Fatalf("cursor = %d; want the highlighted third track", m.cursor())
	}
	// The highlight stays on the song when the cursor moves away.
	m, _ = press(t, m, "up")
	if view := plain(m); !strings.Contains(view, "▶ 3  DIGITAL LOVE") {
		t.Fatalf("song not highlighted:\n%s", view)
	}
}

func TestSongHighlightFallsBackToTheTitle(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.SearchCatalogResult.Songs[1].ID = "other-storefront-id"
	m := openSong(t, f, 1)
	if m.cursor() != 2 {
		t.Fatalf("cursor = %d; want the track titled like the song", m.cursor())
	}
}

func TestAlbumEnterPlaysTheAlbumFromTheSelectedTrack(t *testing.T) {
	f := playbacktest.New()
	m := openSong(t, f, 1)
	m, cmd := press(t, m, "down", "enter")
	settle(t, m, cmd)
	assertCall(t, f, "PlaySongs", []string{"s1", "s4", "s2", "s5"}, 3)
}

func TestAlbumPageShowsHeaderTracksAndFacts(t *testing.T) {
	f := playbacktest.New()
	m := openFromArtist(t, f, itemAlbum)
	if calls := callsOf(f, "Album"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{"al1"}) {
		t.Fatalf("Album calls = %v; want one for al1", calls)
	}
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 160, Height: 60})
	view := plain(m)
	order := []string{
		"ALBUM", "DISCOVERY", "DAFT PUNK", "ELECTRONIC · 2001",
		" 1  ONE MORE TIME", "5:20", " 4  HARDER, BETTER, FASTER, STRONGER", "3:44",
		"7 MAR 2001", "4 SONGS, 17 MINUTES", "℗ 2001 DAFT LIFE LTD.", "PARLOPHONE",
		"Discovery is the second", "▸ MORE",
	}
	last := -1
	for _, want := range order {
		i := strings.Index(view[last+1:], want)
		if i < 0 {
			t.Fatalf("view missing %q after position %d:\n%s", want, last, view)
		}
		last += 1 + i
	}
	if strings.Contains(view, "▶") {
		t.Errorf("an album opened from the artist page highlights a track:\n%s", view)
	}
}

func TestAlbumLeavesOutUnknownFacts(t *testing.T) {
	f := playbacktest.New()
	f.AlbumResult = playback.AlbumDetail{
		Album:  playback.Album{ID: "al1", Title: "Discovery"},
		Tracks: []playback.Track{{Song: playback.Song{ID: "s1", Title: "One More Time"}}},
	}
	m := openDaftPunk(t, f)
	for m.artistItems()[m.cursor()].kind != itemAlbum {
		m, _ = press(t, m, "down")
	}
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	view := plain(m)
	// A track without a number is numbered by position; one without a
	// duration shows none.
	if !strings.Contains(view, " 1  ONE MORE TIME") || !strings.Contains(view, "1 SONG") {
		t.Errorf("sparse album lacks its track or count:\n%s", view)
	}
	for _, absent := range []string{"MINUTE", "℗", "▸ MORE", " · 0"} {
		if strings.Contains(view, absent) {
			t.Errorf("sparse album shows %q:\n%s", absent, view)
		}
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "ONE MORE TIME") && strings.Contains(l[:strings.Index(l, "│ │")], ":") {
			t.Errorf("a track without a duration shows one: %q", l)
		}
	}
}

func TestAlbumWithSeveralDiscsHeadsEachDisc(t *testing.T) {
	d := discovery()
	d.Tracks[2].Disc, d.Tracks[2].Number = 2, 1
	d.Tracks[3].Disc, d.Tracks[3].Number = 2, 2
	f := playbacktest.New()
	f.AlbumResult = d
	m := openFromArtist(t, f, itemAlbum)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 160, Height: 60})
	view := plain(m)
	one, two := strings.Index(view, "D I S C   1"), strings.Index(view, "D I S C   2")
	if one < 0 || two < one || !strings.Contains(view[two:], " 1  DIGITAL LOVE") {
		t.Fatalf("discs not headed:\n%s", view)
	}
	// Disc headers are not selectable: the third item is still a track.
	m, _ = press(t, m, "down", "down")
	m, cmd := press(t, m, "enter")
	settle(t, m, cmd)
	assertCall(t, f, "PlaySongs", []string{"s1", "s4", "s2", "s5"}, 2)
}

func TestArtistPlaylistOpensThePlaylistView(t *testing.T) {
	f := playbacktest.New()
	m := openFromArtist(t, f, itemPlaylist)
	if m.top().kind != viewPlaylist {
		t.Fatalf("top view = %v; want the playlist view", m.top().kind)
	}
	if calls := callsOf(f, "CatalogPlaylist"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{"pl1"}) {
		t.Fatalf("CatalogPlaylist calls = %v; want one for pl1", calls)
	}
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 160, Height: 60})
	view := plain(m)
	for _, want := range []string{"PLAYLIST", "DAFT PUNK ESSENTIALS", "APPLE MUSIC ELECTRONIC", " 2  GET LUCKY · DAFT PUNK, PHARRELL", "6:09", "4 SONGS, 24 MINUTES", "robot-helmeted"} {
		if !strings.Contains(view, want) {
			t.Errorf("playlist view missing %q:\n%s", want, view)
		}
	}
	m, cmd := press(t, m, "down", "enter")
	settle(t, m, cmd)
	assertCall(t, f, "PlaySongs", []string{"s1", "s3", "s6", "s7"}, 1)
}

func TestTrackNotesMoreTogglesFullNotes(t *testing.T) {
	f := playbacktest.New()
	m := openFromArtist(t, f, itemAlbum)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 60})
	if view := plain(m); strings.Contains(view, "refusing to stop") || !strings.Contains(view, "▸ MORE") {
		t.Fatalf("collapsed notes show everything or lack MORE:\n%s", view)
	}
	for range 10 {
		m, _ = press(t, m, "down")
	}
	m, cmd := press(t, m, "enter")
	if cmd != nil {
		t.Fatal("MORE called the player")
	}
	if view := plain(m); !strings.Contains(view, "refusing to stop.") || !strings.Contains(view, "▴ LESS") {
		t.Fatalf("expanded notes lack the end or LESS:\n%s", view)
	}
}

func TestTrackPageFailureOffersRetry(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"SongAlbum": errors.New("catalog offline")}
	m := openSong(t, f, 0)
	if view := plain(m); !strings.Contains(view, "[R] RETRY") || !strings.Contains(view, "CATALOG OFFLINE") {
		t.Fatalf("failed page lacks the retry notice or the reason:\n%s", view)
	}
	f.MethodErr = nil
	m, cmd := press(t, m, "r")
	if !strings.Contains(plain(m), "DECRYPTING") {
		t.Fatalf("retry does not show loading:\n%s", plain(m))
	}
	m = settle(t, m, cmd)
	if len(callsOf(f, "SongAlbum")) != 2 || !strings.Contains(plain(m), "AERODYNAMIC") {
		t.Fatalf("retried page not shown:\n%s", plain(m))
	}
	// r on a loaded page is not a retry.
	before := len(f.Calls())
	if _, cmd := press(t, m, "r"); cmd != nil || len(f.Calls()) != before {
		t.Fatal("r reloaded a loaded page")
	}
}

func TestTrackPageEmptyShowsNoData(t *testing.T) {
	f := playbacktest.New()
	m := openFromArtist(t, f, itemPlaylist)
	m.stack[len(m.stack)-1].tracks.playlistDetail = playback.PlaylistDetail{}
	if view := plain(m); !strings.Contains(view, "NO DATA ON FILE") {
		t.Fatalf("empty page lacks its notice:\n%s", view)
	}
	before := len(f.Calls())
	if _, cmd := press(t, m, "down", "enter"); cmd != nil || len(f.Calls()) != before {
		t.Fatal("enter on an empty page did something")
	}
}

func TestEscFromTrackPagesRestoresThePreviousView(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		f := playbacktest.New()
		m := openSong(t, f, 1)
		m, _ = press(t, m, "esc")
		if m.top().kind != viewSearch || m.input.Value() != "daft" || m.cursor() != 4 || !m.input.Focused() {
			t.Fatalf("search came back as %v, input %q, cursor %d, focused %v", m.top().kind, m.input.Value(), m.cursor(), m.input.Focused())
		}
	})
	t.Run("artist", func(t *testing.T) {
		f := playbacktest.New()
		m := openFromArtist(t, f, itemPlaylist)
		m, _ = press(t, m, "esc")
		if m.top().kind != viewArtist || m.artistItems()[m.cursor()].kind != itemPlaylist {
			t.Fatalf("artist came back as %v with cursor %d", m.top().kind, m.cursor())
		}
		if len(artistCalls(f)) != 1 {
			t.Fatal("going back reloaded the artist")
		}
	})
}

// blockingDetailPlayer hands every detail context to the test and blocks
// until that context is done.
type blockingDetailPlayer struct {
	*playbacktest.Fake
	ctxs chan context.Context
}

func (p *blockingDetailPlayer) SongAlbum(ctx context.Context, _ string) (playback.AlbumDetail, error) {
	p.ctxs <- ctx
	<-ctx.Done()
	return playback.AlbumDetail{}, ctx.Err()
}

func TestLeavingALoadingTrackPageCancelsIt(t *testing.T) {
	for _, k := range []string{"esc", "tab", "/"} {
		t.Run(k, func(t *testing.T) {
			f := playbacktest.New()
			f.PlaylistsResult = stations()
			f.SearchCatalogResult = catalog()
			p := &blockingDetailPlayer{Fake: f, ctxs: make(chan context.Context, 1)}
			m := New(p, Options{Now: newClock().now, Seed: 2077})
			m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
			m = searchFor(t, m, "daft")
			m, _ = press(t, m, "down", "down", "down", "down")
			m, cmd := press(t, m, "enter")
			done := make(chan tea.Msg, 4)
			launch(cmd, done)
			var ctx context.Context
			select {
			case ctx = <-p.ctxs:
			case <-time.After(2 * time.Second):
				t.Fatal("the album load never reached the player")
			}
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) < defaultCallTimeout {
				t.Errorf("album deadline in %v; want the longer detail timeout", time.Until(deadline))
			}
			m, _ = press(t, m, k)
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("leaving the page did not cancel its load")
			}
			for answered := false; !answered; {
				select {
				case msg := <-done:
					_, answered = msg.(albumMsg)
					m, _ = step(t, m, msg)
				case <-time.After(2 * time.Second):
					t.Fatal("the cancelled load never answered")
				}
			}
			if strings.Contains(m.status, "FEED FAILED") {
				t.Fatalf("a cancelled load was reported: %q", m.status)
			}
		})
	}
}

func TestTrackAnswerForAnotherPageIsDropped(t *testing.T) {
	f := playbacktest.New()
	m := openSong(t, f, 0)
	seq := m.top().tracks.seq
	m, _ = step(t, m, albumMsg{seq: seq - 1, detail: playback.AlbumDetail{Album: playback.Album{Title: "Stale Album"}}})
	m, _ = step(t, m, playlistMsg{seq: seq, detail: playback.PlaylistDetail{Playlist: playback.CatalogPlaylist{Name: "Wrong Kind"}}})
	if view := plain(m); strings.Contains(view, "STALE ALBUM") || strings.Contains(view, "WRONG KIND") {
		t.Fatalf("an answer for another page was shown:\n%s", view)
	}
}

func TestTrackViewsFitEverySize(t *testing.T) {
	sizes := []struct{ w, h int }{{1, 1}, {10, 3}, {30, 8}, {40, 10}, {59, 15}, {60, 16}, {80, 24}, {160, 50}}
	for _, kind := range []artistItemKind{itemAlbum, itemPlaylist} {
		for _, sz := range sizes {
			t.Run(fmt.Sprintf("%d/%dx%d", kind, sz.w, sz.h), func(t *testing.T) {
				f := playbacktest.New()
				f.AlbumResult = discovery()
				f.AlbumResult.Tracks[0].Title = "宇多田ヒカル First Love (Remastered 2014 Extended Edition)"
				f.AlbumResult.Tracks[1].Duration = 2*time.Hour + 3*time.Second
				m := openFromArtist(t, f, kind)
				m, _ = press(t, m, "down", "down")
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
}

func TestTrackBodyLinesFillWidth(t *testing.T) {
	f := playbacktest.New()
	f.AlbumResult = discovery()
	f.AlbumResult.Tracks[0].Title = "宇多田ヒカル First Love"
	m := openFromArtist(t, f, itemAlbum)
	m.stack[len(m.stack)-1].tracks.song = playback.Song{ID: "s4"}
	for _, w := range []int{9, 30, 46} {
		for cur := range len(m.trackItems()) {
			m.stack[len(m.stack)-1].cursor = cur
			for i, row := range m.trackBody(w, 14) {
				if got := ansi.StringWidth(row); got != w {
					t.Errorf("w=%d cursor=%d: row %d is %d cells: %q", w, cur, i, got, ansi.Strip(row))
				}
			}
		}
	}
}

func TestTrackFormatting(t *testing.T) {
	times := map[time.Duration]string{0: "", 7 * time.Second: "0:07", 320 * time.Second: "5:20", time.Hour + 2*time.Minute + 3*time.Second: "1:02:03"}
	for d, want := range times {
		if got := trackTime(d); got != want {
			t.Errorf("trackTime(%v) = %q; want %q", d, got, want)
		}
	}
	dates := map[string]string{"1975-06-27": "27 JUN 1975", "2001-03-07": "7 MAR 2001", "1999": "1999", "": ""}
	for in, want := range dates {
		if got := releaseLabel(in); got != want {
			t.Errorf("releaseLabel(%q) = %q; want %q", in, got, want)
		}
	}
	summaries := []struct {
		n     int
		total time.Duration
		want  string
	}{
		{1, 0, "1 SONG"},
		{1, 40 * time.Second, "1 SONG, 1 MINUTE"},
		{4, 1049 * time.Second, "4 SONGS, 17 MINUTES"},
		{14, 61 * time.Minute, "14 SONGS, 1 HOUR 1 MINUTE"},
		{20, 2 * time.Hour, "20 SONGS, 2 HOURS"},
	}
	for _, tt := range summaries {
		if got := tracksSummary(tt.n, tt.total); got != tt.want {
			t.Errorf("tracksSummary(%d, %v) = %q; want %q", tt.n, tt.total, got, tt.want)
		}
	}
}

// TestCatalogBudgetMatchesTheHelper pins the Go deadlines to the helper's
// budget (helper/Sources/Nu11SignalProtocol/Catalog.swift): each catalog
// command must answer, or time out on its own, before the Go call gives up.
func TestCatalogBudgetMatchesTheHelper(t *testing.T) {
	src, err := os.ReadFile("../../helper/Sources/Nu11SignalProtocol/Catalog.swift")
	if err != nil {
		t.Fatal(err)
	}
	seconds := func(name string) time.Duration {
		t.Helper()
		m := regexp.MustCompile(`static let ` + name + `: TimeInterval = (\d+)`).FindSubmatch(src)
		if m == nil {
			t.Fatalf("Catalog.swift has no %s", name)
		}
		n, _ := strconv.Atoi(string(m[1]))
		return time.Duration(n) * time.Second
	}
	lookup, section := seconds("lookup"), seconds("section")
	if got := seconds("goArtistCallTimeout"); got != artistCallTimeout {
		t.Errorf("helper mirrors the artist timeout as %v; Go uses %v", got, artistCallTimeout)
	}
	if got := seconds("goDetailCallTimeout"); got != detailCallTimeout {
		t.Errorf("helper mirrors the detail timeout as %v; Go uses %v", got, detailCallTimeout)
	}
	if lookup+section >= artistCallTimeout {
		t.Errorf("artist budget %v does not fit in %v", lookup+section, artistCallTimeout)
	}
	if 2*lookup >= detailCallTimeout {
		t.Errorf("song album budget %v does not fit in %v", 2*lookup, detailCallTimeout)
	}
}

func TestTrackViewGolden(t *testing.T) {
	tests := []struct {
		name string
		open func(t *testing.T, f *playbacktest.Fake) Model
	}{
		{"song_80x24", func(t *testing.T, f *playbacktest.Fake) Model { return openSong(t, f, 1) }},
		{"playlist_80x24", func(t *testing.T, f *playbacktest.Fake) Model {
			m := openFromArtist(t, f, itemPlaylist)
			m, _ = press(t, m, "down")
			return m
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			m := tt.open(t, f)
			m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
			for i := 0; i < 12; i++ {
				m, _ = step(t, m, tickMsg{gen: m.tickGen})
			}
			m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
			assertGolden(t, tt.name+".golden", ansi.Strip(m.View().Content))
		})
	}
}
