package demo

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Compile-time check that the demo satisfies the port.
var _ playback.Player = (*Player)(nil)

func newPlayer(t *testing.T) *Player {
	t.Helper()
	p := New(Options{Tick: 5 * time.Millisecond})
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// waitState reads states until one satisfies ok, failing after a timeout.
func waitState(t *testing.T, p *Player, ok func(playback.State) bool) playback.State {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case s, open := <-p.States():
			if !open {
				t.Fatal("states closed while waiting")
			}
			if ok(s) {
				return s
			}
		case <-deadline:
			t.Fatal("timed out waiting for state")
		}
	}
}

func TestAuthorizeAndStations(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	if s, err := p.Authorize(ctx); err != nil || s != playback.AuthAuthorized {
		t.Fatalf("Authorize = %v, %v", s, err)
	}
	pls, err := p.Playlists(ctx)
	if err != nil || len(pls) < 3 {
		t.Fatalf("Playlists = %d, %v; want at least 3", len(pls), err)
	}
}

func TestPlaylistPlaysAndProgresses(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	pls, _ := p.Playlists(ctx)
	if err := p.PlayPlaylist(ctx, pls[0].ID); err != nil {
		t.Fatal(err)
	}
	first := waitState(t, p, func(s playback.State) bool { return s.Status == playback.StatusPlaying })
	if first.Title == "" || first.Duration <= 0 {
		t.Fatalf("playing state lacks a track: %+v", first)
	}
	waitState(t, p, func(s playback.State) bool { return s.Position > 0 })

	if err := p.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, func(s playback.State) bool { return s.Status == playback.StatusPaused })

	if err := p.Next(ctx); err != nil {
		t.Fatal(err)
	}
	next := waitState(t, p, func(s playback.State) bool { return s.SongID != first.SongID })
	if next.Position != 0 {
		t.Errorf("next track starts at %v, want 0", next.Position)
	}

	if err := p.Seek(ctx, time.Hour); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, func(s playback.State) bool { return s.SongID == next.SongID && s.Position == s.Duration })
}

func TestSearchAndPlaySongs(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	res, err := p.SearchCatalog(ctx, "NEON", 25)
	if err != nil || len(res.Songs) == 0 {
		t.Fatalf("SearchCatalog = %d songs, %v; want matches", len(res.Songs), err)
	}
	if limited, _ := p.SearchCatalog(ctx, "e", 2); len(limited.Songs) != 2 {
		t.Errorf("SearchCatalog limit 2 returned %d songs", len(limited.Songs))
	}
	songs := res.Songs
	ids := []string{songs[0].ID}
	if err := p.PlaySongs(ctx, ids, 0); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, func(s playback.State) bool { return s.SongID == songs[0].ID && s.Status == playback.StatusPlaying })
}

// TestSearchCatalogFollowsHelperContract pins the edges the helper enforces:
// a blank term is an error and limit is clamped to 1...25.
func TestSearchCatalogFollowsHelperContract(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()

	for _, term := range []string{"", "   "} {
		if _, err := p.SearchCatalog(ctx, term, 5); !errors.Is(err, ErrEmptyTerm) {
			t.Errorf("SearchCatalog(%q) error = %v; want ErrEmptyTerm", term, err)
		}
	}
	for _, limit := range []int{0, -3} {
		res, err := p.SearchCatalog(ctx, "e", limit)
		if err != nil || len(res.Songs) != 1 {
			t.Errorf("SearchCatalog limit %d = %d songs, %v; want clamped to 1", limit, len(res.Songs), err)
		}
	}
	res, err := p.SearchCatalog(ctx, "e", 1000)
	if err != nil || len(res.Songs) == 0 || len(res.Songs) > 25 {
		t.Errorf("SearchCatalog limit 1000 = %d songs, %v; want 1...25", len(res.Songs), err)
	}
}

