package radio

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The visualizer is what the spectrum area at the bottom of NOW PLAYING
// draws: the rain (see rainViz), the only one. The config file's
// visualizer setting, which once picked among several, is read and
// ignored (see onConfig).
//
// The rain reads the same input each animation frame (vizInput): the
// bars' levels, which already follow the player's readings or animate
// decoratively and fall when paused (see stepBars), and the player's
// waveform when it sends one. The rain is a value: Step returns the next
// one and never changes the receiver, so older Models keep theirs.

// vizInput is what the rain reads each frame.
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
	// Seed is the Model's seed.
	Seed uint64
	// W and H are the size the rain is drawn at.
	W, H int
}

// ink is a style as the escape codes around a run of cells: the
// rain colors cell by cell, and styling each run through lipgloss
// would cost more than drawing it.
type ink struct{ pre, post string }

func inkOf(st lipgloss.Style) ink {
	s := st.Render("x")
	i := strings.Index(s, "x")
	return ink{pre: s[:i], post: s[i+1:]}
}

// Inks of the rain's palette, the equalizer bars' own: yellow tips, bold
// red, red, and the theme's dim reds for what sits behind the music (the
// trails). The rain's trail steps down
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
