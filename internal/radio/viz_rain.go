package radio

import "math"

// rainViz is data rain: hex digits and half-width katakana fall down the
// columns (every other one), a bright head over a trail that dims. Each
// column follows the band under it: the louder the band, the more often
// drops start there, the faster and longer they fall, and a loud one
// falls with a yellow head. Without the player's readings it drizzles
// slowly instead. Paused, the rain holds still.
type rainViz struct {
	drops []rainDrop
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
	loud   bool
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

// saltRain keeps the rain's random stream apart from the other effects'.
const saltRain uint64 = 301

func (rainViz) Name() string { return vizNames[vizRain] }

func (v rainViz) Step(in vizInput) visualizer {
	if !in.Playing {
		return v
	}
	v.tick++
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
		level := bandAt(in.Bands, x, in.W)
		chance, speed, length := 0.04, 0.35, 3
		if in.Real {
			chance = 0.03 + 0.4*level*level
			speed = 0.4 + 1.2*level
			length = 2 + int(math.Round(level*float64(in.H)))
		}
		h := mix(in.Seed, saltRain, v.tick, uint64(x))
		if unit(h) >= chance {
			continue
		}
		drops = append(drops, rainDrop{x: x, speed: speed, length: length, loud: in.Real && level > 0.7, seed: h})
	}
	v.drops = drops
	return v
}

func (rainViz) Idle() bool { return true }

func (v rainViz) Render(w, h int) []string {
	c := newCanvas(w, h)
	n := uint64(len(rainGlyphs))
	for _, d := range v.drops {
		head := int(d.y)
		for i := d.length - 1; i >= 0; i-- {
			y := head - i
			if y < 0 || y >= h || d.x >= w {
				continue
			}
			// A trail cell keeps its glyph; the head flickers.
			g := rainGlyphs[mix(d.seed, uint64(y))%n]
			k := inkCyanDim
			switch {
			case i == 0:
				g = rainGlyphs[mix(d.seed, v.tick)%n]
				k = inkPale
				if d.loud {
					k = inkYellow
				}
			case i <= 2:
				k = inkCyanBold
			case i < d.length/2+2:
				k = inkCyan
			case i == d.length-1:
				k = inkCyanDeep
			}
			c.set(d.x, y, g, k)
		}
	}
	return c.lines()
}
