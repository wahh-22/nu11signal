package radio

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/config"
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// configSource is a config.Source returning fixed settings.
type configSource struct {
	cfg config.Config
	err error
}

func (s configSource) Load() (config.Config, error) { return s.cfg, s.err }

func (configSource) Save(config.Config) error { return nil }

// withConfig is a loaded model that read src at startup.
func withConfig(t *testing.T, f *playbacktest.Fake, c *clock, src config.Source) Model {
	t.Helper()
	m := New(f, Options{Now: c.now, Seed: 2077, Config: src})
	m, _ = step(t, m, run(t, m.loadConfigCmd()))
	return m
}

func TestConfigVisualizerIsAcceptedAndIgnored(t *testing.T) {
	tests := []struct {
		name       string
		src        configSource
		wantStatus string
	}{
		{"missing file", configSource{}, ""},
		{"rain", configSource{cfg: config.Config{Visualizer: "rain"}}, ""},
		// Names of the retired visualizers, or any other, say nothing.
		{"retired bars", configSource{cfg: config.Config{Visualizer: "bars"}}, ""},
		{"retired synthwave", configSource{cfg: config.Config{Visualizer: "SynthWave"}}, ""},
		{"retired random", configSource{cfg: config.Config{Visualizer: "random"}}, ""},
		{"unknown name", configSource{cfg: config.Config{Visualizer: "lasers"}}, ""},
		{"invalid file", configSource{err: errors.New("invalid character 'v'")}, "CONFIG UNREADABLE // invalid character 'v'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := withConfig(t, playbacktest.New(), newClock(), tt.src)
			if m.status != tt.wantStatus {
				t.Fatalf("status %q; want %q", m.status, tt.wantStatus)
			}
		})
	}
}

func TestNoConfigSourceLoadsNothing(t *testing.T) {
	if m := New(playbacktest.New(), Options{}); m.loadConfigCmd() != nil {
		t.Fatal("a command to load settings without a source")
	}
}

func TestTheVKeyNoLongerCyclesAnything(t *testing.T) {
	m, _, _ := playingWithLevels(t)
	before := m
	m, _ = press(t, m, "v")
	if m.status != before.status {
		t.Fatalf("v said %q", m.status)
	}
	for _, h := range playerHints {
		if h.key == "V" {
			t.Fatalf("the footer still hints %+v", h)
		}
	}
	// In SEARCH it types.
	s := searchFor(t, loaded(t, playbacktest.New(), newClock()), "da")
	if s = typeText(t, s, "v"); !strings.HasSuffix(s.input.Value(), "v") {
		t.Fatalf("input %q; want v typed", s.input.Value())
	}
}

// vizFrame is a playing frame with every band at level and a waveform.
func vizFrame(level float64, w, h int, real bool) vizInput {
	bands := make([]float64, 24)
	for i := range bands {
		bands[i] = level
	}
	in := vizInput{Bands: bands, Playing: true, Real: real, Seed: 2077, W: w, H: h}
	if real {
		in.Wave = make([]float64, 64)
		for i := range in.Wave {
			in.Wave[i] = level * math.Sin(float64(i)/64*4*math.Pi)
		}
	}
	return in
}

// stepped runs n playing frames of v at level.
func stepped(v rainViz, n int, level float64, w, h int, real bool) rainViz {
	for range n {
		v = v.Step(vizFrame(level, w, h, real))
	}
	return v
}

func TestRainRendersItsExactSize(t *testing.T) {
	sizes := []struct{ w, h int }{{1, 1}, {1, 2}, {2, 2}, {5, 3}, {27, 5}, {38, 12}, {150, 12}, {9, 7}}
	for _, sz := range sizes {
		for _, real := range []bool{true, false} {
			t.Run(fmt.Sprintf("%dx%d/real=%v", sz.w, sz.h, real), func(t *testing.T) {
				v := stepped(rainViz{}, 20, 0.8, sz.w, sz.h, real)
				for _, size := range []struct{ w, h int }{sz, {sz.w + 3, sz.h}, {max(sz.w-1, 1), sz.h + 1}} {
					// Also at a size other than the one stepped (a resize
					// between a frame and its drawing).
					rows := v.Render(size.w, size.h)
					if len(rows) != size.h {
						t.Fatalf("%dx%d: %d rows", size.w, size.h, len(rows))
					}
					for i, row := range rows {
						if got := ansi.StringWidth(row); got != size.w {
							t.Fatalf("%dx%d: row %d is %d cells: %q", size.w, size.h, i, got, ansi.Strip(row))
						}
					}
				}
			})
		}
	}
}

