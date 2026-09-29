package demo

import (
	"context"
	"errors"
	"math"
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

// TestPlaySongsStartsAtThePosition pins the helper contract that start is a
// position in ids, not a song: a song listed twice starts at the chosen copy.
func TestPlaySongsStartsAtThePosition(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	res, err := p.SearchCatalog(ctx, "e", 25)
	if err != nil || len(res.Songs) < 3 {
		t.Fatalf("SearchCatalog = %d songs, %v; want at least 3", len(res.Songs), err)
	}
	a, b, c := res.Songs[0].ID, res.Songs[1].ID, res.Songs[2].ID
	if err := p.PlaySongs(ctx, []string{a, b, a, c}, 2); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, func(s playback.State) bool { return s.SongID == a && s.Status == playback.StatusPlaying })
	if err := p.Next(ctx); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, func(s playback.State) bool { return s.SongID == c })
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

func TestSearchCatalogMatchesAlbumsPlaylistsAndTopResults(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()

	res, err := p.SearchCatalog(ctx, "chrome", 25)
	if err != nil {
		t.Fatal(err)
	}
	chromeSaints := playback.Artist{ID: "demo-artist-chrome-saints", Name: "Chrome Saints", Genres: []string{"Synthwave"}}
	lastCall := playback.Album{ID: "demo-album-last-call-sessions", Title: "Last Call Sessions", Artist: "Chrome Saints", Year: 2076, TrackCount: 2}
	essentials := playback.CatalogPlaylist{ID: "demo-playlist-chrome-saints-essentials", Name: "Chrome Saints Essentials", Curator: "Nu11Signal"}
	// Albums match by title or artist, so Spire Tower (holding "Ghost in
	// the Chrome") is not one.
	if want := []playback.Album{lastCall}; !reflect.DeepEqual(res.Albums, want) {
		t.Errorf("Albums = %+v; want %+v", res.Albums, want)
	}
	if want := []playback.CatalogPlaylist{essentials}; !reflect.DeepEqual(res.Playlists, want) {
		t.Errorf("Playlists = %+v; want %+v", res.Playlists, want)
	}
	wantTop := []playback.SearchItem{
		{Kind: playback.ItemArtist, Artist: chromeSaints},
		{Kind: playback.ItemAlbum, Album: lastCall},
		{Kind: playback.ItemSong, Song: res.Songs[0]},
		{Kind: playback.ItemPlaylist, Playlist: essentials},
	}
	if !reflect.DeepEqual(res.Top, wantTop) {
		t.Errorf("Top = %+v; want %+v", res.Top, wantTop)
	}

	// Every album and playlist result opens in the demo.
	for _, a := range res.Albums {
		if _, err := p.Album(ctx, a.ID); err != nil {
			t.Errorf("Album(%s): %v", a.ID, err)
		}
	}
	for _, pl := range res.Playlists {
		if _, err := p.CatalogPlaylist(ctx, pl.ID); err != nil {
			t.Errorf("CatalogPlaylist(%s): %v", pl.ID, err)
		}
	}

	limited, err := p.SearchCatalog(ctx, "chrome", 1)
	if err != nil || len(limited.Albums) != 1 || len(limited.Playlists) != 1 || len(limited.Top) != 1 {
		t.Errorf("limit 1 = %+v, %v; want one of each", limited, err)
	}
	songOnly, err := p.SearchCatalog(ctx, "lullaby", 25)
	if err != nil || len(songOnly.Albums)+len(songOnly.Playlists) != 0 ||
		!reflect.DeepEqual(songOnly.Top, []playback.SearchItem{{Kind: playback.ItemSong, Song: songOnly.Songs[0]}}) {
		t.Errorf("song-only search = %+v, %v; want only the song on top", songOnly, err)
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

func TestVolumeStartsAtDefaultAndClamps(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	if v, err := p.Volume(ctx); err != nil || v != DefaultVolume {
		t.Fatalf("Volume = %v, %v; want %v", v, err, DefaultVolume)
	}
	tests := []struct {
		name      string
		set, want float64
	}{
		{"within range", 0.3, 0.3},
		{"above one", 2, 1},
		{"below zero", -1, 0},
		{"not a number", math.NaN(), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := p.SetVolume(ctx, tt.set); err != nil {
				t.Fatalf("SetVolume(%v): %v", tt.set, err)
			}
			if v, err := p.Volume(ctx); err != nil || v != tt.want {
				t.Fatalf("Volume after SetVolume(%v) = %v, %v; want %v", tt.set, v, err, tt.want)
			}
		})
	}
}

func TestLibraryPlaylistListsStationTracks(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	pls, err := p.Playlists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range pls {
		d, err := p.LibraryPlaylist(ctx, pl.ID)
		if err != nil {
			t.Fatalf("LibraryPlaylist(%s): %v", pl.ID, err)
		}
		if d.Playlist != (playback.CatalogPlaylist{ID: pl.ID, Name: pl.Name}) || len(d.Tracks) == 0 {
			t.Fatalf("LibraryPlaylist(%s) = %+v; want the station with tracks", pl.ID, d)
		}
		for _, s := range d.Tracks {
			if s.ID == "" || s.Title == "" || s.Duration <= 0 {
				t.Fatalf("LibraryPlaylist(%s) track %+v lacks an id, title or duration", pl.ID, s)
			}
		}
		again, _ := p.LibraryPlaylist(ctx, pl.ID)
		if !reflect.DeepEqual(again, d) {
			t.Fatalf("LibraryPlaylist(%s) is not deterministic", pl.ID)
		}
	}
	first, _ := p.LibraryPlaylist(ctx, "demo-1")
	var ids []string
	for _, s := range first.Tracks {
		ids = append(ids, s.ID)
	}
	if want := []string{"d01", "d03", "d09"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("demo-1 tracks = %v; want %v", ids, want)
	}
	if _, err := p.LibraryPlaylist(ctx, "nope"); err == nil {
		t.Error("unknown library playlist accepted")
	}
}

func TestPlayPlaylistFromStartsAtTheTrack(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	d, err := p.LibraryPlaylist(ctx, "demo-2")
	if err != nil || len(d.Tracks) < 4 {
		t.Fatalf("LibraryPlaylist(demo-2) = %+v, %v; want at least 4 tracks", d, err)
	}
	if err := p.PlayPlaylistFrom(ctx, "demo-2", 2); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, func(s playback.State) bool {
		return s.Status == playback.StatusPlaying && s.SongID == d.Tracks[2].ID
	})
	if err := p.Next(ctx); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, func(s playback.State) bool { return s.SongID == d.Tracks[3].ID })

	for _, start := range []int{-1, len(d.Tracks)} {
		if err := p.PlayPlaylistFrom(ctx, "demo-2", start); err == nil {
			t.Errorf("PlayPlaylistFrom start %d accepted", start)
		}
	}
	if err := p.PlayPlaylistFrom(ctx, "nope", 0); err == nil {
		t.Error("unknown playlist accepted")
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
	if _, err := p.Volume(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Volume after Close = %v, want ErrClosed", err)
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

func TestAlbumReturnsItsTracksAndFacts(t *testing.T) {
	p := newPlayer(t)
	d, err := p.Album(context.Background(), "demo-album-last-call-sessions")
	if err != nil {
		t.Fatal(err)
	}
	wantAlbum := playback.Album{ID: "demo-album-last-call-sessions", Title: "Last Call Sessions", Artist: "Chrome Saints", Year: 2076, TrackCount: 2}
	if d.Album != wantAlbum {
		t.Errorf("Album = %+v; want %+v", d.Album, wantAlbum)
	}
	d01, _ := songByID("d01")
	d03, _ := songByID("d03")
	wantTracks := []playback.Track{{Song: d01, Number: 1, Disc: 1}, {Song: d03, Number: 2, Disc: 1}}
	if !reflect.DeepEqual(d.Tracks, wantTracks) {
		t.Errorf("Tracks = %+v; want %+v", d.Tracks, wantTracks)
	}
	if d.Genre != "Synthwave" || d.ReleaseDate != "2076-03-14" || d.RecordLabel == "" || d.Copyright != "℗ 2076 Chrome Saints" || d.Notes == "" {
		t.Errorf("album facts = %+v", d)
	}
}

// TestEveryArtistPageEntryOpens walks every album, single and playlist an
// artist page lists: each must open, and every track must be playable.
func TestEveryArtistPageEntryOpens(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	playable := func(where string, s playback.Song) {
		if _, ok := songByID(s.ID); !ok {
			t.Errorf("%s: track %q is not in the catalog", where, s.ID)
		}
	}
	for _, a := range artists {
		page, err := p.Artist(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, section := range [][]playback.Album{page.EssentialAlbums, page.Albums, page.Singles, page.Compilations} {
			for _, al := range section {
				d, err := p.Album(ctx, al.ID)
				if err != nil {
					t.Errorf("Album(%s): %v", al.ID, err)
					continue
				}
				if d.Album != al || len(d.Tracks) != al.TrackCount || len(d.Tracks) == 0 {
					t.Errorf("Album(%s) = %+v with %d tracks; want the listed album %+v", al.ID, d.Album, len(d.Tracks), al)
				}
				for _, tr := range d.Tracks {
					playable(al.ID, tr.Song)
				}
			}
		}
		for _, pl := range page.Playlists {
			d, err := p.CatalogPlaylist(ctx, pl.ID)
			if err != nil {
				t.Errorf("CatalogPlaylist(%s): %v", pl.ID, err)
				continue
			}
			if d.Playlist != pl || len(d.Tracks) == 0 || d.Notes == "" {
				t.Errorf("CatalogPlaylist(%s) = %+v; want the listed playlist with tracks and notes", pl.ID, d)
			}
			for _, s := range d.Tracks {
				playable(pl.ID, s)
			}
		}
	}
}

func TestSongAlbumIsTheAlbumHoldingTheSong(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	for _, s := range catalog {
		d, err := p.SongAlbum(ctx, s.ID)
		if err != nil {
			t.Fatalf("SongAlbum(%s): %v", s.ID, err)
		}
		found := false
		for _, tr := range d.Tracks {
			found = found || tr.ID == s.ID
		}
		if d.Album.Title != s.Album || !found {
			t.Errorf("SongAlbum(%s) = %q without the song; want %q", s.ID, d.Album.Title, s.Album)
		}
		byID, _ := p.Album(ctx, d.Album.ID)
		if !reflect.DeepEqual(d, byID) {
			t.Errorf("SongAlbum(%s) differs from Album(%s)", s.ID, d.Album.ID)
		}
	}
}

func TestDetailsRejectUnknownIDsAndClosedPlayer(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	if _, err := p.Album(ctx, "nope"); err == nil {
		t.Error("unknown album accepted")
	}
	if _, err := p.SongAlbum(ctx, "nope"); err == nil {
		t.Error("unknown song accepted")
	}
	if _, err := p.CatalogPlaylist(ctx, "nope"); err == nil {
		t.Error("unknown playlist accepted")
	}
	_ = p.Close()
	if _, err := p.Album(ctx, "demo-album-route-77"); !errors.Is(err, ErrClosed) {
		t.Errorf("Album after Close = %v; want ErrClosed", err)
	}
	if _, err := p.SongAlbum(ctx, "d04"); !errors.Is(err, ErrClosed) {
		t.Errorf("SongAlbum after Close = %v; want ErrClosed", err)
	}
	if _, err := p.CatalogPlaylist(ctx, "demo-playlist-chrome-saints-essentials"); !errors.Is(err, ErrClosed) {
		t.Errorf("CatalogPlaylist after Close = %v; want ErrClosed", err)
	}
}

// TestArtistPageToleratesSparseData builds the page of an artist the demo
// knows nothing about: no genres, songs or extras.
func TestArtistPageToleratesSparseData(t *testing.T) {
	a := playback.Artist{ID: "demo-artist-ghost", Name: "Ghost"}
	d := buildArtistPage(a, catalog, artistExtras{})
	want := playback.ArtistDetail{Artist: a}
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("sparse page = %+v; want %+v", d, want)
	}
}

func TestCreatedPlaylistJoinsTheLibrary(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	before, _ := p.Playlists(ctx)

	pl, err := p.CreatePlaylist(ctx, "Late Shift", "For the night crew", []string{"d02", "d01"})
	if err != nil || pl.ID == "" || pl.Name != "Late Shift" {
		t.Fatalf("CreatePlaylist = %+v, %v; want a named playlist with an id", pl, err)
	}
	after, _ := p.Playlists(ctx)
	if len(after) != len(before)+1 || after[len(after)-1] != pl {
		t.Fatalf("Playlists after create = %+v; want the old ones then %+v", after, pl)
	}
	d, err := p.LibraryPlaylist(ctx, pl.ID)
	if err != nil || d.Notes != "For the night crew" || !reflect.DeepEqual(songIDs(d.Tracks), []string{"d02", "d01"}) {
		t.Fatalf("LibraryPlaylist(new) = %+v, %v; want its description and songs in order", d, err)
	}

	if err := p.AddToPlaylist(ctx, pl.ID, []string{"d03"}); err != nil {
		t.Fatalf("AddToPlaylist: %v", err)
	}
	d, _ = p.LibraryPlaylist(ctx, pl.ID)
	if !reflect.DeepEqual(songIDs(d.Tracks), []string{"d02", "d01", "d03"}) {
		t.Fatalf("tracks after AddToPlaylist = %v; want d03 appended", songIDs(d.Tracks))
	}
	if err := p.PlayPlaylistFrom(ctx, pl.ID, 2); err != nil {
		t.Fatalf("PlayPlaylistFrom(new, 2): %v", err)
	}
	waitState(t, p, func(s playback.State) bool { return s.SongID == "d03" })

	empty, err := p.CreatePlaylist(ctx, "Empty", "", nil)
	if err != nil || empty.ID == pl.ID {
		t.Fatalf("CreatePlaylist(empty) = %+v, %v; want a second, distinct playlist", empty, err)
	}
}

func TestLibraryEditsAreLocalToOnePlayer(t *testing.T) {
	ctx := context.Background()
	first := newPlayer(t)
	pls, _ := first.Playlists(ctx)
	if err := first.AddToPlaylist(ctx, pls[0].ID, []string{"d12"}); err != nil {
		t.Fatal(err)
	}
	if _, err := first.CreatePlaylist(ctx, "Mine", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := first.SetFavorite(ctx, "d01", true); err != nil {
		t.Fatal(err)
	}

	second := newPlayer(t)
	fresh, _ := second.Playlists(ctx)
	if len(fresh) != len(pls) {
		t.Fatalf("a new player lists %d playlists; want the %d stations", len(fresh), len(pls))
	}
	d, _ := second.LibraryPlaylist(ctx, pls[0].ID)
	if ids := songIDs(d.Tracks); ids[len(ids)-1] == "d12" {
		t.Fatalf("a new player sees another player's added song: %v", ids)
	}
	if on, _ := second.Favorite(ctx, "d01"); on {
		t.Fatal("a new player sees another player's favorite")
	}
}

func TestLibraryEditErrors(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	pls, _ := p.Playlists(ctx)
	tests := []struct {
		name string
		call func() error
	}{
		{"blank name", func() error { _, err := p.CreatePlaylist(ctx, "  ", "", nil); return err }},
		{"create with unknown song", func() error { _, err := p.CreatePlaylist(ctx, "x", "", []string{"nope"}); return err }},
		{"add to unknown playlist", func() error { return p.AddToPlaylist(ctx, "nope", []string{"d01"}) }},
		{"add no songs", func() error { return p.AddToPlaylist(ctx, pls[0].ID, nil) }},
		{"add unknown song", func() error { return p.AddToPlaylist(ctx, pls[0].ID, []string{"d01", "nope"}) }},
		{"favorite unknown song", func() error { _, err := p.Favorite(ctx, "nope"); return err }},
		{"set favorite on unknown song", func() error { return p.SetFavorite(ctx, "nope", true) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("want an error")
			}
		})
	}
	// A failed add changes nothing, even for the songs before the bad one.
	d, _ := p.LibraryPlaylist(ctx, pls[0].ID)
	if want := len(stations[0].ids); pls[0].ID != stations[0].ID || len(d.Tracks) != want {
		t.Fatalf("a failed AddToPlaylist left %d tracks; want %d", len(d.Tracks), want)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.CreatePlaylist(cancelled, "x", "", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("CreatePlaylist(cancelled) = %v; want context.Canceled", err)
	}
}

func TestFavoriteToggles(t *testing.T) {
	p := newPlayer(t)
	ctx := context.Background()
	if on, err := p.Favorite(ctx, "d05"); err != nil || on {
		t.Fatalf("Favorite before = %v, %v; want false", on, err)
	}
	for _, want := range []bool{true, true, false} {
		if err := p.SetFavorite(ctx, "d05", want); err != nil {
			t.Fatalf("SetFavorite(%v): %v", want, err)
		}
		if on, err := p.Favorite(ctx, "d05"); err != nil || on != want {
			t.Fatalf("Favorite after SetFavorite(%v) = %v, %v", want, on, err)
		}
	}
}

func songIDs(songs []playback.Song) []string {
	ids := make([]string, len(songs))
	for i, s := range songs {
		ids[i] = s.ID
	}
	return ids
}
