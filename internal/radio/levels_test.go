package radio

import (
	"math"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

func TestResampleLevels(t *testing.T) {
	tests := []struct {
		name  string
		bands []float64
		n     int
		want  []float64
	}{
		{"same count", []float64{0.1, 0.2, 0.3}, 3, []float64{0.1, 0.2, 0.3}},
		{"halved averages pairs", []float64{0, 1, 0.5, 0.5}, 2, []float64{0.5, 0.5}},
		{"doubled repeats each band", []float64{0.2, 0.8}, 4, []float64{0.2, 0.2, 0.8, 0.8}},
		// Three bands on two bars: each bar covers one and a half bands.
		{"uneven weighs by overlap", []float64{0.3, 0.6, 0.9}, 2, []float64{0.4, 0.8}},
		{"no bars", []float64{0.5}, 0, []float64{}},
		{"no bands", nil, 3, []float64{0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resampleLevels(tt.bands, tt.n)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v; want %v", got, tt.want)
			}
			for i := range got {
				if math.Abs(got[i]-tt.want[i]) > 1e-9 {
					t.Fatalf("got %v; want %v", got, tt.want)
				}
			}
		})
	}
}

func TestEQFollowSetsTheShownBarsAndClearsTheRest(t *testing.T) {
	var e eq
	for i := range e {
		e[i] = 0.9
	}
	e = e.follow([]float64{0.2, 0.8}, 4)
	if got := e[:5]; !slices.Equal(got, []float64{0.2, 0.2, 0.8, 0.8, 0}) {
		t.Fatalf("bars = %v", got)
	}
	if e.total() != 2 {
		t.Fatalf("bars past the shown ones not cleared: total %v", e.total())
	}
}

func TestEQBarCountFollowsThePanel(t *testing.T) {
	tests := []struct {
		name     string
		w, h     int
		expanded bool
		want     int
	}{
		// NOW PLAYING is 31 cells wide beside the list: 27 inside its margins.
		{"beside the list", 80, 24, false, 14},
		{"wide beside the list", 160, 40, false, 42},
		{"expanded", 80, 24, true, 38},
		{"expanded wider than the bands", 200, 40, true, eqBands},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(playbacktest.New(), Options{SkipBoot: true})
			m.width, m.height, m.expanded = tt.w, tt.h, tt.expanded
			if got := m.eqBarCount(); got != tt.want {
				t.Fatalf("eqBarCount() = %d; want %d", got, tt.want)
			}
		})
	}
}

// playingWithLevels is a loaded model that plays, at 80x24 (14 bars).
func playingWithLevels(t *testing.T) (Model, *playbacktest.Fake, *clock) {
	t.Helper()
	f := playbacktest.New()
	c := newClock()
	m := loaded(t, f, c)
	m, _ = step(t, m, stateMsg{state: playing(time.Second, time.Minute)})
	return m, f, c
}

func TestEQDrawsFreshLevels(t *testing.T) {
	m, f, _ := playingWithLevels(t)
	bands := make([]float64, 24)
	for i := range bands {
		bands[i] = float64(i) / 23
	}
	f.PushLevels(playback.Spectrum{Bands: bands})
	m = tick(t, m)

	want := resampleLevels(bands, 14)
	if got := m.bars[:14]; !slices.Equal(got, want) {
		t.Fatalf("bars = %v; want %v", got, want)
	}
	if rest := m.bars[14:]; slices.ContainsFunc(rest, func(v float64) bool { return v != 0 }) {
		t.Fatalf("hidden bars not cleared: %v", rest)
	}
}

func TestEQKeepsTheLatestLevelsWhileFresh(t *testing.T) {
	m, f, c := playingWithLevels(t)
	f.PushLevels(playback.Spectrum{Bands: []float64{1}})
	m = tick(t, m)
	// No new reading for a moment (a reading is late): the bars hold it.
	c.advance(400 * time.Millisecond)
	m = tick(t, m)
	if m.bars[0] != 1 || m.bars[13] != 1 {
		t.Fatalf("bars = %v; want the held reading", m.bars[:14])
	}
}