func TestRainBeforeAnyFrame(t *testing.T) {
	rows := rainViz{}.Render(20, 4)
	if len(rows) != 4 || filled(rows) != 0 {
		t.Fatalf("a new rain draws %d rows, %d cells lit", len(rows), filled(rows))
	}
}

// filled counts the cells of rows that are not blank.
func filled(rows []string) int {
	n := 0
	for _, row := range rows {
		for _, r := range ansi.Strip(row) {
			if r != ' ' {
				n++
			}
		}
	}
	return n
}

func TestRainFreezesWhenPaused(t *testing.T) {
	for _, real := range []bool{true, false} {
		v := stepped(rainViz{}, 15, 0.7, 30, 8, real)
		before := v.Render(30, 8)
		paused := vizInput{Bands: make([]float64, 24), W: 30, H: 8, Seed: 2077}
		for range 5 {
			v = v.Step(paused)
		}
		if after := v.Render(30, 8); !slices.Equal(before, after) {
			t.Fatalf("real=%v: the rain moved while paused:\n%s\n--\n%s", real, ansi.Strip(strings.Join(before, "\n")), ansi.Strip(strings.Join(after, "\n")))
		}
	}
}

func TestRainStepsWithoutChangingTheReceiver(t *testing.T) {
	v := stepped(rainViz{}, 10, 0.6, 30, 8, true)
	before := v.Render(30, 8)
	stepped(v, 5, 1, 30, 8, true)
	// A bass hit too, which flashes and bursts.
	hit := vizFrame(0.6, 30, 8, true)
	for b := range 6 {
		hit.Bands[b] = 1
	}
	v.Step(hit)
	if after := v.Render(30, 8); !slices.Equal(before, after) {
		t.Fatal("Step changed the rain it was called on")
	}
}

func TestLouderMusicLightsMoreRain(t *testing.T) {
	prev := -1
	for _, level := range []float64{0.1, 0.3, 0.6, 0.9} {
		lit := 0
		for _, n := range []int{25, 30, 35, 40} {
			lit += filled(stepped(rainViz{}, n, level, 40, 10, true).Render(40, 10))
		}
		if lit <= prev {
			t.Fatalf("level %.1f lit %d cells, no more than %d a notch quieter", level, lit, prev)
		}
		prev = lit
	}
}

func TestRainEnergyEmphasizesDifferences(t *testing.T) {
	if rainEnergy(0) != 0 || rainEnergy(rainFloor) != 0 || rainEnergy(rainFloor/2) != 0 {
		t.Fatal("quiet bands carry energy")
	}
	if got := rainEnergy(1); math.Abs(got-1) > 1e-9 {
		t.Fatalf("full level: energy %v; want 1", got)
	}
	prev := 0.0
	for l := rainFloor + 0.01; l <= 1; l += 0.01 {
		got := rainEnergy(l)
		if got <= prev || got > 1 {
			t.Fatalf("energy %v at level %.2f after %v", got, l, prev)
		}
		prev = got
	}
	// Steeper than the level: twice as loud is more than twice the energy.
	if lo, hi := rainEnergy(0.4), rainEnergy(0.8); hi <= 2.5*lo {
		t.Fatalf("energy %v at 0.4, %v at 0.8: the curve does not stand the loud bands out", lo, hi)
	}
}

func TestRainGlyphsAreOneCellWide(t *testing.T) {
	for _, r := range rainGlyphs {
		if w := ansi.StringWidth(string(r)); w != 1 {
			t.Errorf("%q (%U) is %d cells wide", r, r, w)
		}
	}
	if len(rainGlyphs) != 16+56 {
		t.Errorf("%d rain glyphs", len(rainGlyphs))
	}
}

