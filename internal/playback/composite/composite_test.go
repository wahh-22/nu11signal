package composite

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

var ctx = context.Background()

const (
	loc1 = "local:aaaa"
	loc2 = "local:bbbb"
	locP = "local:pl:cccc"
)

func methods(f *playbacktest.Fake) []string {
	var out []string
	for _, c := range f.Calls() {
		out = append(out, c.Method)
	}
	return out
}

func called(f *playbacktest.Fake, method string) []playbacktest.Call {
	var out []playbacktest.Call
	for _, c := range f.Calls() {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

func newPair(t *testing.T) (*Player, *playbacktest.Fake, *playbacktest.Fake) {
	t.Helper()
	apple, local := playbacktest.New(), playbacktest.New()
	p := New(apple, local)
	t.Cleanup(func() { _ = p.Close() })
	return p, apple, local
}

// recv waits for one value of ch.
func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("nothing delivered")
	}
	var zero T
	return zero
}

// quiet checks ch delivers nothing for a moment.
func quiet[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("unexpected %+v", v)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPlaylistsMergeAppleThenLocalWithTheirSource(t *testing.T) {
	p, apple, local := newPair(t)
	apple.PlaylistsResult = []playback.Playlist{{ID: "p.1", Name: "Night Drive", Editable: true}}
	local.PlaylistsResult = []playback.Playlist{{ID: locP, Name: "Music"}}
	got, err := p.Playlists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []playback.Playlist{
		{ID: "p.1", Name: "Night Drive", Editable: true, Source: playback.SourceApple},
		{ID: locP, Name: "Music", Source: playback.SourceLocal},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Playlists = %+v\nwant %+v", got, want)
	}
}

func TestPlaylistsKeepLocalWhenAppleFails(t *testing.T) {
	p, apple, local := newPair(t)
	apple.MethodErr = map[string]error{"Playlists": errors.New("offline")}
	local.PlaylistsResult = []playback.Playlist{{ID: locP, Name: "Music"}}
	got, err := p.Playlists(ctx)
	if err != nil || len(got) != 1 || got[0].ID != locP {
		t.Fatalf("Playlists = %+v, %v; want the local one", got, err)
	}
	if err := recv(t, p.Errors()); err == nil {
		t.Error("the Apple failure was not reported")
	}
	local.PlaylistsResult = nil
	if _, err := p.Playlists(ctx); err == nil {
		t.Error("with nothing local, the Apple failure should fail Playlists")
	}
}

func TestPlaySongsRoutesByTheStartSongAndSkipsTheOtherSource(t *testing.T) {
	p, apple, local := newPair(t)
	rep, err := p.PlaySongs(ctx, []string{"s1", loc1, "s2", loc2}, 1)
	if err != nil {
		t.Fatal(err)
	}
	calls := called(local, "PlaySongs")
	if len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{[]string{loc1, loc2}, 0}) {
		t.Errorf("local PlaySongs = %+v, want [loc1 loc2] from 0", calls)
	}
	if !reflect.DeepEqual(rep.Skipped, []string{"s1", "s2"}) {
		t.Errorf("Skipped = %v, want the Apple songs", rep.Skipped)
	}
	if n := len(called(apple, "PlaySongs")); n != 0 {
		t.Errorf("Apple played %d times", n)
	}

	rep, err = p.PlaySongs(ctx, []string{"s1", loc1, "s2"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	calls = called(apple, "PlaySongs")
	if len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{[]string{"s1", "s2"}, 1}) {
		t.Errorf("Apple PlaySongs = %+v, want [s1 s2] from 1", calls)
	}
	if !reflect.DeepEqual(rep.Skipped, []string{loc1}) {
		t.Errorf("Skipped = %v", rep.Skipped)
	}
}

func TestTransportGoesToTheActiveBackendAndSwitchingStopsTheOther(t *testing.T) {
	p, apple, local := newPair(t)
	if err := p.PlayPlaylist(ctx, locP); err != nil {
		t.Fatal(err)
	}
	if n := len(called(apple, "Stop")); n != 1 {
		t.Errorf("switching to local stopped Apple %d times, want once", n)
	}
	_ = p.Pause(ctx)
	_ = p.Next(ctx)
	_ = p.Seek(ctx, time.Second)
	_ = p.SetVolume(ctx, 0.5)
	_ = p.SetRepeat(ctx, playback.RepeatAll)
	for _, m := range []string{"Pause", "Next", "Seek", "SetVolume", "SetRepeat"} {
		if len(called(local, m)) != 1 || len(called(apple, m)) != 0 {
			t.Errorf("%s: local %v, apple %v", m, methods(local), methods(apple))
		}
	}
	if err := p.PlayPlaylistFrom(ctx, "p.1", 2); err != nil {
		t.Fatal(err)
	}
	if n := len(called(local, "Stop")); n != 1 {
		t.Errorf("switching to Apple stopped local %d times, want once", n)
	}
	_ = p.Resume(ctx)
	if len(called(apple, "Resume")) != 1 {
		t.Errorf("Resume after the switch went to %v", methods(local))
	}
	// Playing on the same source again stops nothing.
	_, _ = p.PlaySongs(ctx, []string{"s1"}, 0)
	if n := len(called(local, "Stop")); n != 1 {
		t.Errorf("local stopped %d times", n)
	}
}

func TestStatesAndLevelsComeFromTheActiveBackend(t *testing.T) {
	p, apple, local := newPair(t)
	// Apple is active until something local plays.
	apple.PushState(playback.State{Title: "apple"})
	if s := recv(t, p.States()); s.Title != "apple" {
		t.Fatalf("state = %+v", s)
	}
	local.PushState(playback.State{Title: "local idle"})
	quiet(t, p.States())

	if _, err := p.PlaySongs(ctx, []string{loc1}, 0); err != nil {
		t.Fatal(err)
	}
	apple.PushState(playback.State{Title: "apple stopped"})
	local.PushState(playback.State{Title: "local playing"})
	if s := recv(t, p.States()); s.Title != "local playing" {
		t.Fatalf("state = %+v, want the local one", s)
	}
	apple.PushLevels(playback.Spectrum{Bands: []float64{0.1}})
	quiet(t, p.Levels())
	local.PushLevels(playback.Spectrum{Bands: []float64{0.9}})
	if l := recv(t, p.Levels()); len(l.Bands) != 1 || l.Bands[0] != 0.9 {
		t.Fatalf("levels = %+v, want the local reading", l)
	}
}

func TestErrorsOfBothBackendsAreMerged(t *testing.T) {
	p, apple, local := newPair(t)
	apple.PushError(errors.New("a"))
	local.PushError(errors.New("b"))
	got := []string{recv(t, p.Errors()).Error(), recv(t, p.Errors()).Error()}
	slices.Sort(got)
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("errors = %v", got)
	}
}

