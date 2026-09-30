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

func TestParseVisualizer(t *testing.T) {
	tests := []struct {
		name   string
		want   vizMode
		wantOK bool
	}{
		{"", vizMode{kind: vizBars}, true},
		{"bars", vizMode{kind: vizBars}, true},
		{"oscilloscope", vizMode{kind: vizScope}, true},
		{"waterfall", vizMode{kind: vizBars}, false}, // retired
		{" RAIN ", vizMode{kind: vizRain}, true},
		{"synthWave", vizMode{kind: vizSynthwave}, true},
		{"Random", vizMode{random: true}, true},
		{"lasers", vizMode{kind: vizBars}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseVisualizer(tt.name)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("parseVisualizer(%q) = %+v, %v; want %+v, %v", tt.name, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRandomVisualizerIsStablePerSongAndNeverRepeats(t *testing.T) {
	seen := map[vizKind]bool{}
	for i := range 200 {
		song := fmt.Sprintf("song-%d", i)
		for prev := vizNone; prev < vizCount; prev++ {
			got := randomVisualizer(song, prev)
			if got < 0 || got >= vizCount {
				t.Fatalf("randomVisualizer(%q, %v) = %d, not a visualizer", song, prev, got)
			}
			if got == prev {
				t.Fatalf("randomVisualizer(%q, %v) repeated the previous one", song, prev)
			}
			if again := randomVisualizer(song, prev); again != got {
				t.Fatalf("randomVisualizer(%q, %v) = %v, then %v", song, prev, got, again)
			}
			seen[got] = true
		}
	}
	if len(seen) != int(vizCount) {
		t.Fatalf("200 songs picked only %v", seen)
	}
}

// configSource is a config.Source returning fixed settings.
type configSource struct {
	cfg config.Config
	err error
}

func (s configSource) Load() (config.Config, error) { return s.cfg, s.err }

// withConfig is a loaded model that read src at startup.
func withConfig(t *testing.T, f *playbacktest.Fake, c *clock, src config.Source) Model {
	t.Helper()
	m := New(f, Options{Now: c.now, Seed: 2077, Config: src})
	m, _ = step(t, m, run(t, m.loadConfigCmd()))
	return m
}

func TestConfigSelectsTheVisualizer(t *testing.T) {
	tests := []struct {
		name       string
		src        configSource
		want       vizKind
		wantRandom bool
		wantStatus string
	}{
		{"missing file", configSource{}, vizBars, false, ""},
		{"a visualizer", configSource{cfg: config.Config{Visualizer: "rain"}}, vizRain, false, ""},
		{"any case", configSource{cfg: config.Config{Visualizer: "SynthWave"}}, vizSynthwave, false, ""},
		{"random", configSource{cfg: config.Config{Visualizer: "random"}}, randomVisualizer("", vizNone), true, ""},
		{"unknown name", configSource{cfg: config.Config{Visualizer: "lasers"}}, vizBars, false, `UNKNOWN VISUALIZER "lasers" // BARS`},
		{"retired waterfall", configSource{cfg: config.Config{Visualizer: "waterfall"}}, vizBars, false, `UNKNOWN VISUALIZER "waterfall" // BARS`},
		{"invalid file", configSource{err: errors.New("invalid character 'v'")}, vizBars, false, "CONFIG UNREADABLE // VISUALIZER BARS // invalid character 'v'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := withConfig(t, playbacktest.New(), newClock(), tt.src)
			if m.vizKind != tt.want || m.viz.Name() != vizNames[tt.want] || m.vizMode.random != tt.wantRandom {
				t.Fatalf("visualizer %v (%s, random %v); want %v (random %v)", m.vizKind, m.viz.Name(), m.vizMode.random, tt.want, tt.wantRandom)
			}
			if m.status != tt.wantStatus {
				t.Fatalf("status %q; want %q", m.status, tt.wantStatus)
			}
		})
	}
}

func TestNoConfigSourceKeepsTheBars(t *testing.T) {
	m := New(playbacktest.New(), Options{})
	if m.loadConfigCmd() != nil || m.vizKind != vizBars {
		t.Fatalf("without a source: command %v, visualizer %v", m.loadConfigCmd() != nil, m.vizKind)
	}
}

func song(id string) playback.State {
	s := playing(time.Second, time.Minute)
	s.SongID, s.Title = id, "Title "+id
	return s
}

func TestRandomModePicksAVisualizerPerSong(t *testing.T) {
	m := withConfig(t, playbacktest.New(), newClock(), configSource{cfg: config.Config{Visualizer: "random"}})
	prev := m.vizKind
	for _, id := range []string{"s1", "s2", "s3", "s4"} {
		m, _ = step(t, m, stateMsg{state: song(id)})
		want := randomVisualizer(id, prev)
		if m.vizKind != want {
			t.Fatalf("song %s: visualizer %v; want %v", id, m.vizKind, want)
		}
		// A state of the same song keeps it.
		m, _ = step(t, m, stateMsg{state: song(id)})
		if m.vizKind != want {
			t.Fatalf("song %s again: visualizer %v; want %v kept", id, m.vizKind, want)
		}
		prev = want
	}
}

func TestAFixedVisualizerStaysAcrossSongs(t *testing.T) {
	m := withConfig(t, playbacktest.New(), newClock(), configSource{cfg: config.Config{Visualizer: "rain"}})
	for _, id := range []string{"s1", "s2"} {
		m, _ = step(t, m, stateMsg{state: song(id)})
		if m.vizKind != vizRain {
			t.Fatalf("song %s: visualizer %v; want rain", id, m.vizKind)
		}
	}
}

func TestTheVisualizerKeyCycles(t *testing.T) {
	m, _, _ := playingWithLevels(t)
	for _, want := range []vizKind{vizScope, vizRain, vizSynthwave, vizBars} {
		m, _ = press(t, m, "v")
		if m.vizKind != want {
			t.Fatalf("visualizer %v; want %v", m.vizKind, want)
		}
		if wantStatus := "VISUALIZER // " + strings.ToUpper(vizNames[want]); m.status != wantStatus {
			t.Fatalf("status %q; want %q", m.status, wantStatus)
		}
	}
	// On the player too.
	m, _ = press(t, m, "right", "v")
	if m.vizKind != vizScope || m.focus != areaPlayer {
		t.Fatalf("on the player: visualizer %v, focus %v", m.vizKind, m.focus)
	}
}

func TestTheVisualizerKeyTypesInSearch(t *testing.T) {
	m := searchFor(t, loaded(t, playbacktest.New(), newClock()), "da")
	m = typeText(t, m, "v")
	if m.vizKind != vizBars || !strings.HasSuffix(m.input.Value(), "v") {
		t.Fatalf("visualizer %v, input %q; want v typed", m.vizKind, m.input.Value())
	}
}

func TestInRandomModeTheKeyCyclesAndTheNextSongPicksAgain(t *testing.T) {
	m := withConfig(t, playbacktest.New(), newClock(), configSource{cfg: config.Config{Visualizer: "random"}})
	m.auth = authOK
	m, _ = step(t, m, stateMsg{state: song("s1")})
	picked := m.vizKind
	m, _ = press(t, m, "v")
	cycled := m.vizKind
	if cycled != (picked+1)%vizCount {
		t.Fatalf("v from %v gave %v", picked, cycled)
	}
	m, _ = step(t, m, stateMsg{state: song("s2")})
	if want := randomVisualizer("s2", cycled); m.vizKind != want || !m.vizMode.random {
		t.Fatalf("next song: visualizer %v (random %v); want %v", m.vizKind, m.vizMode.random, want)
	}
}

// vizFrame is a playing frame with every band at level and a waveform.
func vizFrame(level float64, frame uint64, w, h int, real bool) vizInput {
	bands := make([]float64, 24)
	for i := range bands {
		bands[i] = level
	}
	in := vizInput{Bands: bands, Playing: true, Real: real, Frame: frame, Seed: 2077, W: w, H: h}
	if real {
		in.Wave = make([]float64, 64)
		for i := range in.Wave {
			in.Wave[i] = level * math.Sin(float64(i)/64*4*math.Pi)
		}
	}
	return in
}

// stepped runs n playing frames of v at level.
func stepped(v visualizer, n int, level float64, w, h int, real bool) visualizer {
	for i := range n {
		v = v.Step(vizFrame(level, uint64(i+1), w, h, real))
	}
	return v
}

func TestVisualizersRenderTheirExactSize(t *testing.T) {
	sizes := []struct{ w, h int }{{1, 1}, {1, 2}, {2, 2}, {5, 3}, {27, 5}, {38, 12}, {150, 12}, {9, 7}}
	for k := range vizCount {
		for _, sz := range sizes {
			for _, real := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%dx%d/real=%v", vizNames[k], sz.w, sz.h, real), func(t *testing.T) {
					v := stepped(newVisualizer(k), 20, 0.8, sz.w, sz.h, real)
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
}

func TestVisualizersBeforeAnyFrame(t *testing.T) {
	for k := range vizCount {
		v := newVisualizer(k)
		if !v.Idle() {
			t.Errorf("%s: a new visualizer is not idle", v.Name())
		}
		rows := v.Render(20, 4)
		if len(rows) != 4 {
			t.Errorf("%s: %d rows", v.Name(), len(rows))
		}
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

func TestVisualizersFreezeWhenPaused(t *testing.T) {
	for k := range vizCount {
		t.Run(vizNames[k], func(t *testing.T) {
			v := stepped(newVisualizer(k), 15, 0.7, 30, 8, true)
			paused := vizInput{Bands: make([]float64, 24), W: 30, H: 8, Seed: 2077}
			// Paused, a visualizer settles within a second of frames.
			for i := 0; !v.Idle(); i++ {
				if i == 10 {
					t.Fatal("still not idle after 10 paused frames")
				}
				paused.Frame++
				v = v.Step(paused)
			}
			before := v.Render(30, 8)
			for range 5 {
				paused.Frame++
				v = v.Step(paused)
			}
			if after := v.Render(30, 8); !slices.Equal(before, after) {
				t.Fatalf("an idle visualizer moved while paused:\n%s\n--\n%s", ansi.Strip(strings.Join(before, "\n")), ansi.Strip(strings.Join(after, "\n")))
			}
		})
	}
}

func TestVisualizersStepWithoutChangingTheReceiver(t *testing.T) {
	for k := range vizCount {
		v := stepped(newVisualizer(k), 10, 0.6, 30, 8, true)
		before := v.Render(30, 8)
		stepped(v, 5, 1, 30, 8, true)
		if after := v.Render(30, 8); !slices.Equal(before, after) {
			t.Errorf("%s: Step changed the visualizer it was called on", v.Name())
		}
	}
}

func TestLouderMusicFillsMore(t *testing.T) {
	for k := range vizCount {
		if k == vizScope {
			continue // its trace is one line at any level: see TestScopeFollowsTheWave
		}
		t.Run(vizNames[k], func(t *testing.T) {
			quiet := filled(stepped(newVisualizer(k), 30, 0.05, 40, 10, true).Render(40, 10))
			loud := filled(stepped(newVisualizer(k), 30, 0.9, 40, 10, true).Render(40, 10))
			if loud <= quiet {
				t.Fatalf("loud filled %d cells, quiet %d", loud, quiet)
			}
		})
	}
}

func TestScopeFollowsTheWave(t *testing.T) {
	flat := func(level float64) vizInput {
		wave := make([]float64, 64)
		for i := range wave {
			wave[i] = level
		}
		return vizInput{Bands: []float64{0.5}, Wave: wave, Playing: true, Real: true, W: 10, H: 3}
	}
	// Full scale up: the top dot row of the top cells; down: the bottom
	// dot row of the bottom cells.
	top := scopeViz{}.Step(flat(1)).Render(10, 3)
	bottom := scopeViz{}.Step(flat(-1)).Render(10, 3)
	topRow := strings.Repeat(string(brailleRune(0x01|0x08)), 10)
	bottomRow := strings.Repeat(string(brailleRune(0x40|0x80)), 10)
	if got := ansi.Strip(top[0]); got != topRow {
		t.Fatalf("wave at +1: top row %q; want %q", got, topRow)
	}
	if got := ansi.Strip(bottom[2]); got != bottomRow {
		t.Fatalf("wave at -1: bottom row %q; want %q", got, bottomRow)
	}
	// The axis runs through the middle: dot row 6 of 12 (the third of
	// the middle cell) in dim dots.
	axis := strings.Repeat(string(brailleRune(0x04|0x20)), 10)
	if got := ansi.Strip(top[1]); got != axis {
		t.Fatalf("middle row %q; want the axis %q", got, axis)
	}
	// Silence traces the axis itself.
	silent := scopeViz{}.Step(flat(0)).Render(10, 3)
	if got := ansi.Strip(silent[1]); got != axis || filled(silent) != 10 {
		t.Fatalf("silence: %q", ansi.Strip(strings.Join(silent, "\n")))
	}
}

func TestScopeDrawsASyntheticWaveWithoutOne(t *testing.T) {
	v := stepped(newVisualizer(vizScope), 3, 0.8, 20, 4, false)
	rows := v.Render(20, 4)
	// A wave, not the flat axis: dots on more than one row.
	busy := 0
	for _, row := range rows {
		if strings.TrimFunc(ansi.Strip(row), func(r rune) bool { return r == ' ' || r == brailleRune(0x01|0x08) }) != "" {
			busy++
		}
	}
	if busy < 3 {
		t.Fatalf("synthetic wave spans %d rows:\n%s", busy, ansi.Strip(strings.Join(rows, "\n")))
	}
}

func TestBrailleDots(t *testing.T) {
	tests := []struct {
		dots [][2]int
		want rune
	}{
		{nil, '⠀'},
		{[][2]int{{0, 0}}, '⠁'},
		{[][2]int{{0, 1}}, '⠂'},
		{[][2]int{{0, 2}}, '⠄'},
		{[][2]int{{1, 0}}, '⠈'},
		{[][2]int{{1, 2}}, '⠠'},
		{[][2]int{{0, 3}}, '⡀'},
		{[][2]int{{1, 3}}, '⢀'},
		{[][2]int{{0, 0}, {1, 1}, {0, 2}, {1, 3}}, '⢕'},
	}
	for _, tt := range tests {
		g := newBraille(1, 1)
		for _, d := range tt.dots {
			g.set(d[0], d[1])
		}
		g.set(2, 0) // outside: ignored
		g.set(0, 4)
		if got := brailleRune(g.bits(0, 0)); got != tt.want {
			t.Errorf("dots %v: %q; want %q", tt.dots, got, tt.want)
		}
	}
	full := newBraille(1, 1)
	for x := range 2 {
		for y := range 4 {
			full.set(x, y)
		}
	}
	if got := brailleRune(full.bits(0, 0)); got != '⣿' {
		t.Errorf("all dots: %q; want ⣿", got)
	}
	// The second cell of a row takes dot columns 2 and 3.
	two := newBraille(2, 1)
	two.set(3, 0)
	if two.bits(0, 0) != 0 || brailleRune(two.bits(1, 0)) != '⠈' {
		t.Errorf("dot (3,0): cells %x %x", two.bits(0, 0), two.bits(1, 0))
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
	real := stepped(newVisualizer(vizRain), 40, 0.9, 40, 10, true).(rainViz)
	drizzle := stepped(newVisualizer(vizRain), 40, 0.9, 40, 10, false).(rainViz)
	if len(drizzle.drops) == 0 || len(drizzle.drops) >= len(real.drops) {
		t.Fatalf("drizzle has %d drops, loud readings %d", len(drizzle.drops), len(real.drops))
	}
}

func TestSynthwaveGridScrollsWithTheMusic(t *testing.T) {
	quiet := synthViz{}.Step(vizFrame(0, 1, 40, 10, true)).(synthViz)
	loud := synthViz{}.Step(vizFrame(1, 1, 40, 10, true)).(synthViz)
	if quiet.phase <= 0 || loud.phase <= quiet.phase {
		t.Fatalf("grid moved %v in silence and %v at full energy", quiet.phase, loud.phase)
	}
	paused := loud.Step(vizInput{Bands: make([]float64, 24), W: 40, H: 10}).(synthViz)
	if paused.phase != loud.phase {
		t.Fatalf("grid moved while paused: %v to %v", loud.phase, paused.phase)
	}
}

func TestBarsVisualizerMatchesTheEQ(t *testing.T) {
	var e eq
	for i := range e {
		e[i] = float64(i%7) / 6
	}
	in := vizInput{Bands: e[:14], Playing: true, W: 27, H: 5}
	got := barsViz{}.Step(in).Render(27, 5)
	want := e.render(27, 5)
	for i := range want {
		if ansi.Strip(got[i]) != want[i] {
			t.Fatalf("row %d: %q; want %q", i, ansi.Strip(got[i]), want[i])
		}
	}
}

// vizModel is a model playing with readings at w x h, showing kind,
// after frames animation frames with a reading each.
func vizModel(t *testing.T, kind vizKind, w, h, frames int, expanded bool) (Model, *playbacktest.Fake) {
	t.Helper()
	f := playbacktest.New()
	c := newClock()
	m := withConfig(t, f, c, configSource{cfg: config.Config{Visualizer: vizNames[kind]}})
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

func TestVisualizersFitEveryLayout(t *testing.T) {
	sizes := []struct{ w, h int }{{20, 5}, {40, 10}, {59, 15}, {60, 16}, {60, 17}, {80, 18}, {80, 24}, {100, 30}, {160, 50}}
	for k := range vizCount {
		for _, sz := range sizes {
			for _, expanded := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%dx%d/expanded=%v", vizNames[k], sz.w, sz.h, expanded), func(t *testing.T) {
					m, _ := vizModel(t, k, sz.w, sz.h, 6, expanded)
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
					// The zones do not depend on the visualizer.
					bars, _ := vizModel(t, vizBars, sz.w, sz.h, 6, expanded)
					if _, got := m.layout(); !reflect.DeepEqual(got, func() zones { _, z := bars.layout(); return z }()) {
						t.Fatalf("zones differ from the bars'")
					}
				})
			}
		}
	}
}

func TestVizSizeMatchesTheDrawnArea(t *testing.T) {
	for _, h := range []int{16, 17, 18, 19, 20, 24, 30, 60} {
		for _, expanded := range []bool{false, true} {
			m, _ := vizModel(t, vizScope, 80, h, 0, expanded)
			w, rows := m.vizSize()
			m.viz = fillViz{}
			out := ansi.Strip(m.render())
			if got := strings.Count(out, "Z"); got != w*rows {
				t.Fatalf("h=%d expanded=%v: %d cells drawn; vizSize %dx%d", h, expanded, got, w, rows)
			}
		}
	}
}

// fillViz draws Z in every cell.
type fillViz struct{}

func (fillViz) Name() string               { return "fill" }
func (v fillViz) Step(vizInput) visualizer { return v }
func (fillViz) Idle() bool                 { return true }
func (fillViz) Render(w, h int) []string {
	rows := make([]string, h)
	for i := range rows {
		rows[i] = strings.Repeat("Z", w)
	}
	return rows
}

func TestPausedVisualizersLetTheTickIdle(t *testing.T) {
	for k := range vizCount {
		t.Run(vizNames[k], func(t *testing.T) {
			m, _ := vizModel(t, k, 80, 24, 10, false)
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
		})
	}
}

func TestVisualizerGoldens80x24(t *testing.T) {
	for k := range vizCount {
		t.Run(vizNames[k], func(t *testing.T) {
			m, _ := vizModel(t, k, 80, 24, 14, false)
			assertGolden(t, "viz_"+vizNames[k]+"_80x24.golden", ansi.Strip(m.View().Content))
		})
	}
}

func TestTextWavesSpareTheScopeTrace(t *testing.T) {
	trace := scopeViz{}.Step(vizFrame(0.8, 1, 12, 2, true)).Render(12, 2)
	if words := textWords(trace, -1); len(words) != 0 {
		t.Fatalf("the trace reads as %d words", len(words))
	}
	if words := textWords([]string{"AB ⠁⠂ CD"}, -1); len(words) != 2 {
		t.Fatalf("text around braille: %d words; want 2", len(words))
	}
}

// barsPalette are the escape codes the bars color with, and the theme's
// dim reds the visualizers may use for trails, axes and reflections.
var barsPalette = func() map[string]bool {
	ok := map[string]bool{}
	for _, st := range []lipgloss.Style{stRed, stRedBold, stYellow, stMuted, stDim} {
		k := inkOf(st)
		ok[k.pre], ok[k.post] = true, true
	}
	return ok
}()

var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

func TestVisualizersUseTheBarsPalette(t *testing.T) {
	bright := []string{inkOf(stYellow).pre, inkOf(stRedBold).pre, inkOf(stRed).pre}
	for k := range vizCount {
		for _, real := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/real=%v", vizNames[k], real), func(t *testing.T) {
				var out strings.Builder
				for _, level := range []float64{0.2, 0.6, 1} {
					v := stepped(newVisualizer(k), 30, level, 40, 12, real)
					out.WriteString(strings.Join(v.Render(40, 12), "\n"))
				}
				for _, seq := range sgrPattern.FindAllString(out.String(), -1) {
					if !barsPalette[seq] {
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
}

// traceRows counts the rows the scope's trace reaches (bright dots, not
// the dim axis).
func traceRows(v visualizer, w, h int) int {
	n := 0
	for _, row := range v.Render(w, h) {
		if strings.Contains(row, inkOf(stRed).pre) || strings.Contains(row, inkOf(stRedBold).pre) || strings.Contains(row, inkOf(stYellow).pre) {
			n++
		}
	}
	return n
}

func TestScopeSwingsWithTheLoudness(t *testing.T) {
	quiet := stepped(newVisualizer(vizScope), 40, 0.1, 40, 12, true)
	loud := stepped(newVisualizer(vizScope), 40, 0.9, 40, 12, true)
	if q, l := traceRows(quiet, 40, 12), traceRows(loud, 40, 12); l <= q+3 {
		t.Fatalf("loud music reaches %d rows, quiet %d", l, q)
	}
	// A loud passage after a quiet one swings wider at once, and the gain
	// does not blow a quiet passage up to full scale.
	burst := stepped(quiet, 2, 0.9, 40, 12, true)
	if b, q := traceRows(burst, 40, 12), traceRows(quiet, 40, 12); b <= q+3 {
		t.Fatalf("a loud burst reaches %d rows, the quiet before it %d", b, q)
	}
	if q := traceRows(quiet, 40, 12); q >= 12 {
		t.Fatalf("quiet music fills all %d rows", q)
	}
}

func TestScopeGainNeverBlowsUpSilence(t *testing.T) {
	// Near silence, a hiss at 2% of full scale, stays by the axis however
	// long it plays; a loud wave fills the height.
	hiss := stepped(newVisualizer(vizScope), 400, 0.02, 40, 12, true)
	if n := traceRows(hiss, 40, 12); n > 2 {
		t.Fatalf("a hiss reaches %d of 12 rows", n)
	}
	if peak := hiss.(scopeViz).peak; peak < scopeMinPeak {
		t.Fatalf("the gain rose to %.1fx on a hiss; at most %.1fx", 1/peak, 1/scopeMinPeak)
	}
	loud := stepped(newVisualizer(vizScope), 40, 0.95, 40, 12, true)
	if n := traceRows(loud, 40, 12); n < 11 {
		t.Fatalf("a loud wave reaches %d of 12 rows", n)
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
	silent := stepped(newVisualizer(vizRain), 40, 0, 40, 10, true).(rainViz)
	if len(silent.drops) != 0 {
		t.Fatalf("%d drops in silence", len(silent.drops))
	}
	// Loud bass, silent treble: the rain falls on the left only.
	var v visualizer = rainViz{}
	for i := range 40 {
		in := vizFrame(0, uint64(i+1), 40, 10, true)
		for b := range in.Bands[:len(in.Bands)/2] {
			in.Bands[b] = 0.9
		}
		v = v.Step(in)
	}
	drops := v.(rainViz).drops
	if len(drops) < 5 {
		t.Fatalf("%d drops under loud bands", len(drops))
	}
	for _, d := range drops {
		if d.x >= 20 {
			t.Fatalf("a drop at column %d, under a silent band", d.x)
		}
	}
	// Louder bands fall faster.
	slow := stepped(newVisualizer(vizRain), 40, 0.3, 40, 10, true).(rainViz)
	fast := stepped(newVisualizer(vizRain), 40, 1, 40, 10, true).(rainViz)
	if len(slow.drops) == 0 || fast.drops[0].speed <= slow.drops[0].speed {
		t.Fatal("louder bands do not fall faster")
	}
}

func TestSynthwaveReadsAsASpectrum(t *testing.T) {
	const w, h = 30, 11
	in := vizFrame(0, 1, w, h, true)
	for b := range in.Bands {
		in.Bands[b] = 0.1 + 0.8*float64(b)/float64(len(in.Bands)-1)
	}
	var v visualizer = synthViz{}
	for i := range 20 {
		in.Frame = uint64(i + 1)
		v = v.Step(in)
	}
	rows := v.Render(w, h)
	plainRows := make([]string, h)
	for i, r := range rows {
		plainRows[i] = ansi.Strip(r)
	}
	horizon := h / 2
	if plainRows[horizon] != strings.Repeat("━", w) {
		t.Fatalf("no horizon across the middle:\n%s", strings.Join(plainRows, "\n"))
	}
	// Above it a solid range: every column filled from the horizon up, as
	// high as its band, taller to the right here.
	heights := make([]int, w)
	for x := range w {
		for y := horizon - 1; y >= 0 && []rune(plainRows[y])[x] != ' '; y-- {
			heights[x]++
		}
		if heights[x] == 0 {
			t.Fatalf("column %d is empty:\n%s", x, strings.Join(plainRows, "\n"))
		}
	}
	if heights[w-1] <= heights[0] {
		t.Fatalf("the range does not follow the bands: heights %v", heights)
	}
	// Below it the reflection, dim, never the bars' bright colors.
	for y := horizon + 1; y < h; y++ {
		for _, seq := range []string{inkOf(stYellow).pre, inkOf(stRedBold).pre} {
			if strings.Contains(rows[y], seq) {
				t.Fatalf("row %d under the horizon is bright: %q", y, plainRows[y])
			}
		}
	}
	if filled(rows[horizon+1:]) == 0 {
		t.Fatal("nothing under the horizon")
	}
}

func TestSynthwaveGridPulsesWithTheBass(t *testing.T) {
	quiet := stepped(newVisualizer(vizSynthwave), 1, 0.05, 40, 12, true).(synthViz)
	loud := stepped(newVisualizer(vizSynthwave), 1, 1, 40, 12, true).(synthViz)
	if loud.bass <= quiet.bass || loud.phase <= quiet.phase {
		t.Fatalf("bass %v / %v, grid moved %v / %v", quiet.bass, loud.bass, quiet.phase, loud.phase)
	}
}