func TestRainDrizzlesWithoutReadings(t *testing.T) {
	real := stepped(rainViz{}, 40, 0.9, 40, 10, true)
	drizzle := stepped(rainViz{}, 40, 0.9, 40, 10, false)
	if len(drizzle.drops) == 0 || len(drizzle.drops) >= len(real.drops) {
		t.Fatalf("drizzle has %d drops, loud readings %d", len(drizzle.drops), len(real.drops))
	}
	// Dim: no bright heads, whatever the decorative level.
	out := strings.Join(drizzle.Render(40, 10), "\n")
	for _, st := range []lipgloss.Style{stYellow, stRedBold} {
		if strings.Contains(out, inkOf(st).pre) {
			t.Fatalf("the drizzle burns in %q", inkOf(st).pre)
		}
	}
}

// The trail ink steps down the ramp from the head, whatever the order the
// inks are declared in.
func TestRainTrailStepsDownTheRamp(t *testing.T) {
	for i, k := range rainRamp[:len(rainRamp)-1] {
		if got := trailInk(k, 1, 40); got != rainRamp[i+1] {
			t.Errorf("one step down from ink %d: %d; want %d", k, got, rainRamp[i+1])
		}
	}
	// Four steps at most: a red head's trail ends dim, a yellow one's
	// muted.
	if got := trailInk(inkRed, 39, 40); got != inkDim {
		t.Errorf("far down a red trail: ink %d; want dim", got)
	}
	if got := trailInk(inkYellow, 39, 40); got != inkMuted {
		t.Errorf("far down a yellow trail: ink %d; want muted", got)
	}
}

func TestRainFollowsEachBand(t *testing.T) {
	silent := stepped(rainViz{}, 40, 0, 40, 10, true)
	if len(silent.drops) != 0 {
		t.Fatalf("%d drops in silence", len(silent.drops))
	}
	// Loud bass, silent treble: the rain falls on the left only, a bass
	// hit included.
	v := rainViz{}
	for range 40 {
		in := vizFrame(0, 40, 10, true)
		for b := range in.Bands[:len(in.Bands)/2] {
			in.Bands[b] = 0.9
		}
		v = v.Step(in)
	}
	if len(v.drops) < 5 {
		t.Fatalf("%d drops under loud bands", len(v.drops))
	}
	for _, d := range v.drops {
		if d.x >= 20 {
			t.Fatalf("a drop at column %d, under a silent band", d.x)
		}
	}
	// Louder bands fall faster and longer.
	speed := func(v rainViz) (s float64, l int) {
		for _, d := range v.drops {
			s, l = max(s, d.speed), max(l, d.length)
		}
		return s, l
	}
	slowS, slowL := speed(stepped(rainViz{}, 40, 0.3, 40, 10, true))
	fastS, fastL := speed(stepped(rainViz{}, 40, 1, 40, 10, true))
	if slowS == 0 || fastS <= 2*slowS || fastL <= slowL {
		t.Fatalf("speed %.2f / %.2f, length %d / %d at 0.3 / 1", slowS, fastS, slowL, fastL)
	}
}

func TestRainHeadsBurnWithTheEnergy(t *testing.T) {
	yellow := inkOf(stYellow).pre
	quiet := strings.Join(stepped(rainViz{}, 40, 0.3, 40, 10, true).Render(40, 10), "\n")
	loud := strings.Join(stepped(rainViz{}, 40, 1, 40, 10, true).Render(40, 10), "\n")
	if strings.Contains(quiet, yellow) || !strings.Contains(loud, yellow) {
		t.Fatalf("yellow heads: quiet %v, loud %v; want loud only", strings.Contains(quiet, yellow), strings.Contains(loud, yellow))
	}
}

