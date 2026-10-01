package radio

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// fxModel is a loaded, playing model at 80x24 with the signal effects on,
// armed by one tick at the clock's time.
func fxModel(t *testing.T, c *clock) Model {
	t.Helper()
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	m := New(f, Options{Now: c.now, Seed: 2077, Effects: true})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	// The playlists, without the tick the intro of the new rows raised.
	for _, msg := range runAll(t, cmd) {
		if _, ok := msg.(tickMsg); !ok {
			m, _ = step(t, m, msg)
		}
	}
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	// Let the title's song-change glitch settle, and forget the intro of
	// the first frames: the clock stands still here (see introModel).
	for range glitchFrames + 1 {
		m = tick(t, m)
	}
	m.intro = intro{}
	return m
}

func tick(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = step(t, m, tickMsg{gen: m.tickGen})
	return m
}

// forceBurst moves the clock to the next scheduled bursts until one
// starts whose NO SIGNAL flash is noSignal, and returns the model on its
// first frame.
func forceBurst(t *testing.T, m Model, c *clock, noSignal bool) Model {
	t.Helper()
	for range 200 {
		c.t = m.fx.nextBurst
		m = tick(t, m)
		if !m.fx.bursting(c.t) {
			t.Fatalf("no burst at the scheduled time %v", c.t)
		}
		if m.fx.noSignal == noSignal {
			return m
		}
		c.t = m.fx.burstEnd
		m = tick(t, m)
	}
	t.Fatalf("no burst with noSignal=%v in 200 bursts", noSignal)
	return m
}

// cells splits a stripped line into its cells; a wide rune takes two, the
// second empty.
func cells(line string) []string {
	var out []string
	for _, r := range ansi.Strip(line) {
		switch ansi.StringWidth(string(r)) {
		case 0:
			continue
		case 2:
			out = append(out, string(r), "")
		default:
			out = append(out, string(r))
		}
	}
	return out
}

func TestBurstScheduleStaysInRange(t *testing.T) {
	const stepDur = 50 * time.Millisecond
	bursts, flashes := 0, 0
	for seed := uint64(1); seed <= 20; seed++ {
		start := time.Date(2077, 1, 1, 0, 0, 0, 0, time.UTC)
		var e effects
		e.on = true
		var lastEnd time.Time
		seq := uint64(0)
		for now := start; now.Before(start.Add(30 * time.Minute)); now = now.Add(stepDur) {
			e = e.advance(now, seed, true)
			if e.burstSeq == seq {
				continue
			}
			seq = e.burstSeq
			bursts++
			if d := e.burstEnd.Sub(e.burstStart); d < burstMin || d > burstMax {
				t.Errorf("seed %d burst %d lasts %v, want %v..%v", seed, seq, d, burstMin, burstMax)
			}
			from := lastEnd
			if from.IsZero() {
				from = start
			}
			if gap := e.burstStart.Sub(from); gap < burstGapMin || gap > burstGapMax+stepDur {
				t.Errorf("seed %d burst %d comes %v after the last, want %v..%v", seed, seq, gap, burstGapMin, burstGapMax)
			}
			if e.noSignal {
				flashes++
				if d := e.burstEnd.Sub(e.burstStart); d >= time.Second {
					t.Errorf("NO SIGNAL flash lasts %v, want under 1s", d)
				}
			}
			lastEnd = e.burstEnd
		}
	}
	if bursts < 20*30*60/int(burstGapMax/time.Second) {
		t.Fatalf("only %d bursts in 20 half hours", bursts)
	}
	if frac := float64(flashes) / float64(bursts); frac < 0.15 || frac > 0.35 {
		t.Errorf("NO SIGNAL in %.2f of the bursts, want about one in four", frac)
	}
}

func TestPausedEffectsNeverBurst(t *testing.T) {
	start := time.Date(2077, 1, 1, 0, 0, 0, 0, time.UTC)
	e := effects{on: true}
	for now := start; now.Before(start.Add(10 * time.Minute)); now = now.Add(100 * time.Millisecond) {
		e = e.advance(now, 3, false)
		if e.bursting(now) {
			t.Fatalf("paused effects burst at %v", now.Sub(start))
		}
	}
	// Resuming does not fire the bursts missed while paused at once.
	now := start.Add(10 * time.Minute)
	if e = e.advance(now, 3, true); e.bursting(now) {
		t.Fatal("resuming fired a missed burst at once")
	}
}

