package playbacktest

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

var _ playback.Player = (*Fake)(nil)

func TestFakeRecordsCallsAndReturnsCannedResults(t *testing.T) {
	f := New()
	f.PlaylistsResult = []playback.Playlist{{ID: "p1"}}
	f.SearchCatalogResult = playback.SearchResults{Artists: []playback.Artist{{ID: "a1"}}}
	ctx := t.Context()

	if st, err := f.Authorize(ctx); err != nil || st != playback.AuthAuthorized {
		t.Fatalf("Authorize = %q, %v", st, err)
	}
	if res, _ := f.SearchCatalog(ctx, "daft", 3); len(res.Artists) != 1 {
		t.Fatalf("SearchCatalog = %+v", res)
	}
	if lists, _ := f.Playlists(ctx); len(lists) != 1 {
		t.Fatalf("Playlists = %v", lists)
	}
	_ = f.PlaySongs(ctx, []string{"s1"}, 0)
	_ = f.Seek(ctx, 3*time.Second)

	want := []Call{
		{Method: "Authorize"},
		{Method: "SearchCatalog", Args: []any{"daft", 3}},
		{Method: "Playlists"},
		{Method: "PlaySongs", Args: []any{[]string{"s1"}, 0}},
		{Method: "Seek", Args: []any{3 * time.Second}},
	}
	if got := f.Calls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Calls = %#v; want %#v", got, want)
	}
}

func TestFakeCallsAreSnapshots(t *testing.T) {
	f := New()
	ids := []string{"s1", "s2"}
	_ = f.PlaySongs(t.Context(), ids, 1)
	ids[0] = "mutated"
	calls := f.Calls()
	calls[0].Method = "mutated"

	want := []Call{{Method: "PlaySongs", Args: []any{[]string{"s1", "s2"}, 1}}}
	if got := f.Calls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Calls = %#v; want %#v", got, want)
	}
}

func TestFakeBuffersUpToChannelBuffer(t *testing.T) {
	f := New()
	for range ChannelBuffer {
		f.PushState(playback.State{}) // must not block
		f.PushError(errors.New("e"))
	}
	if len(f.States()) != ChannelBuffer || len(f.Errors()) != ChannelBuffer {
		t.Fatalf("buffered %d states, %d errors; want %d each", len(f.States()), len(f.Errors()), ChannelBuffer)
	}
}

func TestFakeErrPushAndClose(t *testing.T) {
	f := New()
	boom := errors.New("boom")
	f.Err = boom
	if err := f.Pause(t.Context()); !errors.Is(err, boom) {
		t.Fatalf("Pause = %v; want boom", err)
	}

	f.PushState(playback.State{Status: playback.StatusPlaying})
	if s := <-f.States(); s.Status != playback.StatusPlaying {
		t.Fatalf("state = %+v", s)
	}
	f.PushError(boom)
	if err := <-f.Errors(); !errors.Is(err, boom) {
		t.Fatalf("error = %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, open := <-f.States(); open {
		t.Fatal("States not closed")
	}
	if !f.Closed() {
		t.Fatal("Closed() = false")
	}
}

func TestFakeSearchCatalogMethodErr(t *testing.T) {
	f := New()
	boom := errors.New("boom")
	f.SearchCatalogResult = playback.SearchResults{Songs: []playback.Song{{ID: "s1"}}}
	f.MethodErr = map[string]error{"SearchCatalog": boom}
	res, err := f.SearchCatalog(t.Context(), "x", 1)
	if !errors.Is(err, boom) {
		t.Fatalf("SearchCatalog err = %v; want boom", err)
	}
	if !reflect.DeepEqual(res, playback.SearchResults{}) {
		t.Fatalf("SearchCatalog result on error = %+v; want zero", res)
	}
}

func TestFakeArtistReturnsCannedDetailAndRecordsID(t *testing.T) {
	f := New()
	f.ArtistResult = playback.ArtistDetail{
		Artist:   playback.Artist{ID: "a1", Name: "Daft Punk"},
		TopSongs: []playback.Song{{ID: "s1"}},
		About:    playback.ArtistAbout{Genre: "Electronic"},
	}
	got, err := f.Artist(t.Context(), "a1")
	if err != nil || !reflect.DeepEqual(got, f.ArtistResult) {
		t.Fatalf("Artist = %+v, %v; want the canned detail", got, err)
	}
	if want := []Call{{Method: "Artist", Args: []any{"a1"}}}; !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("Calls = %#v; want %#v", f.Calls(), want)
	}

	boom := errors.New("boom")
	f.MethodErr = map[string]error{"Artist": boom}
	got, err = f.Artist(t.Context(), "a1")
	if !errors.Is(err, boom) || !reflect.DeepEqual(got, playback.ArtistDetail{}) {
		t.Fatalf("Artist on error = %+v, %v; want zero, boom", got, err)
	}
}