func TestSupportsPerId(t *testing.T) {
	p, _, _ := newPair(t)
	for _, c := range []playback.Capability{playback.CapCatalogSearch, playback.CapFavorites, playback.CapEditPlaylists} {
		if !p.Supports(c, "s1") || !p.Supports(c, "") {
			t.Errorf("%s: Apple ids and the player should support it", c)
		}
		if p.Supports(c, loc1) || p.Supports(c, locP) {
			t.Errorf("%s: local ids should not support it", c)
		}
	}
}

func TestLocalOnlyHasNoCatalog(t *testing.T) {
	local := playbacktest.New()
	local.PlaylistsResult = []playback.Playlist{{ID: locP, Name: "Music"}}
	p := New(nil, local)
	defer p.Close()
	if s, err := p.Authorize(ctx); err != nil || s != playback.AuthAuthorized {
		t.Fatalf("Authorize = %v, %v", s, err)
	}
	if p.Supports(playback.CapCatalogSearch, "") || p.Supports(playback.CapFavorites, "") || p.Supports(playback.CapEditPlaylists, "") {
		t.Error("a player without Apple Music should support no catalog capability")
	}
	checks := map[string]error{}
	_, checks["SearchCatalog"] = p.SearchCatalog(ctx, "x", 5)
	_, checks["Artist"] = p.Artist(ctx, "a")
	_, checks["Album"] = p.Album(ctx, "a")
	_, checks["SongAlbum"] = p.SongAlbum(ctx, "s1")
	_, checks["CatalogPlaylist"] = p.CatalogPlaylist(ctx, "p")
	_, checks["CreatePlaylist"] = p.CreatePlaylist(ctx, "n", "", nil)
	checks["AddToPlaylist"] = p.AddToPlaylist(ctx, "p.1", []string{"s1"})
	_, checks["Favorite"] = p.Favorite(ctx, "s1")
	checks["SetFavorite"] = p.SetFavorite(ctx, "s1", true)
	_, checks["PlaySongs"] = p.PlaySongs(ctx, []string{"s1"}, 0)
	for name, err := range checks {
		if !errors.Is(err, playback.ErrUnsupported) {
			t.Errorf("%s = %v, want ErrUnsupported", name, err)
		}
	}
	if pls, err := p.Playlists(ctx); err != nil || len(pls) != 1 {
		t.Errorf("Playlists = %v, %v", pls, err)
	}
	// Local is active from the start: its states drive the UI.
	local.PushState(playback.State{Title: "local"})
	if s := recv(t, p.States()); s.Title != "local" {
		t.Errorf("state = %+v", s)
	}
}

