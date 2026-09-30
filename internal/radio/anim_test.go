package radio

import (
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// runCadence drives the tick chain as the program does for d: each tick
// is the one the chain scheduled, delivered once its interval passed.
func runCadence(t *testing.T, m Model, c *clock, d time.Duration) Model {
	t.Helper()
	for end := c.t.Add(d); c.t.Before(end); {
		msg := m.nextTick()
		c.advance(m.tickInterval())
		m, _ = step(t, m, msg)
	}
	return m
}

// TestEffectsKeepTheAnimationPace checks that the redraws of an intro
// (introTick) or a burst (burstTick) never speed the animation up: the
// rain steps once per fastTick of wall time with or without them.
func TestEffectsKeepTheAnimationPace(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, m Model, c *clock) (Model, time.Duration)
	}{
		{"no effect", func(_ *testing.T, m Model, _ *clock) (Model, time.Duration) {
			return m, introDur
		}},
		{"intro", func(t *testing.T, m Model, c *clock) (Model, time.Duration) {
			m, _ = press(t, m, "/")
			if !m.intro.running(c.t) {
				t.Fatal("opening SEARCH started no intro")
			}
			return m, introDur
		}},
		{"burst", func(t *testing.T, m Model, c *clock) (Model, time.Duration) {
			m = forceBurst(t, m, c, false)
			return m, m.fx.burstEnd.Sub(c.t)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newClock()
			m := introModel(t, c)
			m = runCadence(t, m, c, time.Second) // settle into the chain
			m, window := tt.setup(t, m, c)
			before := m.rain.tick
			m = runCadence(t, m, c, window)
			got, want := int(m.rain.tick-before), int(window/fastTick)
			if got < want-1 || got > want+1 {
				t.Fatalf("rain stepped %d times in %v, want %d±1 (one per %v)", got, window, want, fastTick)
			}
		})
	}
}

// TestAnIntroKeepsTheTitleGlitchLength checks that the song-change
// glitch of the title lasts the same wall time with an intro running.
func TestAnIntroKeepsTheTitleGlitchLength(t *testing.T) {
	for _, withIntro := range []bool{false, true} {
		c := newClock()
		m := introModel(t, c)
		m = runCadence(t, m, c, time.Second)
		if withIntro {
			m, _ = press(t, m, "/")
		}
		next := playing(0, 200*time.Second)
		next.SongID, next.Title = "next-song", "Another Song"
		m, _ = step(t, m, stateMsg{state: next})
		if m.glitch != glitchFrames {
			t.Fatalf("intro=%v: glitch = %d, want %d", withIntro, m.glitch, glitchFrames)
		}
		start := c.t
		for i := 0; m.glitch > 0; i++ {
			if i == 100 {
				t.Fatalf("intro=%v: the glitch never settled", withIntro)
			}
			m = runCadence(t, m, c, time.Nanosecond)
		}
		got, want := c.t.Sub(start), glitchFrames*fastTick
		if got < want-fastTick || got > want+fastTick {
			t.Fatalf("intro=%v: the title glitched for %v, want about %v", withIntro, got, want)
		}
	}
}

// TestPausedAnimationStaysIdleThroughABurst checks that a burst while
// paused redraws at burstTick but moves no animation: bars stay flat
// and the rain holds.
func TestPausedAnimationStaysIdleThroughABurst(t *testing.T) {
	c := newClock()
	m := introModel(t, c)
	paused := playing(90*time.Second, 225*time.Second)
	paused.Status = playback.StatusPaused
	m, _ = step(t, m, stateMsg{state: paused})
	m = runCadence(t, m, c, 3*time.Second)
	if m.tickFast {
		t.Fatal("paused: still on the fast tick")
	}
	rain := m.rain.tick
	m = forceBurst(t, m, c, false)
	m = runCadence(t, m, c, m.fx.burstEnd.Sub(c.t))
	if !m.bars.flat() || m.rain.tick != rain {
		t.Fatalf("a paused burst animated: bars %v, rain %d -> %d", m.bars, rain, m.rain.tick)
	}
}
