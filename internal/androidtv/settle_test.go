package androidtv

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

// settleProbe scripts waitSettled: each probe reports the next busy time (how
// long, within one settleWindow, the page's thread was taken by other work),
// or fails; probes only advance a fake clock.
type settleProbe struct {
	t      time.Time
	busy   []time.Duration
	fail   map[int]bool
	failAt func(i int) error // overrides fail; the probe then fails instantly
	calls  int
	sleeps int
	budget []time.Duration // the time each probe was allowed
	// slow makes each probe use its whole budget, as a page that never
	// answers does; otherwise a probe takes one window at most.
	slow bool
	// during runs inside each probe, as work in flight would.
	during func()
	// pausing runs inside each pause after a failed probe.
	pausing func()
}

func (p *settleProbe) now() time.Time { return p.t }

func (p *settleProbe) probe(ctx context.Context, budget time.Duration) (time.Duration, error) {
	i := p.calls
	p.calls++
	p.budget = append(p.budget, budget)
	if p.failAt != nil {
		if err := p.failAt(i); err != nil {
			return 0, err
		}
	}
	if p.during != nil {
		p.during()
	}
	if p.slow {
		p.t = p.t.Add(budget)
	} else {
		p.t = p.t.Add(min(settleWindow, budget))
	}
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

func (p *settleProbe) sleep(ctx context.Context, d time.Duration) error {
	p.sleeps++
	if p.pausing != nil {
		p.pausing()
	}
	p.t = p.t.Add(d)
	return ctx.Err()
}

func runSettle(ctx context.Context, p *settleProbe) error {
	return waitSettled(ctx, p.probe, p.now, p.sleep, settleIdle, settleStreak, settleLimit)
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
	if waited := p.t.Sub(start); waited != settleLimit {
		t.Fatalf("gave up after %v; want exactly %v", waited, settleLimit)
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

// A probe that fails at once (the page is navigating) must not spin: each
// failure waits before the next probe, so a 20 s hold is not a busy loop.
func TestWaitSettledWaitsAfterInstantFailures(t *testing.T) {
	p := &settleProbe{t: time.Unix(0, 0), failAt: func(int) error { return errors.New("page navigated") }}
	if err := runSettle(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if p.sleeps != p.calls-1 && p.sleeps != p.calls {
		t.Fatalf("%d probes but %d waits: failures retried without waiting", p.calls, p.sleeps)
	}
	if max := int(settleLimit/settleRetry) + 1; p.calls > max {
		t.Fatalf("%d probes in %v; want at most %d", p.calls, settleLimit, max)
	}
}

// A closed bridge never answers again: stop holding at once.
func TestWaitSettledStopsWhenTheBrowserIsGone(t *testing.T) {
	p := &settleProbe{t: time.Unix(0, 0), failAt: func(int) error { return webplayer.ErrBrowserGone }}
	if err := runSettle(context.Background(), p); err != nil {
		t.Fatalf("a gone browser must not fail startup here: %v", err)
	}
	if p.calls != 1 || p.sleeps != 0 {
		t.Fatalf("%d probes, %d waits after the browser was gone; want 1 and 0", p.calls, p.sleeps)
	}
}

// No probe may run past the limit: each gets at most the time left.
func TestWaitSettledBoundsEachProbeByTheTimeLeft(t *testing.T) {
	p := &settleProbe{t: time.Unix(0, 0)}
	for range 1000 {
		p.busy = append(p.busy, 950*time.Millisecond)
	}
	start := p.t
	if err := runSettle(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	for i, b := range p.budget {
		if b <= 0 || b > settleTimeout {
			t.Fatalf("probe %d allowed %v; want within (0, %v]", i, b, settleTimeout)
		}
	}
	if waited := p.t.Sub(start); waited > settleLimit {
		t.Fatalf("held %v; want at most %v", waited, settleLimit)
	}
}

// Probes that use their whole budget end exactly at the limit: the last one
// is cut to the time left, not given a full settleTimeout past it.
func TestWaitSettledCutsTheLastProbeToTheTimeLeft(t *testing.T) {
	p := &settleProbe{t: time.Unix(0, 0), slow: true}
	for range 100 {
		p.busy = append(p.busy, 950*time.Millisecond)
	}
	start := p.t
	if err := runSettle(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if waited := p.t.Sub(start); waited != settleLimit {
		t.Fatalf("held %v with slow probes; want exactly %v", waited, settleLimit)
	}
	want := settleLimit % settleTimeout
	if last := p.budget[len(p.budget)-1]; want == 0 || last != want {
		t.Fatalf("last probe allowed %v; want the %v left", last, want)
	}
}

// Cancellation during a probe, or during the pause after a failed one, ends
// the hold with the context's error and no further probe.
func TestWaitSettledStopsWhenCancelledInFlight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &settleProbe{t: time.Unix(0, 0), during: cancel}
	if err := runSettle(ctx, p); !errors.Is(err, context.Canceled) || p.calls != 1 {
		t.Fatalf("err %v after %d probes; want context.Canceled after 1", err, p.calls)
	}

	ctx, cancel = context.WithCancel(context.Background())
	p = &settleProbe{t: time.Unix(0, 0), failAt: func(int) error { return errors.New("page navigated") }, pausing: cancel}
	if err := runSettle(ctx, p); !errors.Is(err, context.Canceled) || p.calls != 1 || p.sleeps != 1 {
		t.Fatalf("err %v after %d probes and %d pauses; want context.Canceled after 1 and 1", err, p.calls, p.sleeps)
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