func TestEQFallsBackToDecorativeWhenLevelsStop(t *testing.T) {
	m, f, c := playingWithLevels(t)
	f.PushLevels(playback.Spectrum{Bands: []float64{0.5}})
	m = tick(t, m)
	c.advance(levelsFresh)
	before := m.bars
	m = tick(t, m)
	if want := before.step(true, m.seed, m.frame); m.bars != want {
		t.Fatalf("bars = %v; want the decorative step %v", m.bars, want)
	}
}

func TestEQFallsWhenPausedEvenWithFreshLevels(t *testing.T) {
	m, f, _ := playingWithLevels(t)
	f.PushLevels(playback.Spectrum{Bands: []float64{1}})
	m = tick(t, m)
	paused := playing(time.Second, time.Minute)
	paused.Status = playback.StatusPaused
	m, _ = step(t, m, stateMsg{state: paused})
	f.PushLevels(playback.Spectrum{Bands: []float64{1}})
	before := m.bars
	m = tick(t, m)
	if want := before.step(false, m.seed, m.frame); m.bars != want {
		t.Fatalf("bars = %v; want them decaying %v", m.bars, want)
	}
}

func TestEQIsDecorativeWithoutALevelSource(t *testing.T) {
	c := newClock()
	m := New(noLevels{playbacktest.New()}, Options{SkipBoot: true, Now: c.now, Seed: 2077})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = step(t, m, stateMsg{state: playing(time.Second, time.Minute)})
	before := m.bars
	m = tick(t, m)
	if want := before.step(true, m.seed, m.frame); m.bars != want {
		t.Fatalf("bars = %v; want the decorative step %v", m.bars, want)
	}
}

func TestLevelsClosedFallsBackToDecorative(t *testing.T) {
	m, f, _ := playingWithLevels(t)
	f.PushLevels(playback.Spectrum{Bands: []float64{1}})
	m = tick(t, m)
	_ = f.Close()
	m = tick(t, m) // the closed channel is dropped, the reading kept
	if m.levels != nil {
		t.Fatal("closed levels channel still polled")
	}
}

// noLevels hides the Fake's Levels: a player that cannot measure.
type noLevels struct{ *playbacktest.Fake }

func (noLevels) Levels() {}

// appPlaying is a loaded model at 80x24 whose player measures what it
// plays (app volume mode), just told it plays.
func appPlaying(t *testing.T) (Model, *playbacktest.Fake, *clock) {
	t.Helper()
	f := playbacktest.New()
	c := newClock()
	m := loaded(t, f, c)
	m, _ = step(t, m, stateMsg{state: appState(playback.StatusPlaying)})
	return m, f, c
}

func appState(status playback.Status) playback.State {
	s := playing(time.Second, time.Minute)
	s.Status, s.VolumeMode = status, playback.VolumeApp
	return s
}

// allBars is a reading of v on every band.
func allBars(v float64) []float64 { return []float64{v, v, v, v} }

func TestEQWaitsForLevelsInAppMode(t *testing.T) {
	m, f, c := appPlaying(t)
	// The tap attaches: no reading yet, and no decorative bars either.
	for elapsed := time.Duration(0); elapsed < eqWaitLevels; elapsed += fastTick {
		m = tick(t, m)
		if !m.bars.flat() {
			t.Fatalf("decorative bars %v after %v without a reading, want the bars still", m.bars[:14], elapsed)
		}
		c.advance(fastTick)
	}
	c.t = m.playSince.Add(eqWaitLevels - time.Millisecond)
	f.PushLevels(playback.Spectrum{Bands: allBars(0.6)})
	m = tick(t, m)
	if want := (eq{}).follow(allBars(0.6), 14); m.bars != want {
		t.Fatalf("bars = %v; want the first reading", m.bars[:14])
	}
}

func TestEQHoldsItsHeightsWhileWaitingForLevels(t *testing.T) {
	m, _, _ := appPlaying(t)
	m.bars[0], m.bars[3] = 0.4, 0.2
	before := m.bars
	m = tick(t, m)
	if m.bars != before {
		t.Fatalf("bars = %v; want them held at %v", m.bars[:14], before[:14])
	}
}

func TestEQTurnsDecorativeWithoutLevelsInAppMode(t *testing.T) {
	m, _, c := appPlaying(t)
	m = tick(t, m)
	c.t = m.playSince.Add(eqWaitLevels)
	before := m.bars
	m = tick(t, m)
	if want := before.step(true, m.seed, m.frame); m.bars != want || m.bars.flat() {
		t.Fatalf("bars = %v; want the decorative step %v", m.bars[:14], want[:14])
	}
}