func TestFakeAlbumPlaylistAndSongAlbumReturnCannedDetails(t *testing.T) {
	f := New()
	f.AlbumResult = playback.AlbumDetail{Album: playback.Album{ID: "al1"}, Genre: "Electronic"}
	f.SongAlbumResult = playback.AlbumDetail{Album: playback.Album{ID: "al2"}}
	f.CatalogPlaylistResult = playback.PlaylistDetail{Playlist: playback.CatalogPlaylist{ID: "pl1"}, Tracks: []playback.Song{{ID: "s1"}}}
	ctx := t.Context()

	if got, err := f.Album(ctx, "al1"); err != nil || !reflect.DeepEqual(got, f.AlbumResult) {
		t.Fatalf("Album = %+v, %v; want the canned album", got, err)
	}
	if got, err := f.SongAlbum(ctx, "s9"); err != nil || !reflect.DeepEqual(got, f.SongAlbumResult) {
		t.Fatalf("SongAlbum = %+v, %v; want the canned song album", got, err)
	}
	if got, err := f.CatalogPlaylist(ctx, "pl1"); err != nil || !reflect.DeepEqual(got, f.CatalogPlaylistResult) {
		t.Fatalf("CatalogPlaylist = %+v, %v; want the canned playlist", got, err)
	}
	want := []Call{
		{Method: "Album", Args: []any{"al1"}},
		{Method: "SongAlbum", Args: []any{"s9"}},
		{Method: "CatalogPlaylist", Args: []any{"pl1"}},
	}
	if !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("Calls = %#v; want %#v", f.Calls(), want)
	}

	boom := errors.New("boom")
	f.MethodErr = map[string]error{"Album": boom, "SongAlbum": boom, "CatalogPlaylist": boom}
	if got, err := f.Album(ctx, "al1"); !errors.Is(err, boom) || !reflect.DeepEqual(got, playback.AlbumDetail{}) {
		t.Errorf("Album on error = %+v, %v; want zero, boom", got, err)
	}
	if got, err := f.SongAlbum(ctx, "s9"); !errors.Is(err, boom) || !reflect.DeepEqual(got, playback.AlbumDetail{}) {
		t.Errorf("SongAlbum on error = %+v, %v; want zero, boom", got, err)
	}
	if got, err := f.CatalogPlaylist(ctx, "pl1"); !errors.Is(err, boom) || !reflect.DeepEqual(got, playback.PlaylistDetail{}) {
		t.Errorf("CatalogPlaylist on error = %+v, %v; want zero, boom", got, err)
	}
}

func TestFakeVolumeIsSettableAndRecorded(t *testing.T) {
	f := New()
	f.VolumeResult = 0.4
	ctx := t.Context()

	if v, err := f.Volume(ctx); err != nil || v != 0.4 {
		t.Fatalf("Volume = %v, %v; want the canned 0.4", v, err)
	}
	// SetVolume records the level as asked and stores it clamped, so a
	// later Volume reads it back as a real player would.
	for _, tt := range []struct{ set, want float64 }{{0.7, 0.7}, {1.5, 1}, {-2, 0}} {
		if err := f.SetVolume(ctx, tt.set); err != nil {
			t.Fatalf("SetVolume(%v): %v", tt.set, err)
		}
		if v, _ := f.Volume(ctx); v != tt.want {
			t.Fatalf("Volume after SetVolume(%v) = %v; want %v", tt.set, v, tt.want)
		}
	}
	want := []Call{
		{Method: "Volume"},
		{Method: "SetVolume", Args: []any{0.7}}, {Method: "Volume"},
		{Method: "SetVolume", Args: []any{1.5}}, {Method: "Volume"},
		{Method: "SetVolume", Args: []any{-2.0}}, {Method: "Volume"},
	}
	if !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("Calls = %#v; want %#v", f.Calls(), want)
	}

	boom := errors.New("boom")
	f.MethodErr = map[string]error{"Volume": boom, "SetVolume": boom}
	if v, err := f.Volume(ctx); !errors.Is(err, boom) || v != 0 {
		t.Errorf("Volume on error = %v, %v; want 0, boom", v, err)
	}
	if err := f.SetVolume(ctx, 0.2); !errors.Is(err, boom) {
		t.Errorf("SetVolume on error = %v; want boom", err)
	}
	f.MethodErr = nil
	if v, _ := f.Volume(ctx); v != 0 {
		t.Errorf("a failed SetVolume changed the volume to %v", v)
	}
}

