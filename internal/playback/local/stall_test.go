package local

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// The stall watchdog runs on the state ticks; the tests call onStateTick
// themselves (the test clock's ticker never fires), so they decide exactly
// how much time passes between the device's pulls.

// noErrors fails when an error was reported.
func noErrors(t *testing.T, p *Player) {
	t.Helper()
	select {
	case err := <-p.Errors():
		t.Fatalf("unexpected error: %v", err)
	default:
	}
}

// ticks delivers n state ticks, as n×stateInterval passing.
func ticks(p *Player, n int) {
	for range n {
		p.onStateTick()
	}
}

func TestStallTicksCoverTheTimeout(t *testing.T) {
	if got := time.Duration(stallTicks) * stateInterval; got != stallTimeout {
		t.Fatalf("stallTicks × stateInterval = %v, want stallTimeout %v", got, stallTimeout)
	}
	if stallTimeout < 3*time.Second {
		t.Fatalf("stallTimeout = %v: too short for a slow start", stallTimeout)
	}
}

func TestOutputThatNeverPullsStopsAndReportsNoOutput(t *testing.T) {
	lib, ids := testLibrary(t)
	p, _ := newTestPlayer(t, lib)
	if _, err := p.PlaySongs(ctx, ids, 0); err != nil {
		t.Fatal(err)
	}
	ticks(p, stallTicks-1)
	noErrors(t, p)
	if s := lastState(t, p); s.Status != playback.StatusPlaying {
		t.Fatalf("status before the timeout = %v, want playing", s.Status)
	}
	ticks(p, 1)
	select {
	case err := <-p.Errors():
		if !errors.Is(err, playback.ErrNoOutput) {
			t.Fatalf("error = %v, want one wrapping playback.ErrNoOutput", err)
		}
		if !strings.Contains(err.Error(), "// ") {
			t.Errorf("error %q has no hint for the user", err)
		}
	default:
		t.Fatal("a device that never pulled was not reported")
	}
	if s := lastState(t, p); s.Status != playback.StatusStopped {
		t.Errorf("status after the timeout = %v, want stopped", s.Status)
	}
	ticks(p, 2*stallTicks)
	noErrors(t, p) // reported once, not on every tick
}

func TestOutputThatStopsPullingIsReported(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	if _, err := p.PlaySongs(ctx, ids, 0); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		sink.pull(testRate / 10)
		ticks(p, 1)
	}
	noErrors(t, p)
	ticks(p, stallTicks) // the device took the first buffers, then stopped
	select {
	case err := <-p.Errors():
		if !errors.Is(err, playback.ErrNoOutput) {
			t.Fatalf("error = %v, want one wrapping playback.ErrNoOutput", err)
		}
	default:
		t.Fatal("a device that stopped pulling was not reported")
	}
}

func TestOutputThatPullsIsNotReported(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	if _, err := p.PlaySongs(ctx, ids, 0); err != nil {
		t.Fatal(err)
	}
	// A slow start: the first pull comes just before the timeout, and the
	// next ones as late again.
	for range 4 {
		ticks(p, stallTicks-1)
		sink.pull(256)
	}
	for range 3 * stallTicks {
		sink.pull(testRate / 100)
		ticks(p, 1)
	}
	noErrors(t, p)
	if s := lastState(t, p); s.Status != playback.StatusPlaying {
		t.Errorf("status = %v, want playing", s.Status)
	}
}

func TestPausedOrStoppedOutputIsNotReported(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	if _, err := p.PlaySongs(ctx, ids, 0); err != nil {
		t.Fatal(err)
	}
	ticks(p, stallTicks-1)
	sink.pull(testRate / 10)
	if err := p.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	ticks(p, 3*stallTicks) // paused: the device may rest
	noErrors(t, p)
	if err := p.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	ticks(p, 3*stallTicks)
	noErrors(t, p)

	// Playing again starts the count over: the ticks while paused or
	// stopped do not count.
	if err := p.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	ticks(p, stallTicks-1)
	noErrors(t, p)
	ticks(p, 1)
	select {
	case err := <-p.Errors():
		if !errors.Is(err, playback.ErrNoOutput) {
			t.Fatalf("error = %v, want one wrapping playback.ErrNoOutput", err)
		}
	default:
		t.Fatal("a device that never pulled after Resume was not reported")
	}
}

func TestAwaitOutputBoundsTheOpen(t *testing.T) {
	ready := make(chan struct{})
	timeout := make(chan time.Time, 1)
	timeout <- time.Time{}
	err := awaitOutput(ready, timeout)
	if !errors.Is(err, playback.ErrNoOutput) {
		t.Fatalf("open that never finished: error = %v, want one wrapping playback.ErrNoOutput", err)
	}
	close(ready)
	if err := awaitOutput(ready, make(chan time.Time)); err != nil {
		t.Fatalf("open that finished: error = %v", err)
	}
}

// A play that stops waiting while the output is still opening leaves it
// open to a later play; a context that failed fails for good. Both wrap
// playback.ErrNoOutput with its hint.
func TestSettleOutput(t *testing.T) {
	never := make(chan struct{})
	fired := make(chan time.Time, 1)
	fired <- time.Time{}
	failed, err := settleOutput(never, fired, func() error { t.Fatal("Err asked before ready"); return nil })
	if failed != nil || !errors.Is(err, playback.ErrNoOutput) {
		t.Fatalf("still opening: failed %v, err %v; want no failure, an error wrapping playback.ErrNoOutput", failed, err)
	}

	ready := make(chan struct{})
	close(ready)
	broken := errors.New("pulse: connection refused")
	failed, err = settleOutput(ready, make(chan time.Time), func() error { return broken })
	var hint *playback.NoOutputError
	if failed == nil || err != failed || !errors.Is(err, broken) || !errors.As(err, &hint) || hint.Hint == "" {
		t.Fatalf("failed context: failed %v, err %v; want the same error wrapping %v and a NoOutputError with a hint", failed, err, broken)
	}

	if failed, err = settleOutput(ready, make(chan time.Time), func() error { return nil }); failed != nil || err != nil {
		t.Fatalf("open context: failed %v, err %v; want neither", failed, err)
	}
}

func TestNoOutputCarriesItsHint(t *testing.T) {
	var hint *playback.NoOutputError
	if err := noOutput(); !errors.Is(err, playback.ErrNoOutput) || !errors.As(err, &hint) || hint.Hint == "" {
		t.Fatalf("noOutput() = %v; want a NoOutputError with a hint, wrapping playback.ErrNoOutput", err)
	}
}
