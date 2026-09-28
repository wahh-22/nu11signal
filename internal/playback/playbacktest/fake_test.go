package playbacktest

import (
	"errors"
	"testing"
	"time"

	"soulking/internal/playback"
)

var _ playback.Player = (*Fake)(nil)

func TestFakeRecordsCallsAndReturnsCannedResults(t *testing.T) {
	f := New()
	f.SearchResult = []playback.Song{{ID: "s1"}}
	f.PlaylistsResult = []playback.Playlist{{ID: "p1"}}
	ctx := t.Context()

	if st, err := f.Authorize(ctx); err != nil || st != playback.AuthAuthorized {
		t.Fatalf("Authorize = %q, %v", st, err)
	}
	if songs, _ := f.Search(ctx, "daft", 5); len(songs) != 1 {
		t.Fatalf("Search = %v", songs)
	}
	if lists, _ := f.Playlists(ctx); len(lists) != 1 {
		t.Fatalf("Playlists = %v", lists)
	}
	_ = f.PlaySongs(ctx, []string{"s1"}, 0)
	_ = f.Seek(ctx, 3*time.Second)

	want := []Call{
		{Method: "Authorize"},
		{Method: "Search", Args: []any{"daft", 5}},
		{Method: "Playlists"},
		{Method: "PlaySongs", Args: []any{[]string{"s1"}, 0}},
		{Method: "Seek", Args: []any{3 * time.Second}},
	}
	got := f.Calls()
	if len(got) != len(want) {
		t.Fatalf("Calls = %v; want %v", got, want)
	}
	for i := range want {
		if got[i].Method != want[i].Method || len(got[i].Args) != len(want[i].Args) {
			t.Fatalf("call %d = %v; want %v", i, got[i], want[i])
		}
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