// fresh counts the drops of v not in before (by seed): the ones spawned
// since.
func fresh(before, v rainViz) (n int, xs []int) {
	old := map[uint64]bool{}
	for _, d := range before.drops {
		old[d.seed] = true
	}
	for _, d := range v.drops {
		if !old[d.seed] {
			n++
			xs = append(xs, d.x)
		}
	}
	return n, xs
}

func TestABassHitBurstsTheRain(t *testing.T) {
	const w, h = 60, 10
	base := stepped(rainViz{}, 40, 0.3, w, h, true)
	calm := base.Step(vizFrame(0.3, w, h, true))
	hit := vizFrame(0.3, w, h, true)
	for b := range len(hit.Bands) / 4 {
		hit.Bands[b] = 0.95
	}
	burst := base.Step(hit)
	nCalm, _ := fresh(base, calm)
	nBurst, xs := fresh(base, burst)
	// A wave of new drops at once, across the width, not only under the
	// bass.
	if nBurst < nCalm+w/2/3 {
		t.Fatalf("a bass hit spawned %d drops, a calm frame %d", nBurst, nCalm)
	}
	if slices.Max(xs) < w/2 {
		t.Fatalf("the burst stays on the left: columns %v", xs)
	}
	// And a brief flash: brighter heads now, gone a few frames on.
	if burst.flash <= 0 {
		t.Fatal("no flash on a bass hit")
	}
	yellow := inkOf(stYellow).pre
	if !strings.Contains(strings.Join(burst.Render(w, h), "\n"), yellow) {
		t.Fatal("the hit does not brighten the heads")
	}
	later := burst
	for range 8 {
		later = later.Step(vizFrame(0.3, w, h, true))
	}
	if later.flash > 0.05 {
		t.Fatalf("the flash lingers at %.2f eight frames on", later.flash)
	}
	// A bigger hit bursts more.
	small := vizFrame(0.3, w, h, true)
	for b := range len(small.Bands) / 4 {
		small.Bands[b] = 0.55
	}
	if nSmall, _ := fresh(base, base.Step(small)); nSmall >= nBurst {
		t.Fatalf("a small hit spawned %d drops, a big one %d", nSmall, nBurst)
	}
}

func TestAHighBandHitBurstsUnderItsBand(t *testing.T) {
	const w, h = 48, 10
	base := stepped(rainViz{}, 40, 0.3, w, h, true)
	hit := vizFrame(0.3, w, h, true)
	for b := 18; b < 24; b++ {
		hit.Bands[b] = 0.95
	}
	n, xs := fresh(base, base.Step(hit))
	nCalm, _ := fresh(base, base.Step(vizFrame(0.3, w, h, true)))
	if n <= nCalm {
		t.Fatalf("a treble hit spawned %d drops, a calm frame %d", n, nCalm)
	}
	burstRight := 0
	for _, x := range xs {
		if x >= w*18/24 {
			burstRight++
		}
	}
	if burstRight < n/2 {
		t.Fatalf("a treble hit spawned on columns %v, want mostly under the treble", xs)
	}
}

func TestSteadyMusicNeverFlashes(t *testing.T) {
	v := rainViz{}
	for range 60 {
		v = v.Step(vizFrame(0.7, 40, 10, true))
		if v.flash > 0 {
			t.Fatalf("steady music flashed %.2f", v.flash)
		}
	}
}

func TestSilenceDriesTheRain(t *testing.T) {
	v := stepped(rainViz{}, 30, 0.9, 40, 10, true)
	if len(v.drops) == 0 {
		t.Fatal("no drops under loud music")
	}
	prev := len(v.drops)
	for i := 0; len(v.drops) > 0; i++ {
		if i == 60 {
			t.Fatalf("%d drops left after 60 silent frames", len(v.drops))
		}
		before := v
		v = v.Step(vizFrame(0, 40, 10, true))
		if n, _ := fresh(before, v); n > 0 || len(v.drops) > prev {
			t.Fatalf("silent frame %d spawned %d drops", i, n)
		}
		prev = len(v.drops)
	}
	if lit := filled(v.Render(40, 10)); lit != 0 {
		t.Fatalf("%d cells lit after the trails fell", lit)
	}
}