func TestAppleRefusalLeavesLocalUsable(t *testing.T) {
	p, apple, _ := newPair(t)
	apple.AuthStatus = playback.AuthDenied
	if s, err := p.Authorize(ctx); err != nil || s != playback.AuthAuthorized {
		t.Fatalf("Authorize = %v, %v; want authorized for the local files", s, err)
	}
	if err := recv(t, p.Errors()); err == nil {
		t.Error("the refusal was not reported")
	}
	if p.Supports(playback.CapCatalogSearch, "") {
		t.Error("search should be unsupported once Apple Music refused")
	}
	if _, err := p.SearchCatalog(ctx, "x", 5); !errors.Is(err, playback.ErrUnsupported) {
		t.Errorf("SearchCatalog = %v", err)
	}
}

func TestFavoritesAnswerLocalIdsAsNotLoved(t *testing.T) {
	p, apple, _ := newPair(t)
	apple.Loved = map[string]bool{"s1": true}
	got, err := p.Favorites(ctx, []string{"s1", loc1})
	if err != nil {
		t.Fatal(err)
	}
	if !got["s1"] || got[loc1] {
		t.Errorf("Favorites = %v", got)
	}
	if c := called(apple, "Favorites"); len(c) != 1 || !reflect.DeepEqual(c[0].Args, []any{[]string{"s1"}}) {
		t.Errorf("Apple Favorites = %+v, want only the Apple id", c)
	}
}

func TestCloseClosesBothAndTheChannels(t *testing.T) {
	apple, local := playbacktest.New(), playbacktest.New()
	p := New(apple, local)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if !apple.Closed() || !local.Closed() {
		t.Error("Close did not close both backends")
	}
	for range p.States() {
	}
	for range p.Errors() {
	}
	for range p.Levels() {
	}
	_ = p.Close()
}

var (
	_ playback.Player         = (*Player)(nil)
	_ playback.LevelSource    = (*Player)(nil)
	_ playback.Capabilities   = (*Player)(nil)
	_ playback.LibraryWatcher = (*Player)(nil)
)

func TestAHelperThatExitsHandsOverToLocal(t *testing.T) {
	p, apple, local := newPair(t)
	_ = apple.Close() // the helper exited
	if s := recv(t, p.States()); s.Status != playback.StatusStopped {
		t.Fatalf("state = %+v, want stopped", s)
	}
	if err := recv(t, p.Errors()); err == nil {
		t.Fatal("the exit was not reported")
	}
	if p.Supports(playback.CapCatalogSearch, "") {
		t.Error("the catalog outlived the helper")
	}
	local.PushState(playback.State{Title: "local"})
	if s := recv(t, p.States()); s.Title != "local" {
		t.Errorf("state = %+v, want the local backend's", s)
	}
}
