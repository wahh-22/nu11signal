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
