package radio

import (
	"math"
	"slices"
)

// rainViz is data rain: hex digits and half-width katakana fall down the
// columns (every other one), a bright head over a trail that dims, in the
// bars' colors. Each column follows the band under it: silent, nothing
// falls; the louder it gets, the more often drops start there, the faster
// and longer they fall, and the brighter the column burns, up to yellow
// heads, as a bar that high would. Without the player's readings it
// drizzles slowly, dim, instead. Paused, the rain holds still.
type rainViz struct {
	drops []rainDrop
	// levels are the bands under the columns on the last frame (nil
	// without readings): they light the drops as they fall.
	levels []float64
	// tick counts the frames rained, the clock of the rain.
	tick uint64
}

// rainDrop is one falling string of glyphs in column x, its head at row
// y, length cells long.
type rainDrop struct {
	x      int
	y      float64
	speed  float64
	length int
	seed   uint64
}

// rainGlyphs are one cell wide each: hex digits and the half-width
// katakana ｦ to ﾝ (U+FF66 to U+FF9D).
var rainGlyphs = func() []rune {
	g := []rune("0123456789ABCDEF")
	for r := rune(0xFF66); r <= 0xFF9D; r++ {
		g = append(g, r)
	}
	return g
}()

const (
	// saltRain keeps the rain's random stream apart from the other effects'.
	saltRain uint64 = 301
	// rainFloor is the band level under which a column stays dry.
	rainFloor = 0.08
)

func (rainViz) Name() string { return vizNames[vizRain] }

func (v rainViz) Step(in vizInput) visualizer {
	if !in.Playing {
		return v
	}
	v.tick++
	v.levels = nil
	if in.Real {
		v.levels = make([]float64, max(in.W, 0))
		for x := range v.levels {
			v.levels[x] = bandAt(in.Bands, x, in.W)
		}
	}
	drops := make([]rainDrop, 0, len(v.drops)+in.W/2+1)
	// busy marks the columns whose newest drop has not left the top yet,
	// so drops in a column never overlap.
	busy := make([]bool, max(in.W, 0))
	for _, d := range v.drops {
		d.y += d.speed
		if d.x >= in.W || d.y-float64(d.length) >= float64(in.H) {
			continue
		}
		if d.y-float64(d.length) < 1 {
			busy[d.x] = true
		}
		drops = append(drops, d)
	}
	for x := 0; x < in.W; x += 2 {
		if busy[x] {
			continue
		}
		chance, speed, length := 0.04, 0.35, 3
		if in.Real {
			level := v.levels[x]
			if level < rainFloor {
				continue
			}
			chance = 0.05 + 0.6*level*level
			speed = 0.3 + 1.7*level
			length = 1 + int(math.Round(1.2*level*float64(in.H)))
		}
		h := mix(in.Seed, saltRain, v.tick, uint64(x))
		if unit(h) >= chance {
			continue
		}
		drops = append(drops, rainDrop{x: x, speed: speed, length: length, seed: h})
	}
	v.drops = drops
	return v
}

func (rainViz) Idle() bool { return true }

func (v rainViz) Render(w, h int) []string {
	c := newCanvas(w, h)
	n := uint64(len(rainGlyphs))
	for _, d := range v.drops {
		if d.x >= w {
			continue
		}
		// The head burns in the ink of its band's level (dim red in a
		// drizzle); the trail steps down the bars' colors to dim.
		head := inkMuted
		if d.x < len(v.levels) {
			head = levelInk(v.levels[d.x])
		}
		top := int(d.y)
		for i := d.length - 1; i >= 0; i-- {
			y := top - i
			if y < 0 || y >= h {
				continue
			}
			// A trail cell keeps its glyph; the head flickers.
			g := rainGlyphs[mix(d.seed, uint64(y))%n]
			k := trailInk(head, i, d.length)
			if i == 0 {
				g, k = rainGlyphs[mix(d.seed, v.tick)%n], head
			}
			c.set(d.x, y, g, k)
		}
	}
	return c.lines()
}

// rainRamp is the palette a trail steps down, brightest first.
var rainRamp = []uint8{inkYellow, inkRedBold, inkRed, inkMuted, inkDim}

// trailInk is the ink of cell i of a trail length long under a head in
// ink head: one step down the ramp from it next to the head, then down to
// three more along the trail, never past the dimmest.
func trailInk(head uint8, i, length int) uint8 {
	at := max(slices.Index(rainRamp, head), 0)
	return rainRamp[min(at+1+i*3/max(length, 1), len(rainRamp)-1)]
}
