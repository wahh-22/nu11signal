package radio

import (
	"math"
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
	m, _ = step(t, m, run(t, cmd))
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	// Let the title's song-change glitch settle.
	for range glitchFrames + 1 {
		m = tick(t, m)
	}
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
	if bursts < 20*30*60/45 {
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
		if e.bursting(now) || e.waving(now) {
			t.Fatalf("paused effects burst or wave at %v", now.Sub(start))
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
		"fastTick": fastTick, "waveTick": waveTick, "burstTick": burstTick,
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
	want := min(idleTick, m.fx.nextWave.Sub(c.t), m.fx.nextBurst.Sub(c.t))
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
		if m.fx.waving(c.t) {
			t.Fatalf("a text wave runs during the burst (noSignal=%v)", noSignal)
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

func TestWaveComesMidwayBetweenBursts(t *testing.T) {
	const stepDur = 10 * time.Millisecond
	start := time.Date(2077, 1, 1, 0, 0, 0, 0, time.UTC)
	for seed := uint64(1); seed <= 10; seed++ {
		e := effects{on: true}.advance(start, seed, true)
		// The first wave comes halfway to the first burst.
		midpoint := start.Add(e.nextBurst.Sub(start) / 2)
		if e.nextWave != midpoint {
			t.Fatalf("seed %d: first wave at %v, want %v (halfway to the first burst)", seed, e.nextWave.Sub(start), midpoint.Sub(start))
		}
		var waveSeq, burstSeq uint64
		waves, bursts := 0, 0
		for now := start; now.Before(start.Add(30 * time.Minute)); now = now.Add(stepDur) {
			e = e.advance(now, seed, true)
			if e.waveSeq != waveSeq {
				waveSeq = e.waveSeq
				waves++
				if waves != bursts+1 {
					t.Fatalf("seed %d: wave %d after %d bursts, want one wave between two bursts", seed, waves, bursts)
				}
				if d := e.waveStart.Sub(midpoint); d < 0 || d > stepDur {
					t.Errorf("seed %d wave %d starts %v off the midpoint between the bursts", seed, waves, d)
				}
				if d := e.waveEnd.Sub(e.waveStart); d < waveMin || d > waveMax {
					t.Errorf("seed %d wave %d lasts %v, want %v..%v", seed, waves, d, waveMin, waveMax)
				}
				if !e.waveEnd.Before(e.nextBurst) {
					t.Errorf("seed %d wave %d runs into the next burst", seed, waves)
				}
			}
			if e.burstSeq != burstSeq {
				burstSeq = e.burstSeq
				bursts++
				if bursts != waves {
					t.Fatalf("seed %d: burst %d after %d waves, want bursts and waves to alternate", seed, bursts, waves)
				}
				if e.waving(now) {
					t.Errorf("seed %d: burst %d starts during a wave", seed, bursts)
				}
				midpoint = e.burstEnd.Add(e.nextBurst.Sub(e.burstEnd) / 2)
				if e.nextWave != midpoint {
					t.Errorf("seed %d: wave after burst %d due at %v, want the midpoint %v", seed, bursts, e.nextWave, midpoint)
				}
			}
		}
		if waves < 30*60/45 {
			t.Fatalf("seed %d: only %d waves in half an hour", seed, waves)
		}
	}
}

func TestPausedEffectsNeverWave(t *testing.T) {
	start := time.Date(2077, 1, 1, 0, 0, 0, 0, time.UTC)
	e := effects{on: true}
	for now := start; now.Before(start.Add(5 * time.Minute)); now = now.Add(50 * time.Millisecond) {
		if e = e.advance(now, 3, false); e.waving(now) || !e.nextWave.IsZero() {
			t.Fatalf("paused effects waved at %v", now.Sub(start))
		}
	}
}

// forceWave moves the clock to the next text wave and returns the model on
// its first frame.
func forceWave(t *testing.T, m Model, c *clock) Model {
	t.Helper()
	if m.fx.nextWave.IsZero() {
		t.Fatal("no text wave scheduled")
	}
	c.t = m.fx.nextWave
	m = tick(t, m)
	if !m.fx.waving(c.t) {
		t.Fatalf("no text wave at the scheduled time %v", c.t)
	}
	if m.fx.bursting(c.t) {
		t.Fatal("the text wave fell into a burst")
	}
	return m
}

// nextWave lets the wave on screen end, then the burst after it, and
// returns the model on the first frame of the wave that follows.
func nextWave(t *testing.T, m Model, c *clock) Model {
	t.Helper()
	c.t = m.fx.waveEnd
	m = tick(t, m)
	c.t = m.fx.nextBurst
	m = tick(t, m)
	c.t = m.fx.burstEnd
	m = tick(t, m)
	return forceWave(t, m, c)
}

// scrambledCells counts the text cells of the words of base that differ
// in lines, the words with some cells changed and some not (still partly
// readable), and the lines with changed cells.
func scrambledCells(base, lines []string, words []textRun) (n, partial int, rows map[int]bool) {
	rows = map[int]bool{}
	for _, w := range words {
		got, want := cells(lines[w.y]), cells(base[w.y])
		changed := 0
		for _, x := range w.xs {
			if got[x] != want[x] {
				changed++
			}
		}
		if changed > 0 {
			rows[w.y] = true
		}
		if changed > 0 && changed < len(w.xs) {
			partial++
		}
		n += changed
	}
	return n, partial, rows
}

// textCells counts the cells of words.
func textCells(words []textRun) int {
	n := 0
	for _, w := range words {
		n += len(w.xs)
	}
	return n
}

func TestWaveScramblesMostLettersAcrossTheScreen(t *testing.T) {
	for _, keys := range [][]string{nil, {"f"}, {"enter"}} {
		c := newClock()
		m := fxModel(t, c)
		m, _ = press(t, m, keys...)
		m = forceWave(t, m, c)
		for wave := range 4 {
			if wave > 0 {
				m = nextWave(t, m, c)
			}
			// All the chosen letters scramble once the ramp-in is over.
			c.t = waveAt(m, waveRamp)
			base, _ := m.baseLayout()
			lines, _ := m.layout()
			words := textWords(base, -1)
			wordRows := map[int]bool{}
			long := 0
			for _, w := range words {
				wordRows[w.y] = true
				if len(w.xs) >= 3 {
					long++
				}
			}
			n, partial, rows := scrambledCells(base, lines, words)
			if len(words) < 20 {
				t.Fatalf("keys %v: only %d words on the frame", keys, len(words))
			}
			total := textCells(words)
			if share := float64(n) / float64(total); share < 0.35 || share > 0.45 {
				t.Fatalf("keys %v wave %d: %d of %d letters scrambled (%.2f), want about 40%%:\n%s", keys, wave, n, total, share, ansi.Strip(strings.Join(lines, "\n")))
			}
			// Letters, not whole words: many words stay partly readable.
			if partial < long/3 {
				t.Fatalf("keys %v wave %d: only %d words partly scrambled, want many of the %d longer words", keys, wave, partial, long)
			}
			if len(rows) < len(wordRows)*2/3 {
				t.Fatalf("keys %v wave %d: scrambled words on %d of the %d lines with words", keys, wave, len(rows), len(wordRows))
			}
			// The words resolve at their own moments: fewer and fewer stay
			// scrambled, and none by the end.
			initial, staggered := n, false
			for c.t.Before(m.fx.waveEnd) {
				base, baseZones := m.baseLayout()
				lines, zs := m.layout()
				if !reflect.DeepEqual(zs, baseZones) {
					t.Fatalf("keys %v: the wave changed the zones", keys)
				}
				if len(lines) != len(base) {
					t.Fatalf("keys %v: %d lines, want %d", keys, len(lines), len(base))
				}
				for y := range lines {
					if got, want := ansi.StringWidth(lines[y]), ansi.StringWidth(base[y]); got != want {
						t.Fatalf("keys %v line %d is %d cells, want %d", keys, y, got, want)
					}
					if lines[y] == base[y] {
						continue
					}
					if got, want := skeleton(lines[y]), skeleton(base[y]); got != want {
						t.Fatalf("keys %v: the wave changed the styles of line %d:\n got %q\nwant %q", keys, y, lines[y], base[y])
					}
					got, want := cells(lines[y]), cells(base[y])
					for x := range got {
						if got[x] != want[x] && !textCell(want[x]) {
							t.Fatalf("keys %v: the wave drew over %q at (%d,%d), not text", keys, want[x], x, y)
						}
					}
				}
				if n, _, _ := scrambledCells(base, lines, textWords(base, -1)); n > 0 && n < initial*2/3 {
					staggered = true
				}
				c.advance(waveTick)
				m = tick(t, m)
			}
			if !staggered {
				t.Fatalf("keys %v wave %d: the letters never resolved one by one", keys, wave)
			}
			c.t = m.fx.waveEnd
			m = tick(t, m)
			if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
				t.Fatalf("keys %v wave %d: text not whole again after the wave", keys, wave)
			}
		}
	}
}

func TestWaveSparesARealStatus(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	m = forceWave(t, m, c)
	for wave := range 5 {
		if wave > 0 {
			m = nextWave(t, m, c)
		}
		m.setStatus("PLAY FAILED // HELPER GONE")
		at := c.t
		c.t = waveAt(m, waveRamp)
		base, _ := m.baseLayout()
		lines, _ := m.layout()
		c.t = at
		n := len(lines)
		if lines[n-2] != base[n-2] {
			t.Fatalf("the wave touched the status line: %q", ansi.Strip(lines[n-2]))
		}
		if reflect.DeepEqual(lines[:n-2], base[:n-2]) {
			t.Fatal("the wave drew nothing while a status showed")
		}
		m.status = ""
	}
}

func TestNoWaveWhileTyping(t *testing.T) {
	t.Run("SEARCH input", func(t *testing.T) {
		c := newClock()
		m := fxModel(t, c)
		next := m.fx.nextWave
		m, _ = press(t, m, "/")
		c.t = next
		m = tick(t, m)
		if m.fx.waving(c.t) {
			t.Fatal("a text wave started while typing")
		}
		if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
			t.Fatal("the SEARCH input scrambled while typing")
		}
	})
	t.Run("toggled off mid-wave", func(t *testing.T) {
		c := newClock()
		m := forceWave(t, fxModel(t, c), c)
		m, _ = press(t, m, keyEffects)
		if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
			t.Fatal("the wave still drawn after the toggle")
		}
	})
}

func TestWaveTickRisesOnlyDuringTheWave(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	// While playing, the wave rides the frames that run anyway.
	m = forceWave(t, m, c)
	if got := m.tickInterval(); got != fastTick {
		t.Fatalf("playing wave tick = %v, want %v", got, fastTick)
	}
	c.t = m.fx.waveEnd
	m = tick(t, m)
	still := playing(90*time.Second, 225*time.Second)
	still.Status = playback.StatusPaused
	m, _ = step(t, m, stateMsg{state: still})
	for range 20 {
		m = tick(t, m) // let the EQ settle flat
	}
	if m.tickFast {
		t.Fatal("tick still fast while paused")
	}
	// Idle, the tick sleeps until the next effect is due.
	c.t = m.fx.nextBurst
	m = tick(t, m)
	c.t = m.fx.burstEnd
	m = tick(t, m)
	if got, want := m.tickInterval(), min(idleTick, m.fx.nextWave.Sub(c.t)); got != want {
		t.Fatalf("idle tick = %v, want %v (to the next effect)", got, want)
	}
	m = forceWave(t, m, c)
	frames := 1
	for m.fx.waving(c.t) {
		if got := m.tickInterval(); got > waveTick {
			t.Fatalf("idle wave tick = %v, want at most %v", got, waveTick)
		}
		c.advance(m.tickInterval())
		m = tick(t, m)
		frames++
	}
	if lo, hi := int(waveMin/waveTick), int(waveMax/waveTick)+2; frames < lo || frames > hi {
		t.Fatalf("idle wave took %d frames, want %d..%d (about 10 fps)", frames, lo, hi)
	}
	if got := m.tickInterval(); got <= waveTick {
		t.Fatalf("tick stayed fast after the wave: %v", got)
	}
}

func TestWaveGolden80x24(t *testing.T) {
	c := newClock()
	m := forceWave(t, fxModel(t, c), c)
	c.t = waveAt(m, (waveRamp+waveResolve)/2) // the hold: every chosen letter scrambled
	m = tick(t, m)
	assertGolden(t, "text_wave_80x24.golden", ansi.Strip(m.View().Content))
}

// waveAt is the time at share p of the text wave on screen.
func waveAt(m Model, p float64) time.Time {
	d := m.fx.waveEnd.Sub(m.fx.waveStart)
	return m.fx.waveStart.Add(time.Duration(p * float64(d)))
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
		// The text wave is slow and smooth.
		{"waveMin", waveMin, 1000 * time.Millisecond},
		{"waveMax", waveMax, 2000 * time.Millisecond},
		{"waveGlyph", waveGlyph, 160 * time.Millisecond},
		{"waveTick", waveTick, 100 * time.Millisecond},
		{"burstGap", burstGapMin, 20 * time.Second},
		{"burstGapMax", burstGapMax, 45 * time.Second},
	} {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
	if noSignalOdds != 4 {
		t.Errorf("noSignalOdds = %d, want 4", noSignalOdds)
	}
	if waveShare != 0.40 {
		t.Errorf("waveShare = %v, want 0.40 of the letters", waveShare)
	}
}

// chosenScrambled counts the text cells of base scrambled in the frame at
// time at, and the text cells the wave can scramble.
func chosenScrambled(t *testing.T, m Model, c *clock, at time.Time) (scrambled, total int) {
	t.Helper()
	was := c.t
	c.t = at
	defer func() { c.t = was }()
	base, _ := m.baseLayout()
	lines, _ := m.layout()
	ws := textWords(base, -1)
	n, _, _ := scrambledCells(base, lines, ws)
	return n, textCells(ws)
}

func TestWaveRampsInAndEasesOut(t *testing.T) {
	c := newClock()
	m := forceWave(t, fxModel(t, c), c)
	for wave := range 4 {
		if wave > 0 {
			m = nextWave(t, m, c)
		}
		_, total := chosenScrambled(t, m, c, m.fx.waveStart)
		chosen := int(math.Ceil(float64(total) * waveShare))
		// The wave sweeps in: only some letters at first, more and more,
		// every chosen one by the end of the ramp-in.
		start, _ := chosenScrambled(t, m, c, m.fx.waveStart)
		if start > chosen/3 {
			t.Fatalf("wave %d: %d of %d chosen letters scrambled at the start, want a few", wave, start, chosen)
		}
		mid, _ := chosenScrambled(t, m, c, waveAt(m, waveRamp/2))
		if mid <= start || mid >= chosen {
			t.Fatalf("wave %d: %d letters scrambled halfway through the ramp-in, want between %d and %d", wave, mid, start, chosen)
		}
		full, _ := chosenScrambled(t, m, c, waveAt(m, waveRamp))
		if full < chosen*95/100 || full > chosen {
			t.Fatalf("wave %d: %d of %d chosen letters scrambled after the ramp-in", wave, full, chosen)
		}
		// Nothing resolves before the resolve window.
		if hold, _ := chosenScrambled(t, m, c, waveAt(m, waveResolve-0.01)); hold < chosen*95/100 {
			t.Fatalf("wave %d: %d of %d letters scrambled during the hold", wave, hold, chosen)
		}
		// Eased: most letters resolve early in the window, a few linger.
		half, _ := chosenScrambled(t, m, c, waveAt(m, waveResolve+(1-waveResolve)/2))
		if half > chosen/2 {
			t.Fatalf("wave %d: %d of %d letters still scrambled halfway through the resolve, want most resolved", wave, half, chosen)
		}
		late, _ := chosenScrambled(t, m, c, waveAt(m, 0.85))
		if late == 0 {
			t.Fatalf("wave %d: no letter lingers near the end", wave)
		}
		if end, _ := chosenScrambled(t, m, c, m.fx.waveEnd.Add(-time.Nanosecond)); end > late {
			t.Fatalf("wave %d: %d letters scrambled at the end, %d near it", wave, end, late)
		}
		c.t = m.fx.waveEnd
		m = tick(t, m)
		if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
			t.Fatalf("wave %d: text not whole after the wave", wave)
		}
	}
}

