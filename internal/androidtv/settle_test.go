package androidtv

import (
	"context"
	"errors"
	"testing"
	"time"
)

// settleProbe scripts waitSettled: each probe reports the next busy time (how
// long, within one settleWindow, the page's thread was taken by other work),
// or fails; probes only advance a fake clock.
type settleProbe struct {
	t     time.Time
	busy  []time.Duration
	fail  map[int]bool
	calls int
}

func (p *settleProbe) now() time.Time { return p.t }

func (p *settleProbe) probe(ctx context.Context) (time.Duration, error) {
	i := p.calls
	p.calls++
	p.t = p.t.Add(settleWindow)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if p.fail[i] {
		return 0, errors.New("page navigated")
	}
	if i < len(p.busy) {
		return p.busy[i], nil
	}
	return 50 * time.Millisecond, nil
}

func runSettle(ctx context.Context, p *settleProbe) error {
	return waitSettled(ctx, p.probe, p.now, settleIdle, settleStreak, settleLimit)
}

// A thread busy 90% of the time answers short evaluations quickly yet still
// starves the terminal: only windows that leave it mostly free count.
func TestWaitSettledNeedsAStreakOfQuietWindows(t *testing.T) {
	busy := 900 * time.Millisecond
	quiet := 80 * time.Millisecond
	p := &settleProbe{t: time.Unix(0, 0), busy: []time.Duration{busy, busy, quiet, busy, quiet, quiet}}
	if err := runSettle(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if p.calls != 6 {
		t.Fatalf("settled after %d probes; want 6 (a busy window resets the streak)", p.calls)
	}
}

func TestWaitSettledCountsErrorsAsBusy(t *testing.T) {
	p := &settleProbe{t: time.Unix(0, 0), fail: map[int]bool{0: true}}
	if err := runSettle(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if p.calls != 1+settleStreak {
		t.Fatalf("settled after %d probes; want %d (the failure counts as busy)", p.calls, 1+settleStreak)
	}
}

func TestWaitSettledGivesUpAtTheLimit(t *testing.T) {
	p := &settleProbe{t: time.Unix(0, 0)}
	for range 1000 {
		p.busy = append(p.busy, 950*time.Millisecond)
	}
	start := p.t
	if err := runSettle(context.Background(), p); err != nil {
		t.Fatalf("a page that never settles must not fail startup: %v", err)
	}
	if waited := p.t.Sub(start); waited < settleLimit || waited > settleLimit+settleWindow {
		t.Fatalf("gave up after %v; want about %v", waited, settleLimit)
	}
}

func TestWaitSettledStopsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &settleProbe{t: time.Unix(0, 0)}
	if err := runSettle(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v; want context.Canceled", err)
	}
}

func TestParseBusy(t *testing.T) {
	for raw, want := range map[string]time.Duration{"812.4": 812400 * time.Microsecond, "0": 0, "-3": 0} {
		got, err := parseBusy([]byte(raw))
		if err != nil || got != want {
			t.Errorf("parseBusy(%s) = %v, %v; want %v", raw, got, err, want)
		}
	}
	if _, err := parseBusy([]byte(`"x"`)); err == nil {
		t.Error("a non-number was accepted")
	}
}