// flatFrame is a frame of a flat, heavily compressed track: every band at
// 0.5, up to 0.6 on the peak of each rainTestPeriod frames.
func flatFrame(i, w, h int) vizInput {
	in := vizFrame(0.5, w, h, true)
	if i%rainTestPeriod == 0 {
		for b := range in.Bands {
			in.Bands[b] = 0.6
		}
	}
	return in
}

// rainTestPeriod is how many frames apart the test tracks peak or beat.
const rainTestPeriod = 5

// meanSpeed is the average speed of v's drops; 0 without any.
func meanSpeed(v rainViz) float64 {
	if len(v.drops) == 0 {
		return 0
	}
	var s float64
	for _, d := range v.drops {
		s += d.speed
	}
	return s / float64(len(v.drops))
}

// A flat track, its levels swinging only between 0.5 and 0.6, still
// drives the rain: the drops falling speed up on the peaks and brake in
// the troughs, not only the new ones.
func TestFlatMusicStillMovesTheRain(t *testing.T) {
	const w, h = 48, 12
	v := rainViz{}
	var peak, trough float64
	var nPeak, nTrough int
	for i := range 80 {
		v = v.Step(flatFrame(i, w, h))
		if i < 20 {
			continue
		}
		switch i % rainTestPeriod {
		case 0:
			peak += meanSpeed(v)
			nPeak++
		case rainTestPeriod - 1:
			trough += meanSpeed(v)
			nTrough++
		}
	}
	peak, trough = peak/float64(nPeak), trough/float64(nTrough)
	if trough == 0 || peak < 1.4*trough {
		t.Fatalf("mean drop speed %.2f on the peaks, %.2f in the troughs; want at least 1.4 times", peak, trough)
	}
}

// Levels at or under rainFloor stay dry however they jitter: the gain
// that stretches a flat track never makes rain out of near-silence.
func TestJitterUnderTheFloorStaysDry(t *testing.T) {
	v := rainViz{}
	for i := range 60 {
		in := vizFrame(0, 40, 10, true)
		for b := range in.Bands {
			in.Bands[b] = rainFloor * float64((i+b)%3) / 2
		}
		v = v.Step(in)
		if len(v.drops) != 0 {
			t.Fatalf("frame %d: %d drops from levels under the floor", i, len(v.drops))
		}
	}
}

// beatFrame is frame i of a flat track with a beat: the bands drift
// slowly between 0.5 and 0.6 and every rainTestPeriod frames a beat lifts
// them 0.08, the bass more, fading over the next frames.
func beatFrame(i, w, h int) vizInput {
	in := vizFrame(0, w, h, true)
	kick := 0.08 / float64(int(1)<<(i%rainTestPeriod))
	if i%rainTestPeriod == rainTestPeriod-1 {
		kick = 0
	}
	for b := range in.Bands {
		lift := kick
		if b < len(in.Bands)/4 {
			lift *= 1.5
		}
		in.Bands[b] = 0.55 + 0.05*math.Sin(float64(i)*2*math.Pi/40) + lift
	}
	return in
}

// A beat on a flat track pulses the whole rain, on the beat and only
// then: the drops fall further and the heads burn brighter.
func TestABeatPulsesTheRain(t *testing.T) {
	const w, h = 48, 12
	v := rainViz{}
	onBeat, beats := 0, 0
	for i := range 100 {
		before := v
		v = v.Step(beatFrame(i, w, h))
		if i < 20 {
			continue
		}
		pulsed := v.pulse == 1
		if i%rainTestPeriod != 0 {
			if pulsed {
				t.Fatalf("frame %d, between beats, pulsed", i)
			}
			continue
		}
		beats++
		if !pulsed {
			continue
		}
		onBeat++
		// Every drop falls further than its speed on the pulse.
		old := map[uint64]rainDrop{}
		for _, d := range before.drops {
			old[d.seed] = d
		}
		for _, d := range v.drops {
			if o, ok := old[d.seed]; ok && d.y-o.y < d.speed+0.5 {
				t.Fatalf("frame %d: a drop fell %.2f on the pulse at speed %.2f", i, d.y-o.y, d.speed)
			}
		}
		if got := rainHeadInk(0.2, max(v.flash, rainPulseFlash*v.pulse)); got != rainHeadInk(0.2, 1) {
			t.Fatalf("frame %d: a faint head burns ink %d on the pulse", i, got)
		}
	}
	if onBeat < beats-1 {
		t.Fatalf("pulsed on %d of %d beats", onBeat, beats)
	}
	// And the pulse is brief: gone a few frames after the last beat.
	for i := range 4 {
		v = v.Step(vizFrame(0.55, w, h, true))
		if i == 3 && v.pulse > 0.1 {
			t.Fatalf("the pulse lingers at %.2f four frames on", v.pulse)
		}
	}
}

