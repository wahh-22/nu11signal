package androidtv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

// The music page and the terminal share the app's single WebView renderer
// thread. For several seconds after MusicKit is ready, Apple's web app keeps
// that thread busy with many short tasks: a trivial evaluation still answers
// fast, yet the terminal barely redraws. The startup splash therefore holds
// until the page measures its thread mostly free (busyProbe) settleStreak
// windows in a row.
const (
	settleWindow  = time.Second
	settleIdle    = 300 * time.Millisecond // busy time per window that still feels responsive
	settleStreak  = 2
	settleLimit   = 20 * time.Second       // never hold startup longer than this
	settleTimeout = 3 * time.Second        // one probe; a page that cannot answer counts as busy
	settleRetry   = 250 * time.Millisecond // pause after a failed probe
)

// busyProbe chains zero-delay timers for one window in the music page and
// returns how many milliseconds of it went to other work: each tick's delay
// beyond the 4 ms nested-timer clamp.
var busyProbe = fmt.Sprintf(`new Promise(done => {
  let last = performance.now(), busy = 0;
  const end = last + %d;
  (function tick() {
    const now = performance.now();
    busy += Math.max(0, now - last - 4);
    last = now;
    if (now < end) setTimeout(tick, 0); else done(busy);
  })();
})`, settleWindow.Milliseconds())

// WaitSettled waits for the music page to stop monopolizing the renderer
// thread, or settleLimit. Only ctx's end is an error: a page that never
// settles must not keep the UI from starting.
func (r *Runtime) WaitSettled(ctx context.Context) error {
	probe := func(ctx context.Context, budget time.Duration) (time.Duration, error) {
		ctx, cancel := context.WithTimeout(ctx, budget)
		defer cancel()
		raw, err := r.Bridge.Evaluate(ctx, busyProbe)
		if err != nil {
			return 0, err
		}
		return parseBusy(raw)
	}
	return waitSettled(ctx, probe, time.Now, sleepCtx, settleIdle, settleStreak, settleLimit)
}

// waitSettled probes until streak consecutive windows each report at most
// idle busy time, or limit has passed: each probe gets at most the time
// left (and settleTimeout), so the hold never outlasts limit. A failed probe
// counts as busy and waits settleRetry before the next, so a page that fails
// at once (navigating) is not polled in a tight loop. A gone browser ends
// the hold: it will never answer, and the UI handles the closed bridge.
func waitSettled(ctx context.Context, probe func(context.Context, time.Duration) (time.Duration, error),
	now func() time.Time, sleep func(context.Context, time.Duration) error,
	idle time.Duration, streak int, limit time.Duration) error {
	deadline := now().Add(limit)
	run := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		left := deadline.Sub(now())
		if left <= 0 {
			return nil
		}
		busy, err := probe(ctx, min(left, settleTimeout))
		if ctx.Err() != nil {
			return ctx.Err()
		}
		switch {
		case errors.Is(err, webplayer.ErrBrowserGone):
			return nil
		case err != nil:
			run = 0
			if !now().Before(deadline) {
				return nil
			}
			if err := sleep(ctx, min(settleRetry, deadline.Sub(now()))); err != nil {
				return err
			}
		case busy <= idle:
			run++
			if run >= streak {
				return nil
			}
		default:
			run = 0
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// parseBusy reads the probe's milliseconds.
func parseBusy(raw json.RawMessage) (time.Duration, error) {
	var ms float64
	if err := json.Unmarshal(raw, &ms); err != nil {
		return 0, fmt.Errorf("androidtv: busy probe: %w", err)
	}
	return time.Duration(max(ms, 0) * float64(time.Millisecond)), nil
}
