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
			m := New(playbacktest.New(), Options{})
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
	f.PushLevels(bands)
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
	f.PushLevels([]float64{1})
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
	f.PushLevels([]float64{0.5})
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
	f.PushLevels([]float64{1})
	m = tick(t, m)
	paused := playing(time.Second, time.Minute)
	paused.Status = playback.StatusPaused
	m, _ = step(t, m, stateMsg{state: paused})
	f.PushLevels([]float64{1})
	before := m.bars
	m = tick(t, m)
	if want := before.step(false, m.seed, m.frame); m.bars != want {
		t.Fatalf("bars = %v; want them decaying %v", m.bars, want)
	}
}

func TestEQIsDecorativeWithoutALevelSource(t *testing.T) {
	c := newClock()
	m := New(noLevels{playbacktest.New()}, Options{Now: c.now, Seed: 2077})
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
	f.PushLevels([]float64{1})
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