func TestEQHandsOverSmoothlyFromDecorativeToLevels(t *testing.T) {
	m, f, c := appPlaying(t)
	c.t = m.playSince.Add(eqWaitLevels)
	for range 10 {
		m = tick(t, m)
		c.advance(fastTick)
	}
	target := (eq{}).follow(allBars(1), 14)
	before := m.bars
	f.PushLevels(playback.Spectrum{Bands: allBars(1)})
	m = tick(t, m)
	for i := range 14 {
		if m.bars[i] <= before[i] || m.bars[i] >= 1 {
			t.Fatalf("bar %d jumped from %.2f to %.2f, want it between them and the reading 1", i, before[i], m.bars[i])
		}
	}
	for range 20 {
		c.advance(fastTick)
		f.PushLevels(playback.Spectrum{Bands: allBars(1)})
		m = tick(t, m)
	}
	if m.bars != target {
		t.Fatalf("bars = %v; want the reading once handed over", m.bars[:14])
	}
}

func TestEQHandsOverSmoothlyFromLevelsToDecorative(t *testing.T) {
	m, f, c := appPlaying(t)
	f.PushLevels(playback.Spectrum{Bands: allBars(1)})
	m = tick(t, m)
	c.advance(levelsFresh)
	before := m.bars
	m = tick(t, m)
	if want := before.step(true, m.seed, m.frame); m.bars != want {
		t.Fatalf("bars = %v; want the decorative step from the reading", m.bars[:14])
	}
	for i := range 14 {
		if m.bars[i] < 0.4 {
			t.Fatalf("bar %d dropped from 1 to %.2f on the handover", i, m.bars[i])
		}
	}
}

func TestEQIgnoresAPrePauseReadingOnResume(t *testing.T) {
	for _, polled := range []bool{false, true} {
		m, f, c := appPlaying(t)
		f.PushLevels(playback.Spectrum{Bands: allBars(1)})
		m = tick(t, m)
		m, _ = step(t, m, stateMsg{state: appState(playback.StatusPaused)})
		for range 3 {
			c.advance(fastTick)
			m = tick(t, m)
		}
		// The last reading the helper sent before it stopped.
		f.PushLevels(playback.Spectrum{Bands: allBars(1)})
		if polled {
			m = tick(t, m) // taken while paused
		}
		c.advance(fastTick)
		m, _ = step(t, m, stateMsg{state: appState(playback.StatusPlaying)})
		before := m.bars
		m = tick(t, m)
		if m.bars != before {
			t.Fatalf("polled=%v: bars = %v on resume; want them held at %v, not the pre-pause reading", polled, m.bars[:14], before[:14])
		}
	}
}

func TestEQIgnoresEmptyLevels(t *testing.T) {
	t.Run("app mode keeps the last reading", func(t *testing.T) {
		m, f, _ := appPlaying(t)
		f.PushLevels(playback.Spectrum{Bands: allBars(0.8)})
		m = tick(t, m)
		f.PushLevels(playback.Spectrum{Bands: []float64{}})
		m = tick(t, m)
		if want := (eq{}).follow(allBars(0.8), 14); m.bars != want {
			t.Fatalf("bars = %v; want the last reading kept", m.bars[:14])
		}
	})
	t.Run("no reading stays decorative", func(t *testing.T) {
		m, f, _ := playingWithLevels(t)
		f.PushLevels(playback.Spectrum{Bands: nil})
		before := m.bars
		m = tick(t, m)
		if want := before.step(true, m.seed, m.frame); m.bars != want {
			t.Fatalf("bars = %v; want the decorative step", m.bars[:14])
		}
	})
}

func TestLevelsKeepTheWaveform(t *testing.T) {
	m, f, _ := playingWithLevels(t)
	wave := []float64{0.5, -0.5, 1, -1}
	f.PushLevels(playback.Spectrum{Bands: []float64{0.2}, Wave: wave})
	m = tick(t, m)
	got, ok := m.liveSpectrum()
	if !ok || !slices.Equal(got.Wave, wave) {
		t.Fatalf("liveSpectrum() = %+v, %v; want the waveform %v", got, ok, wave)
	}
}