func TestWaveGlyphsChangeSlowly(t *testing.T) {
	const sample = 10 * time.Millisecond
	c := newClock()
	m := forceWave(t, fxModel(t, c), c)
	from, to := waveAt(m, waveRamp), waveAt(m, waveResolve)
	type cell struct{ x, y int }
	glyph, since := map[cell]string{}, map[cell]time.Time{}
	changes := 0
	for c.t = from; c.t.Before(to); c.t = c.t.Add(sample) {
		// The base moves too (the clock, the position): only the cells
		// scrambled on this frame count.
		base, _ := m.baseLayout()
		lines, _ := m.layout()
		for y := range lines {
			got, want := cells(lines[y]), cells(base[y])
			for x := range got {
				if got[x] == want[x] {
					continue
				}
				k := cell{x, y}
				if old, ok := glyph[k]; ok && old != got[x] {
					// Only a gap between two changes seen counts.
					if at, seen := since[k]; seen && !at.IsZero() && c.t.Sub(at) < waveGlyph-sample {
						gap := c.t.Sub(at)
						t.Fatalf("cell (%d,%d) changed after %v, want every %v", x, y, gap, waveGlyph)
					}
					changes++
					since[k] = c.t
				}
				glyph[k] = got[x]
			}
		}
	}
	if changes == 0 {
		t.Fatal("the scrambled glyphs never changed")
	}
	// The cells change at their own moments, not all on the same frame.
	moments := map[time.Time]bool{}
	for _, at := range since {
		moments[at] = true
	}
	if len(moments) < 5 {
		t.Fatalf("the glyphs changed at only %d moments", len(moments))
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
				if n := len(l.tears); n < 2 || n > 4 {
					t.Fatalf("seed %d: %d torn rows, want 2..4", seed, n)
				}
				for _, tr := range l.tears {
					if tr.k < 1 || tr.k > 3 || tr.y < 0 || tr.y >= 24 {
						t.Fatalf("seed %d: tear %+v, want 1..3 cells on one of 24 rows", seed, tr)
					}
				}
				lo, hi := 6, 14
				if e.noSignal {
					lo, hi = 3*lo, 3*hi
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
	if frac := float64(bars) / float64(frames); frac < 0.4 || frac > 0.6 {
		t.Errorf("static bar on %.2f of the burst frames, want about half", frac)
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