func TestSearchCatalogMatchesArtistsSongsAndSuggestions(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()

	res, err := p.SearchCatalog(ctx, "CHROME", 25)
	if err != nil {
		t.Fatal(err)
	}
	wantArtists := []playback.Artist{{ID: "demo-artist-chrome-saints", Name: "Chrome Saints", Genres: []string{"Synthwave"}}}
	if !reflect.DeepEqual(res.Artists, wantArtists) {
		t.Errorf("Artists = %+v; want %+v", res.Artists, wantArtists)
	}
	var ids []string
	for _, s := range res.Songs {
		ids = append(ids, s.ID)
	}
	// Chrome Saints' two songs plus "Ghost in the Chrome", in catalog order.
	if want := []string{"d01", "d03", "d08"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("song ids = %v; want %v", ids, want)
	}
	if want := []string{"chrome saints", "ghost in the chrome"}; !reflect.DeepEqual(res.Suggestions, want) {
		t.Errorf("Suggestions = %v; want %v", res.Suggestions, want)
	}

	limited, err := p.SearchCatalog(ctx, "chrome", 1)
	if err != nil || len(limited.Artists) != 1 || len(limited.Songs) != 1 || len(limited.Suggestions) != 1 {
		t.Errorf("limit 1 = %+v, %v; want one of each", limited, err)
	}

	none, err := p.SearchCatalog(ctx, "no such thing", 25)
	if err != nil || len(none.Artists)+len(none.Songs)+len(none.Suggestions) != 0 {
		t.Errorf("no match = %+v, %v; want empty", none, err)
	}
}

func TestInvalidRequestsFail(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	if err := p.PlayPlaylist(ctx, "nope"); err == nil {
		t.Error("unknown playlist accepted")
	}
	if err := p.PlaySongs(ctx, []string{"nope"}, 0); err == nil {
		t.Error("unknown song accepted")
	}
	if err := p.Resume(ctx); err == nil {
		t.Error("resume with an empty queue accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.Playlists(cancelled); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled context: err = %v", err)
	}
}

func TestCloseClosesChannelsAndRejectsCalls(t *testing.T) {
	p := New(Options{Tick: 5 * time.Millisecond})
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	for range p.States() {
	}
	for range p.Errors() {
	}
	if err := p.Next(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Next after Close = %v, want ErrClosed", err)
	}
}

func TestArtistReturnsDeterministicPages(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()

	for _, a := range artists {
		d, err := p.Artist(ctx, a.ID)
		if err != nil {
			t.Fatalf("Artist(%s): %v", a.ID, err)
		}
		if !reflect.DeepEqual(d.Artist, a) {
			t.Errorf("Artist(%s).Artist = %+v; want %+v", a.ID, d.Artist, a)
		}
		if len(d.TopSongs) == 0 || len(d.Albums) == 0 || d.About.Notes == "" || d.About.Genre != a.Genres[0] {
			t.Errorf("Artist(%s) lacks a top song, an album or about: %+v", a.ID, d)
		}
		for _, s := range d.TopSongs {
			if s.Artist != a.Name {
				t.Errorf("Artist(%s) top song %q is by %q", a.ID, s.Title, s.Artist)
			}
			// Top songs must be playable through PlaySongs.
			if _, ok := songByID(s.ID); !ok {
				t.Errorf("Artist(%s) top song %q is not in the catalog", a.ID, s.ID)
			}
		}
		again, _ := p.Artist(ctx, a.ID)
		if !reflect.DeepEqual(d, again) {
			t.Errorf("Artist(%s) is not deterministic", a.ID)
		}
	}

	d, _ := p.Artist(ctx, "demo-artist-chrome-saints")
	wantAlbums := []playback.Album{{ID: "demo-album-last-call-sessions", Title: "Last Call Sessions", Artist: "Chrome Saints", Year: 2076, TrackCount: 2}}
	if !reflect.DeepEqual(d.Albums, wantAlbums) {
		t.Errorf("Albums = %+v; want %+v", d.Albums, wantAlbums)
	}
	var ids []string
	for _, s := range d.TopSongs {
		ids = append(ids, s.ID)
	}
	if !reflect.DeepEqual(ids, []string{"d01", "d03"}) {
		t.Errorf("top song ids = %v; want catalog order", ids)
	}
	if d.About.Origin == "" || d.About.Formed == "" || len(d.Playlists) == 0 || len(d.Singles) == 0 {
		t.Errorf("Chrome Saints page lacks origin, formed, playlists or singles: %+v", d)
	}
}

func TestArtistRejectsUnknownIDAndClosedPlayer(t *testing.T) {
	p := newPlayer(t)
	if _, err := p.Artist(context.Background(), "nope"); err == nil {
		t.Error("unknown artist accepted")
	}
	_ = p.Close()
	if _, err := p.Artist(context.Background(), artists[0].ID); !errors.Is(err, ErrClosed) {
		t.Errorf("Artist after Close = %v; want ErrClosed", err)
	}
}