func TestFakeLibraryPlaylistAndPlayPlaylistFrom(t *testing.T) {
	f := New()
	f.LibraryPlaylistResult = playback.PlaylistDetail{
		Playlist: playback.CatalogPlaylist{ID: "p1", Name: "Night City"},
		Tracks:   []playback.Song{{ID: "i.s1"}},
	}
	ctx := t.Context()

	if got, err := f.LibraryPlaylist(ctx, "p1"); err != nil || !reflect.DeepEqual(got, f.LibraryPlaylistResult) {
		t.Fatalf("LibraryPlaylist = %+v, %v; want the canned playlist", got, err)
	}
	if err := f.PlayPlaylistFrom(ctx, "p1", 3); err != nil {
		t.Fatalf("PlayPlaylistFrom: %v", err)
	}
	want := []Call{
		{Method: "LibraryPlaylist", Args: []any{"p1"}},
		{Method: "PlayPlaylistFrom", Args: []any{"p1", 3}},
	}
	if !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("Calls = %#v; want %#v", f.Calls(), want)
	}

	boom := errors.New("boom")
	f.MethodErr = map[string]error{"LibraryPlaylist": boom, "PlayPlaylistFrom": boom}
	if got, err := f.LibraryPlaylist(ctx, "p1"); !errors.Is(err, boom) || !reflect.DeepEqual(got, playback.PlaylistDetail{}) {
		t.Errorf("LibraryPlaylist on error = %+v, %v; want zero, boom", got, err)
	}
	if err := f.PlayPlaylistFrom(ctx, "p1", 0); !errors.Is(err, boom) {
		t.Errorf("PlayPlaylistFrom on error = %v; want boom", err)
	}
}

func TestFakeLibraryEdits(t *testing.T) {
	f := New()
	f.CreatePlaylistResult = playback.Playlist{ID: "p.new", Name: "Late Shift"}
	ctx := t.Context()

	ids := []string{"s1", "s2"}
	if got, err := f.CreatePlaylist(ctx, "Late Shift", "notes", ids); err != nil || got != f.CreatePlaylistResult {
		t.Fatalf("CreatePlaylist = %+v, %v; want the canned playlist", got, err)
	}
	ids[0] = "mutated"
	if err := f.AddToPlaylist(ctx, "p.new", []string{"s3"}); err != nil {
		t.Fatalf("AddToPlaylist: %v", err)
	}
	if on, err := f.Favorite(ctx, "s1"); err != nil || on {
		t.Fatalf("Favorite before any set = %v, %v; want false", on, err)
	}
	// SetFavorite stores its value, so a later Favorite reads it back.
	if err := f.SetFavorite(ctx, "s1", true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
	if on, _ := f.Favorite(ctx, "s1"); !on {
		t.Fatal("Favorite after SetFavorite(true) = false")
	}
	want := []Call{
		{Method: "CreatePlaylist", Args: []any{"Late Shift", "notes", []string{"s1", "s2"}}},
		{Method: "AddToPlaylist", Args: []any{"p.new", []string{"s3"}}},
		{Method: "Favorite", Args: []any{"s1"}},
		{Method: "SetFavorite", Args: []any{"s1", true}},
		{Method: "Favorite", Args: []any{"s1"}},
	}
	if !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("Calls = %#v; want %#v", f.Calls(), want)
	}

	boom := errors.New("boom")
	f.MethodErr = map[string]error{"CreatePlaylist": boom, "AddToPlaylist": boom, "Favorite": boom, "SetFavorite": boom}
	if got, err := f.CreatePlaylist(ctx, "x", "", nil); !errors.Is(err, boom) || got != (playback.Playlist{}) {
		t.Errorf("CreatePlaylist on error = %+v, %v; want zero, boom", got, err)
	}
	if err := f.AddToPlaylist(ctx, "p.new", []string{"s1"}); !errors.Is(err, boom) {
		t.Errorf("AddToPlaylist on error = %v; want boom", err)
	}
	if on, err := f.Favorite(ctx, "s1"); !errors.Is(err, boom) || on {
		t.Errorf("Favorite on error = %v, %v; want false, boom", on, err)
	}
	if err := f.SetFavorite(ctx, "s1", false); !errors.Is(err, boom) {
		t.Errorf("SetFavorite on error = %v; want boom", err)
	}
	f.MethodErr = nil
	if on, _ := f.Favorite(ctx, "s1"); !on {
		t.Error("a failed SetFavorite changed the favorite")
	}
}