func TestSteadyOrSilentMusicNeverPulses(t *testing.T) {
	for _, level := range []float64{0, rainFloor, 0.7} {
		v := rainViz{}
		for i := range 60 {
			v = v.Step(vizFrame(level, 40, 10, true))
			if v.pulse > 0 {
				t.Fatalf("level %.2f frame %d: pulse %.2f", level, i, v.pulse)
			}
		}
	}
	// Nor does the drizzle, whatever the decorative bands do.
	v := rainViz{}
	for i := range 40 {
		in := beatFrame(i, 40, 10)
		in.Real, in.Wave = false, nil
		if v = v.Step(in); v.pulse > 0 {
			t.Fatalf("drizzle frame %d: pulse %.2f", i, v.pulse)
		}
	}
}

// vizModel is a model playing with readings at w x h after frames
// animation frames with a reading each.
func vizModel(t *testing.T, w, h, frames int, expanded bool) (Model, *playbacktest.Fake) {
	t.Helper()
	f := playbacktest.New()
	c := newClock()
	m := withConfig(t, f, c, configSource{})
	m, _ = step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	m.width, m.height, m.expanded = w, h, expanded
	for i := range frames {
		bands := make([]float64, 24)
		for b := range bands {
			bands[b] = 0.5 + 0.45*math.Sin(float64(b)/3+float64(i)/2)
		}
		wave := make([]float64, 64)
		for p := range wave {
			wave[p] = 0.6*math.Sin(float64(p)/64*6*math.Pi+float64(i)) + 0.2*math.Sin(float64(p)/64*22*math.Pi)
		}
		f.PushLevels(playback.Spectrum{Bands: bands, Wave: wave})
		c.advance(fastTick)
		m = tick(t, m)
	}
	return m, f
}

func TestRainFitsEveryLayout(t *testing.T) {
	sizes := []struct{ w, h int }{{20, 5}, {40, 10}, {59, 15}, {60, 16}, {60, 17}, {80, 18}, {80, 24}, {100, 30}, {160, 50}}
	for _, sz := range sizes {
		for _, expanded := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/expanded=%v", sz.w, sz.h, expanded), func(t *testing.T) {
				m, _ := vizModel(t, sz.w, sz.h, 6, expanded)
				lines := strings.Split(ansi.Strip(m.render()), "\n")
				if len(lines) > sz.h {
					t.Fatalf("%d lines", len(lines))
				}
				for i, l := range lines {
					if got := ansi.StringWidth(l); got > sz.w {
						t.Fatalf("line %d is %d cells: %q", i, got, l)
					}
				}
				// The panel frame survives: every NOW PLAYING row ends
				// in its border.
				if sz.w >= fullMinWidth && sz.h >= fullMinHeight {
					for i := 3; i < sz.h-3; i++ {
						if !strings.HasSuffix(lines[i], "│") {
							t.Fatalf("row %d lost its border: %q", i, lines[i])
						}
					}
				}
				// The zones do not depend on the rain.
				dry := m
				dry.rain = rainViz{}
				if _, got := m.layout(); !reflect.DeepEqual(got, func() zones { _, z := dry.layout(); return z }()) {
					t.Fatalf("zones differ from a dry rain's")
				}
			})
		}
	}
}

