package radio

import (
	"hash/fnv"
	"strings"

	"charm.land/lipgloss/v2"
)

// The visualizer is what the spectrum area at the bottom of NOW PLAYING
// draws: the equalizer bars, or one of the other looks below. The config
// file picks one, or "random" for a new one each song (see config.go);
// keyVisualizer cycles them for the session.
//
// Every visualizer reads the same input each animation frame (vizInput):
// the bars' levels, which already follow the player's readings or animate
// decoratively and fall when paused (see stepBars), and the player's
// waveform when it sends one. Visualizers are values: Step returns the
// next one and never changes the receiver, so older Models keep theirs.
type visualizer interface {
	// Name is the visualizer's config name.
	Name() string
	// Step advances the visualizer one animation frame.
	Step(in vizInput) visualizer
	// Render draws the visualizer as exactly h lines of exactly w cells.
	Render(w, h int) []string
	// Idle reports that a Step while not playing would change nothing, so
	// the animation tick may slow down (see Model.animating).
	Idle() bool
}

// vizInput is what a visualizer reads each frame.
type vizInput struct {
	// Bands are the bars' levels, 0 to 1 from low to high frequencies, one
	// per bar the panel shows (see eqBarCount).
	Bands []float64
	// Wave is the player's latest waveform, -1 to 1, oldest first; nil
	// without one (a player that does not measure, or not playing).
	Wave []float64
	// Playing is whether music plays; Real whether Bands and Wave come
	// from the player's readings rather than the decorative animation.
	Playing, Real bool
	// Frame counts animation frames; Seed is the Model's seed.
	Frame, Seed uint64
	// W and H are the size the visualizer is drawn at.
	W, H int
}

// vizKind names a visualizer.
type vizKind int

const (
	vizBars vizKind = iota
	vizScope
	vizRain
	vizSynthwave
	vizCount
)

// vizNames are the visualizers' config names, in cycling order.
var vizNames = [vizCount]string{"bars", "oscilloscope", "rain", "synthwave"}

// vizRandom is the config name that picks a visualizer per song.
const vizRandom = "random"

func (k vizKind) String() string { return vizNames[k] }

// newVisualizer returns visualizer k with no history.
func newVisualizer(k vizKind) visualizer {
	switch k {
	case vizScope:
		return scopeViz{}
	case vizRain:
		return rainViz{}
	case vizSynthwave:
		return synthViz{}
	}
	return barsViz{}
}

// vizMode is the visualizer setting: a fixed kind, or random per song.
type vizMode struct {
	kind   vizKind
	random bool
}

// parseVisualizer reads a config name, in any case and ignoring spaces
// around it; empty is bars. ok is false for an unknown name, which is
// bars too.
func parseVisualizer(name string) (mode vizMode, ok bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "":
		return vizMode{kind: vizBars}, true
	case vizRandom:
		return vizMode{random: true}, true
	}
	for k, n := range vizNames {
		if n == name {
			return vizMode{kind: vizKind(k)}, true
		}
	}
	return vizMode{kind: vizBars}, false
}

// randomVisualizer is the visualizer random mode shows for song: a hash
// of its id picks one, but never prev, the one shown before it (there are
// always others to pick). The same song after the same visualizer always
// gets the same one.
func randomVisualizer(song string, prev vizKind) vizKind {
	h := fnv.New64a()
	h.Write([]byte(song))
	sum := h.Sum64()
	k := vizKind(sum % uint64(vizCount))
	if k == prev {
		// One of the others, walking on from prev.
		k = (prev + 1 + vizKind((sum/uint64(vizCount))%uint64(vizCount-1))) % vizCount
	}
	return k
}

// ink is a style as the escape codes around a run of cells: the
// visualizers color cell by cell, and styling each run through lipgloss
// would cost more than drawing it.
type ink struct{ pre, post string }

func inkOf(st lipgloss.Style) ink {
	s := st.Render("x")
	i := strings.Index(s, "x")
	return ink{pre: s[:i], post: s[i+1:]}
}

// Inks of the visualizers' palette, the bars' own (see barsViz): yellow
// tips, bold red, red, and the theme's dim reds for what sits behind the
// music (trails, axes, grids, reflections). The rain's trail steps down
// them in rainRamp's order, not in their numbers'. inkNone draws unstyled.
const (
	inkNone uint8 = iota
	inkYellow
	inkRedBold
	inkRed
	inkMuted
	inkDim
)

var vizInks = []ink{
	inkNone:    {},
	inkYellow:  inkOf(stYellow),
	inkRedBold: inkOf(stRedBold),
	inkRed:     inkOf(stRed),
	inkMuted:   inkOf(stMuted),
	inkDim:     inkOf(stDim),
}

// barInk is the bars' ink for row y of h, top first: the yellow tips on
// the top row, bold red over the upper half, red below.
func barInk(y, h int) uint8 {
	switch {
	case y == 0:
		return inkYellow
	case y < h/2:
		return inkRedBold
	}
	return inkRed
}

// levelInk is the bars' ink for a level, 0 to 1: the ink of the row a bar
// that high tops out in, on a 12-row panel.
func levelInk(level float64) uint8 {
	switch {
	case level >= 0.9:
		return inkYellow
	case level >= 0.5:
		return inkRedBold
	}
	return inkRed
}

// canvas is a w x h grid of one-cell glyphs, each with an ink, that
// renders to styled lines.
type canvas struct {
	w, h  int
	glyph []rune
	ink   []uint8
}

// newCanvas is a blank canvas (spaces, unstyled).
func newCanvas(w, h int) canvas {
	w, h = max(w, 0), max(h, 0)
	c := canvas{w: w, h: h, glyph: make([]rune, w*h), ink: make([]uint8, w*h)}
	for i := range c.glyph {
		c.glyph[i] = ' '
	}
	return c
}

// set draws r in ink k at (x, y); outside the canvas it does nothing.
func (c canvas) set(x, y int, r rune, k uint8) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.glyph[y*c.w+x], c.ink[y*c.w+x] = r, k
}

// at is the glyph at (x, y).
func (c canvas) at(x, y int) rune { return c.glyph[y*c.w+x] }

// lines renders the canvas, one styled line per row, each run of cells in
// the same ink styled once; spaces are never styled.
func (c canvas) lines() []string {
	out := make([]string, c.h)
	var b strings.Builder
	for y := range c.h {
		b.Reset()
		row := c.glyph[y*c.w : (y+1)*c.w]
		inks := c.ink[y*c.w : (y+1)*c.w]
		inkAt := func(x int) uint8 {
			if row[x] == ' ' {
				return inkNone
			}
			return inks[x]
		}
		for x := 0; x < c.w; {
			k := inkAt(x)
			end := x + 1
			for end < c.w && inkAt(end) == k {
				end++
			}
			b.WriteString(vizInks[k].pre)
			for _, r := range row[x:end] {
				b.WriteRune(r)
			}
			b.WriteString(vizInks[k].post)
			x = end
		}
		out[y] = b.String()
	}
	return out
}

// bandAt is the level of the band under column x of w, the bands spread
// evenly across the width; 0 without bands.
func bandAt(bands []float64, x, w int) float64 {
	if len(bands) == 0 || w <= 0 {
		return 0
	}
	return bands[min(x*len(bands)/w, len(bands)-1)]
}

// energy is the mean of the bands, 0 without any.
func energy(bands []float64) float64 {
	if len(bands) == 0 {
		return 0
	}
	var sum float64
	for _, v := range bands {
		sum += v
	}
	return sum / float64(len(bands))
}