func TestBurstKeepsWidthsAndZones(t *testing.T) {
	for _, noSignal := range []bool{false, true} {
		c := newClock()
		m := forceBurst(t, fxModel(t, c), c, noSignal)
		for frame := 0; m.fx.bursting(c.t); frame++ {
			base, baseZones := m.baseLayout()
			lines, zs := m.layout()
			if !reflect.DeepEqual(zs, baseZones) {
				t.Fatalf("noSignal=%v frame %d: burst changed the zones", noSignal, frame)
			}
			if len(lines) != len(base) {
				t.Fatalf("noSignal=%v frame %d: %d lines, want %d", noSignal, frame, len(lines), len(base))
			}
			for i := range lines {
				if got, want := ansi.StringWidth(lines[i]), ansi.StringWidth(base[i]); got != want {
					t.Errorf("noSignal=%v frame %d line %d is %d cells, want %d: %q", noSignal, frame, i, got, want, ansi.Strip(lines[i]))
				}
			}
			c.advance(burstTick)
			m = tick(t, m)
		}
	}
}

func TestBurstGlitchesAndFlashesNoSignal(t *testing.T) {
	c := newClock()
	m := forceBurst(t, fxModel(t, c), c, false)
	base, _ := m.baseLayout()
	lines, _ := m.layout()
	if reflect.DeepEqual(lines, base) {
		t.Fatal("a burst left the frame untouched")
	}
	if strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "NO SIGNAL") {
		t.Fatal("a plain burst flashed NO SIGNAL")
	}
	m = forceBurst(t, m, c, true)
	if out := ansi.Strip(m.render()); !strings.Contains(out, "N O   S I G N A L") {
		t.Fatalf("NO SIGNAL burst lacks the flash:\n%s", out)
	}
	// After the burst the frame returns to normal and the tick slows down.
	c.t = m.fx.burstEnd
	m = tick(t, m)
	if m.fx.bursting(c.t) || m.tickInterval() == burstTick {
		t.Fatal("burst outlived its end")
	}
}

// TestRenderFPSShowsEveryAnimationFrame guards the renderer frame rate:
// its frame period must not exceed any animation step, or a step would
// share a flush with the next and never reach the screen.
func TestRenderFPSShowsEveryAnimationFrame(t *testing.T) {
	period := time.Second / RenderFPS
	for name, step := range map[string]time.Duration{
		"fastTick": fastTick, "introTick": introTick, "burstTick": burstTick,
	} {
		if period > step {
			t.Errorf("frame period %v exceeds %s %v", period, name, step)
		}
	}
	// Bubble Tea's default is 60; the point is to run slower than that.
	if RenderFPS >= 60 {
		t.Errorf("RenderFPS = %d, want fewer than the default 60 frames a second", RenderFPS)
	}
}

func TestTickRateRisesOnlyDuringBursts(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	if got := m.tickInterval(); got != fastTick {
		t.Fatalf("playing tick = %v, want %v", got, fastTick)
	}
	// A burst runs faster than the playing tick.
	m = forceBurst(t, m, c, false)
	if got := m.tickInterval(); got != burstTick {
		t.Fatalf("playing burst tick = %v, want %v", got, burstTick)
	}
	c.t = m.fx.burstEnd
	m = tick(t, m)
	still := playing(90*time.Second, 225*time.Second)
	still.Status = playback.StatusPaused
	m, _ = step(t, m, stateMsg{state: still})
	for range 20 {
		m = tick(t, m) // let the EQ settle flat
	}
	// Between the effects the tick sleeps until the next one is due.
	want := min(idleTick, m.fx.nextBurst.Sub(c.t))
	if got := m.tickInterval(); got != want {
		t.Fatalf("idle tick with effects = %v, want %v (to the next effect)", got, want)
	}
	m, _ = press(t, m, keyEffects)
	if got := m.tickInterval(); got != idleTick {
		t.Fatalf("idle tick without effects = %v, want %v", got, idleTick)
	}
}