// soaked is a rain with a drop down every column of a w x h area.
func soaked(w, h int) rainViz {
	var v rainViz
	for x := range w {
		v.drops = append(v.drops, rainDrop{x: x, y: float64(h - 1), speed: 1, length: h, seed: uint64(x + 1)})
	}
	return v
}

func TestVizSizeMatchesTheDrawnArea(t *testing.T) {
	for _, h := range []int{16, 17, 18, 19, 20, 24, 30, 60} {
		for _, expanded := range []bool{false, true} {
			m, _ := vizModel(t, 80, h, 0, expanded)
			w, rows := m.vizSize()
			m.rain = rainViz{}
			dry := strings.Split(ansi.Strip(m.render()), "\n")
			m.rain = soaked(w, rows)
			wet := strings.Split(ansi.Strip(m.render()), "\n")
			diff := 0
			for y := range dry {
				d, s := []rune(dry[y]), []rune(wet[y])
				for x := range min(len(d), len(s)) {
					if d[x] != s[x] {
						diff++
					}
				}
			}
			if diff != w*rows {
				t.Fatalf("h=%d expanded=%v: %d cells drawn; vizSize %dx%d", h, expanded, diff, w, rows)
			}
		}
	}
}

func TestPausedRainLetsTheTickIdle(t *testing.T) {
	m, _ := vizModel(t, 80, 24, 10, false)
	if !m.tickFast {
		t.Fatal("not animating while playing")
	}
	paused := playing(90*time.Second, 225*time.Second)
	paused.Status = playback.StatusPaused
	m, _ = step(t, m, stateMsg{state: paused})
	for i := 0; m.tickFast; i++ {
		if i == 20 {
			t.Fatal("still ticking fast 20 frames after the pause")
		}
		m = tick(t, m)
	}
	if got := m.tickInterval(); got != idleTick {
		t.Fatalf("tick %v; want %v", got, idleTick)
	}
}

func TestRainGolden80x24(t *testing.T) {
	m, _ := vizModel(t, 80, 24, 14, false)
	assertGolden(t, "viz_rain_80x24.golden", ansi.Strip(m.View().Content))
}

// barsPalette are the escape codes the bars colored with, and the theme's
// dim reds the rain's trails use, in the theme applied.
func barsPalette() map[string]bool {
	ok := map[string]bool{}
	for _, st := range []lipgloss.Style{stRed, stRedBold, stYellow, stMuted, stDim} {
		k := inkOf(st)
		ok[k.pre], ok[k.post] = true, true
	}
	return ok
}

var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

func TestRainUsesTheBarsPalette(t *testing.T) {
	palette := barsPalette()
	bright := []string{inkOf(stYellow).pre, inkOf(stRedBold).pre, inkOf(stRed).pre}
	for _, real := range []bool{true, false} {
		t.Run(fmt.Sprintf("real=%v", real), func(t *testing.T) {
			var out strings.Builder
			for _, level := range []float64{0.2, 0.6, 1} {
				v := stepped(rainViz{}, 30, level, 40, 12, real)
				out.WriteString(strings.Join(v.Render(40, 12), "\n"))
			}
			for _, seq := range sgrPattern.FindAllString(out.String(), -1) {
				if !palette[seq] {
					t.Fatalf("escape %q is not in the bars' palette", seq)
				}
			}
			if !real {
				return
			}
			for _, seq := range bright {
				if !strings.Contains(out.String(), seq) {
					t.Errorf("never draws the bars' %q", seq)
				}
			}
		})
	}
}

func TestRainFillsTheRoomUnderTheControls(t *testing.T) {
	// However tall the panel, the rain takes every row under the controls.
	for _, h := range []int{24, 40, 60} {
		for _, expanded := range []bool{false, true} {
			m, _ := vizModel(t, 120, h, 0, expanded)
			ih := h - 6
			used := nowPlayingControlRows(m.playerPanelWidth()-2, ih)
			if _, rows := m.vizSize(); rows != ih-used {
				t.Fatalf("h=%d expanded=%v: rain %d rows; want the %d left under the controls", h, expanded, rows, ih-used)
			}
		}
	}
}
