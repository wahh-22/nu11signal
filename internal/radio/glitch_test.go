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
	if got := m.tickInterval(); got != rainTick {
		t.Fatalf("idle tick with effects = %v, want %v", got, rainTick)
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

func TestDataRainOnlyFillsFreeSpace(t *testing.T) {
	for _, keys := range [][]string{nil, {"f"}, {"enter"}} {
		c := newClock()
		m := fxModel(t, c)
		m, _ = press(t, m, keys...)
		drew := false
		for range 40 {
			c.advance(rainTick)
			if m.fx.bursting(c.t) || m.fx.alerting(c.t) {
				continue
			}
			base, _ := m.baseLayout()
			lines, _ := m.layout()
			for y := range lines {
				got, want := cells(lines[y]), cells(base[y])
				if len(got) != len(want) {
					t.Fatalf("keys %v line %d: %d cells, want %d", keys, y, len(got), len(want))
				}
				for x := range got {
					if got[x] == want[x] {
						continue
					}
					drew = true
					if want[x] != " " && want[x] != "─" {
						t.Fatalf("keys %v: rain over %q at (%d,%d):\n%s", keys, want[x], x, y, ansi.Strip(strings.Join(lines, "\n")))
					}
				}
			}
			// Readable lines stay whole: the station rows, the title and
			// the footer.
			out := ansi.Strip(strings.Join(lines, "\n"))
			for _, text := range []string{"NIGHT DRIVE", "CHIPPIN' IN", "QUIT", "SYS NOMINAL"} {
				if strings.Contains(ansi.Strip(strings.Join(base, "\n")), text) && !strings.Contains(out, text) {
					t.Fatalf("keys %v: rain hid %q:\n%s", keys, text, out)
				}
			}
			for _, y := range []int{len(lines) - 2, len(lines) - 1} {
				if lines[y] != base[y] {
					t.Fatalf("keys %v: rain on the status or hint line %d: %q", keys, y, ansi.Strip(lines[y]))
				}
			}
		}
		if !drew {
			t.Fatalf("keys %v: no data rain in 20s", keys)
		}
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

func TestBurstsSpareARealStatus(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	for _, noSignal := range []bool{false, true, false, true} {
		m = forceBurst(t, m, c, noSignal)
		m.setStatus("PLAY FAILED // HELPER GONE")
		base, _ := m.baseLayout()
		lines, _ := m.layout()
		n := len(lines)
		if !reflect.DeepEqual(lines[n-2:], base[n-2:]) {
			t.Fatalf("burst (noSignal=%v) touched the status or hint line:\n%q\n%q", noSignal, ansi.Strip(lines[n-2]), ansi.Strip(lines[n-1]))
		}
	}
}

func TestRainToleratesAShortFrame(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	base, _ := m.baseLayout()
	short := append([]string(nil), base[:len(base)/2]...)
	m.rain(short, c.now()) // must not index past the frame
}