func TestEffectsOffLeaveFrameUntouched(t *testing.T) {
	t.Run("calm by default", func(t *testing.T) {
		c := newClock()
		m := loaded(t, playbacktest.New(), c)
		m, _ = step(t, m, stateMsg{state: playing(0, 225*time.Second)})
		for range 600 {
			c.advance(500 * time.Millisecond)
			m = tick(t, m)
			if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
				t.Fatalf("effects drew at %v with Effects off", c.t)
			}
		}
	})
	t.Run("toggled off during a burst", func(t *testing.T) {
		c := newClock()
		m := forceBurst(t, fxModel(t, c), c, true)
		m, _ = press(t, m, keyEffects)
		if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
			t.Fatal("effects still drawn after the toggle")
		}
		if !strings.Contains(m.status, "FX OFF") {
			t.Fatalf("status = %q, want the toggle confirmed", m.status)
		}
		m, _ = press(t, m, keyEffects)
		if !m.fx.on || !strings.Contains(m.status, "FX ON") {
			t.Fatal("second toggle did not turn the effects back on")
		}
	})
	t.Run("typing in SEARCH", func(t *testing.T) {
		c := newClock()
		m := fxModel(t, c)
		next := m.fx.nextBurst
		m, _ = press(t, m, "/")
		c.t = next
		m = tick(t, m)
		if m.fx.bursting(c.t) {
			t.Fatal("a burst started while typing")
		}
		if m.fxActive() {
			t.Fatal("effects active while the SEARCH input takes the keys")
		}
		if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
			t.Fatal("effects drawn while typing")
		}
	})
	t.Run("typing a NEW PLAYLIST name", func(t *testing.T) {
		c := newClock()
		m := fxModel(t, c)
		m, _ = press(t, m, "up", "enter") // + NEW PLAYLIST
		if m.editor.mode != editName {
			t.Fatalf("editor mode = %v, want the name input", m.editor.mode)
		}
		if m.fxActive() {
			t.Fatal("effects active while the name input takes the keys")
		}
	})
	t.Run("tiny layout", func(t *testing.T) {
		c := newClock()
		m := forceBurst(t, fxModel(t, c), c, true)
		m, _ = step(t, m, tea.WindowSizeMsg{Width: 15, Height: 4})
		if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
			t.Fatal("effects drawn on the tiny layout")
		}
	})
}

func first(lines []string, _ zones) []string { return lines }

func TestGlitchGolden80x24(t *testing.T) {
	for _, tt := range []struct {
		name     string
		noSignal bool
	}{{"glitch_burst", false}, {"no_signal", true}} {
		t.Run(tt.name, func(t *testing.T) {
			c := newClock()
			m := forceBurst(t, fxModel(t, c), c, tt.noSignal)
			assertGolden(t, tt.name+"_80x24.golden", ansi.Strip(m.View().Content))
		})
	}
}

// textCell reports whether a base cell holds text a text glitch may
// scramble: not blank, not a border or a shade.
func textCell(s string) bool {
	if s == "" || s == " " {
		return false
	}
	r := []rune(s)[0]
	return r < 0x2500 || r > 0x259F
}

// skeleton is line with every printable cell replaced by a dot: its escape
// sequences, where they are, and its cell count.
func skeleton(line string) string {
	var b strings.Builder
	var state byte
	for len(line) > 0 {
		seq, w, n, next := ansi.DecodeSequence(line, state, nil)
		if w > 0 {
			b.WriteString(strings.Repeat("·", w))
		} else {
			b.WriteString(seq)
		}
		line, state = line[n:], next
	}
	return b.String()
}

func TestBurstsSpareARealStatus(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	for _, noSignal := range []bool{false, true, false, true} {
		m = forceBurst(t, m, c, noSignal)
		m.setStatus("PLAY FAILED // HELPER GONE")
		if !m.fx.bursting(c.t) {
			t.Fatalf("setting a status ended the burst (noSignal=%v)", noSignal)
		}
		base, _ := m.baseLayout()
		lines, _ := m.layout()
		n := len(lines)
		if reflect.DeepEqual(lines[:n-2], base[:n-2]) {
			t.Fatalf("burst (noSignal=%v) drew nothing while a status showed", noSignal)
		}
		if !reflect.DeepEqual(lines[n-2:], base[n-2:]) {
			t.Fatalf("burst (noSignal=%v) touched the status or hint line:\n%q\n%q", noSignal, ansi.Strip(lines[n-2]), ansi.Strip(lines[n-1]))
		}
	}
}

// TestNothingDrawsBetweenBursts: the bursts are the only signal effect;
// between two of them the frame is left as it is (no text wave).
func TestNothingDrawsBetweenBursts(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	for burst := range 3 {
		if burst > 0 {
			c.t = m.fx.nextBurst
			m = tick(t, m)
			c.t = m.fx.burstEnd
			m = tick(t, m)
		}
		from, to := c.t, m.fx.nextBurst
		for c.t = from; c.t.Before(to); c.t = c.t.Add(250 * time.Millisecond) {
			m = tick(t, m)
			m.intro = intro{}
			if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
				t.Fatalf("gap %d: an effect drew %v before the next burst:\n%s", burst, to.Sub(c.t), plain(m))
			}
		}
	}
}

// TestBurstSaltsStayPut pins the salts of the burst streams: retiring an
// effect must not shift them, or every burst would look different.
func TestBurstSaltsStayPut(t *testing.T) {
	got := []uint64{saltBurstGap, saltBurstLen, saltNoSignal, saltBurst}
	if want := []uint64{101, 102, 103, 106}; !reflect.DeepEqual(got, want) {
		t.Fatalf("burst salts = %v, want %v", got, want)
	}
}

