package demo

import (
	"context"
	"errors"
	"testing"
	"time"

	"soulking/internal/playback"
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
	songs, err := p.Search(ctx, "NEON", 25)
	if err != nil || len(songs) == 0 {
		t.Fatalf("Search = %d, %v; want matches", len(songs), err)
	}
	if limited, _ := p.Search(ctx, "", 2); len(limited) != 2 {
		t.Errorf("Search limit 2 returned %d songs", len(limited))
	}
	ids := []string{songs[0].ID}
	if err := p.PlaySongs(ctx, ids, 0); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, func(s playback.State) bool { return s.SongID == songs[0].ID && s.Status == playback.StatusPlaying })
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
