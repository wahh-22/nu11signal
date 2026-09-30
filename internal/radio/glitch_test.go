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
	if frac := float64(flashes) / float64(bursts); frac < 0.1 || frac > 0.45 {
		t.Errorf("NO SIGNAL in %.2f of the bursts, want about a quarter", frac)
	}
}

func TestAlertScheduleStaysInRange(t *testing.T) {
	const stepDur = 50 * time.Millisecond
	start := time.Date(2077, 1, 1, 0, 0, 0, 0, time.UTC)
	e := effects{on: true}
	var lastEnd time.Time
	seq, alerts := uint64(0), 0
	for now := start; now.Before(start.Add(time.Hour)); now = now.Add(stepDur) {
		e = e.advance(now, 7, true)
		if e.alertSeq == seq {
			continue
		}
		seq = e.alertSeq
		alerts++
		if d := e.alertEnd.Sub(e.alertStart); d != alertShow {
			t.Errorf("alert %d shows %v, want %v", seq, d, alertShow)
		}
		from := lastEnd
		if from.IsZero() {
			from = start
		}
		if gap := e.alertStart.Sub(from); gap < alertGapMin || gap > alertGapMax+stepDur {
			t.Errorf("alert %d comes %v after the last, want %v..%v", seq, gap, alertGapMin, alertGapMax)
		}
		lastEnd = e.alertEnd
	}
	if alerts < 50 {
		t.Fatalf("only %d alerts in an hour", alerts)
	}
}

func TestPausedEffectsNeverBurst(t *testing.T) {
	start := time.Date(2077, 1, 1, 0, 0, 0, 0, time.UTC)
	e := effects{on: true}
	for now := start; now.Before(start.Add(10 * time.Minute)); now = now.Add(100 * time.Millisecond) {
		e = e.advance(now, 3, false)
		if e.bursting(now) || e.alerting(now) {
			t.Fatalf("paused effects burst or alert at %v", now.Sub(start))
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
		"fastTick": fastTick, "waveTick": waveTick, "burstTick": burstTick, "alertBlink": alertBlink,
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
	m = forceBurst(t, m, c, false)
	if got := m.tickInterval(); got != burstTick {
		t.Fatalf("burst tick = %v, want %v", got, burstTick)
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
	if !m.fx.alerting(c.t) {
		want := min(idleTick, m.fx.nextWave.Sub(c.t), m.fx.nextBurst.Sub(c.t))
		if got := m.tickInterval(); got != want {
			t.Fatalf("idle tick with effects = %v, want %v (to the next effect)", got, want)
		}
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

func TestAlertsYieldToRealStatus(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	c.t = m.fx.nextAlert
	m = tick(t, m)
	if !m.fx.alerting(c.t) {
		t.Fatal("no alert at its scheduled time")
	}
	want := alertText(m.fx.alertSeq, m.seed)
	if got := ansi.Strip(m.statusLine(80)); !strings.Contains(got, want) || !strings.Contains(got, "▲") {
		t.Fatalf("status line = %q, want the alert %q with its ▲", got, want)
	}
	// The ▲ blinks.
	c.advance(alertBlink)
	if got := ansi.Strip(m.statusLine(80)); strings.Contains(got, "▲") || !strings.Contains(got, want) {
		t.Fatalf("status line = %q, want the alert with its ▲ blinked off", got)
	}
	m.setStatus("PLAY FAILED // TIMEOUT")
	if got := ansi.Strip(m.statusLine(80)); !strings.Contains(got, "PLAY FAILED") || strings.Contains(got, want) {
		t.Fatalf("status line = %q, want the real status over the alert", got)
	}
	c.t = m.fx.alertEnd
	m = tick(t, m)
	m.status = ""
	if got := ansi.Strip(m.statusLine(80)); strings.Contains(got, want) {
		t.Fatalf("status line = %q, want the alert gone after its time", got)
	}
}

func TestAlertTextsVary(t *testing.T) {
	seen := map[string]bool{}
	for seq := uint64(1); seq <= 40; seq++ {
		s := alertText(seq, 2077)
		if s == "" || ansi.StringWidth(s) > 40 {
			t.Fatalf("alert %d = %q", seq, s)
		}
		seen[s] = true
	}
	if len(seen) < 5 {
		t.Fatalf("only %d distinct alerts in 40", len(seen))
	}
}

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

// scrambledWords counts the words of base that differ in lines, and the
// lines they are on.
func scrambledWords(base, lines []string, words []textRun) (int, map[int]bool) {
	n, rows := 0, map[int]bool{}
	for _, w := range words {
		got, want := cells(lines[w.y]), cells(base[w.y])
		for _, x := range w.xs {
			if got[x] != want[x] {
				n++
				rows[w.y] = true
				break
			}
		}
	}
	return n, rows
}

func TestWaveScramblesMostWordsAcrossTheScreen(t *testing.T) {
	for _, keys := range [][]string{nil, {"f"}, {"enter"}} {
		c := newClock()
		m := fxModel(t, c)
		m, _ = press(t, m, keys...)
		m = forceWave(t, m, c)
		for wave := range 4 {
			if wave > 0 {
				m = nextWave(t, m, c)
			}
			base, _ := m.baseLayout()
			lines, _ := m.layout()
			words := textWords(base, -1)
			wordRows := map[int]bool{}
			for _, w := range words {
				wordRows[w.y] = true
			}
			n, rows := scrambledWords(base, lines, words)
			if len(words) < 20 {
				t.Fatalf("keys %v: only %d words on the frame", keys, len(words))
			}
			if share := float64(n) / float64(len(words)); share < 0.7 || share > 0.85 {
				t.Fatalf("keys %v wave %d: %d of %d words scrambled (%.2f), want about three quarters:\n%s", keys, wave, n, len(words), share, ansi.Strip(strings.Join(lines, "\n")))
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
				if n, _ := scrambledWords(base, lines, textWords(base, -1)); n > 0 && n < initial*2/3 {
					staggered = true
				}
				c.advance(waveTick)
				m = tick(t, m)
			}
			if !staggered {
				t.Fatalf("keys %v wave %d: the words never resolved one by one", keys, wave)
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
		base, _ := m.baseLayout()
		lines, _ := m.layout()
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
	if got, want := m.tickInterval(), min(idleTick, m.fx.nextWave.Sub(c.t)); got != want && !m.fx.alerting(c.t) {
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
	for range 4 {
		c.advance(waveTick)
		m = tick(t, m)
	}
	assertGolden(t, "text_wave_80x24.golden", ansi.Strip(m.View().Content))
}