func TestEffectTimings(t *testing.T) {
	for _, tt := range []struct {
		name      string
		got, want time.Duration
	}{
		// Bursts are short and sharp, a real loss of signal.
		{"burstMin", burstMin, 600 * time.Millisecond},
		{"burstMax", burstMax, 1000 * time.Millisecond},
		{"burstTick", burstTick, 66 * time.Millisecond},
		// With no text wave between them, bursts come often.
		{"burstGap", burstGapMin, 10 * time.Second},
		{"burstGapMax", burstGapMax, 22 * time.Second},
	} {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
	if noSignalOdds != 4 {
		t.Errorf("noSignalOdds = %d, want 4", noSignalOdds)
	}
}

func TestBurstsTearAndCorruptLikeASignalLoss(t *testing.T) {
	start := time.Date(2077, 1, 1, 0, 0, 0, 0, time.UTC)
	frames, bars := 0, 0
	for seed := uint64(1); seed <= 20; seed++ {
		e := effects{on: true}.advance(start, seed, true)
		for range 12 {
			e = e.advance(e.nextBurst, seed, true)
			for frame := range uint64(8) {
				l := e.look(seed, frame, 24)
				frames++
				if n := len(l.tears); n < 1 || n > 3 {
					t.Fatalf("seed %d: %d torn rows, want 1..3", seed, n)
				}
				for _, tr := range l.tears {
					if tr.k < 1 || tr.k > 2 || tr.y < 0 || tr.y >= 24 {
						t.Fatalf("seed %d: tear %+v, want 1..2 cells on one of 24 rows", seed, tr)
					}
				}
				lo, hi := 4, 10
				if e.noSignal {
					lo, hi = 2*lo, 2*hi
				}
				if n := len(l.noise); n < lo || n > hi {
					t.Fatalf("seed %d: %d noise cells, want %d..%d", seed, n, lo, hi)
				}
				if l.sign != e.noSignal {
					t.Fatalf("seed %d frame %d: sign %v, want it on every frame of a NO SIGNAL burst only", seed, frame, l.sign)
				}
				if l.bar >= 0 {
					bars++
				}
			}
			// It changes every frame.
			if reflect.DeepEqual(e.look(seed, 0, 24), e.look(seed, 1, 24)) {
				t.Fatalf("seed %d: two frames of a burst look the same", seed)
			}
		}
	}
	if frac := float64(bars) / float64(frames); frac < 0.25 || frac > 0.42 {
		t.Errorf("static bar on %.2f of the burst frames, want about one in three", frac)
	}
}

func TestNoSignalFlashesFullRedFramedInStatic(t *testing.T) {
	c := newClock()
	m := forceBurst(t, fxModel(t, c), c, true)
	sign := "  " + spaced("NO SIGNAL") + "  "
	for frame := 0; m.fx.bursting(c.t); frame++ {
		lines, _ := m.layout()
		at := -1
		for y, line := range lines {
			if strings.Contains(line, stRedBold.Render(sign)) {
				at = y
			}
		}
		if at < 1 || at+1 >= len(lines) {
			t.Fatalf("frame %d: no full red NO SIGNAL sign:\n%s", frame, ansi.Strip(strings.Join(lines, "\n")))
		}
		for _, y := range []int{at - 1, at + 1} {
			plainRow := ansi.Strip(lines[y])
			if shades := strings.Count(plainRow, "░") + strings.Count(plainRow, "▒") + strings.Count(plainRow, "▓"); shades < ansi.StringWidth(sign)+4 {
				t.Fatalf("frame %d: row %d does not frame the sign in static: %q", frame, y, ansi.Strip(lines[y]))
			}
		}
		c.advance(burstTick)
		m = tick(t, m)
	}
}

func TestBurstTick(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	still := playing(90*time.Second, 225*time.Second)
	still.Status = playback.StatusPaused
	m, _ = step(t, m, stateMsg{state: still})
	for range 20 {
		m = tick(t, m)
	}
	m = forceBurst(t, m, c, false)
	if got := m.tickInterval(); got != burstTick {
		t.Fatalf("idle burst tick = %v, want %v", got, burstTick)
	}
	// Near the end the tick is cut short so the frame clears on time.
	c.t = m.fx.burstEnd.Add(-30 * time.Millisecond)
	if got := m.tickInterval(); got != 30*time.Millisecond {
		t.Fatalf("burst tick near the end = %v, want 30ms", got)
	}
}
