package androidtv

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
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
	settleLimit   = 20 * time.Second // never hold startup longer than this
	settleTimeout = 3 * time.Second  // one probe; a page that cannot answer counts as busy
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
	probe := func(ctx context.Context) (time.Duration, error) {
		ctx, cancel := context.WithTimeout(ctx, settleTimeout)
		defer cancel()
		raw, err := r.Bridge.Evaluate(ctx, busyProbe)
		if err != nil {
			return 0, err
		}
		return parseBusy(raw)
	}
	return waitSettled(ctx, probe, time.Now, settleIdle, settleStreak, settleLimit)
}

// waitSettled probes until streak consecutive windows each report at most
// idle busy time, or limit has passed. A failed probe counts as busy.
func waitSettled(ctx context.Context, probe func(context.Context) (time.Duration, error),
	now func() time.Time, idle time.Duration, streak int, limit time.Duration) error {
	deadline := now().Add(limit)
	run := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		busy, err := probe(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil && busy <= idle {
			run++
			if run >= streak {
				return nil
			}
		} else {
			run = 0
		}
		if !now().Before(deadline) {
			return nil
		}
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
