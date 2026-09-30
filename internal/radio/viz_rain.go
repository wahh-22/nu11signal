package radio

import (
	"math"
	"slices"
)

// rainViz is data rain: hex digits and half-width katakana fall down the
// columns (every other one), a bright head over a trail that dims, in the
// bars' colors. With the player's readings it plays the music:
//
//   - Each column follows the band under it through rainEnergy, a curve
//     that keeps quiet bands dry and stands the loud ones out. The more
//     energy, the more often drops start there, the faster and longer
//     they fall, and the brighter their heads burn, up to yellow. Silence
//     is completely dry.
//   - A hit, a band jumping over its recent average (see rainOnsets),
//     bursts: a bass hit sends a wave of new drops across every sounding
//     column, a higher one under its own band, as many as the hit is
//     strong, and every head flashes brighter for a few frames.
//   - The waveform's loudness, when the player sends one, scales how
//     often drops start.
//
// Without readings it drizzles slowly, dim, instead. Paused, the rain
// holds still. Everything comes from the seed and the frames rained:
// tests replay it exactly.
type rainViz struct {
	drops []rainDrop
	// levels are the energies of the columns on the last frame (nil
	// without readings): they light the heads as they fall.
	levels []float64
	// env is each band's recent average, what a hit rises over; nil
	// until a first reading (a first reading is no hit).
	env []float64
	// flash brightens every head after a hit, fading frame by frame.
	flash float64
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

// Salts keep the rain's random streams apart from the other effects'.
const (
	saltRain      uint64 = 301
	saltRainBurst uint64 = 302
)

const (
	// rainFloor is the band level at or under which a column is dry.
	rainFloor = 0.08
	// rainCurve is the power of rainEnergy's curve: over 1, twice as loud
	// is more than twice the rain.
	rainCurve = 1.7
	// A band hits when it rises over its recent average (an average
	// moving rainEnvRate of the way each frame) by more than rainOnsetMin,
	// the bass by rainBassWeight times more; the hit is full at
	// rainOnsetMin+rainOnsetSpan.
	rainEnvRate    = 0.35
	rainOnsetMin   = 0.08
	rainOnsetSpan  = 0.35
	rainBassWeight = 1.5
	// rainFlashFade is what is left of the flash after a frame; a hit
	// above the bass flashes rainTrebleFlash as bright as its strength.
	rainFlashFade   = 0.55
	rainTrebleFlash = 0.6
)

// rainEnergy is the energy of a band at level: 0 at or under rainFloor,
// 1 at full level, rising along a curve steeper than the level.
func rainEnergy(level float64) float64 {
	x := min((level-rainFloor)/(1-rainFloor), 1)
	if x <= 0 {
		return 0
	}
	return math.Pow(x, rainCurve)
}

// rainOnsets compares bands with their recent averages env: hits are the
// bands' hit strengths, 0 to 1, and bass the strongest in the lowest
// quarter of the bands, weighted up. next is env moved toward bands.
// Without a history of as many bands (the first reading, a resize), there
// is no hit and bands start one.
func rainOnsets(bands, env []float64) (hits []float64, bass float64, next []float64) {
	hits = make([]float64, len(bands))
	if len(env) != len(bands) {
		return hits, 0, slices.Clone(bands)
	}
	next = make([]float64, len(bands))
	lows := max(len(bands)/4, 1)
	for i, b := range bands {
		rise := b - env[i]
		if i < lows {
			rise *= rainBassWeight
		}
		hits[i] = min(max((rise-rainOnsetMin)/rainOnsetSpan, 0), 1)
		if i < lows {
			bass = max(bass, hits[i])
		}
		next[i] = env[i] + (b-env[i])*rainEnvRate
	}
	return hits, bass, next
}

// waveGain scales the rain by the waveform's loudness: 1 without one,
// else 0.7 for silence up to 1.3 for a loud wave.
func waveGain(wave []float64) float64 {
	if len(wave) == 0 {
		return 1
	}
	var sum float64
	for _, s := range wave {
		sum += s * s
	}
	return 0.7 + 0.6*min(math.Sqrt(sum/float64(len(wave)))*2.5, 1)
}

func (v rainViz) Step(in vizInput) rainViz {
	if !in.Playing {
		return v
	}
	v.tick++
	w := max(in.W, 0)
	var hits []float64
	var bass float64
	v.levels = nil
	if in.Real {
		hits, bass, v.env = rainOnsets(in.Bands, v.env)
		v.levels = make([]float64, w)
		for x := range v.levels {
			v.levels[x] = rainEnergy(bandLevel(in.Bands, x, w))
		}
		v.flash *= rainFlashFade
		if v.flash < 0.02 {
			v.flash = 0
		}
		// A hit bursts only over what is left of the last one's flash: a
		// rise over several frames bursts once, not on each of them.
		if bass <= v.flash {
			bass = 0
		}
		top := 0.0
		for i, hit := range hits {
			if hit <= v.flash {
				hits[i] = 0
			}
			top = max(top, hits[i])
		}
		v.flash = max(v.flash, bass, rainTrebleFlash*top)
	} else {
		v.env, v.flash = nil, 0
	}
	drops := make([]rainDrop, 0, len(v.drops)+w/2+1)
	// busy marks the columns whose newest drop has not left the top yet,
	// so drops in a column never overlap; fresh those whose newest drop
	// has just started, which a burst leaves alone.
	busy, fresh := make([]bool, w), make([]bool, w)
	for _, d := range v.drops {
		d.y += d.speed
		if d.x >= w || d.y-float64(d.length) >= float64(in.H) {
			continue
		}
		if d.y-float64(d.length) < 1 {
			busy[d.x] = true
		}
		if d.y < 1 {
			fresh[d.x] = true
		}
		drops = append(drops, d)
	}
	gain := waveGain(in.Wave)
	for x := 0; x < w; x += 2 {
		if in.Real {
			if d, ok := v.burstDrop(in, x, hits, bass, fresh[x]); ok {
				drops = append(drops, d)
				continue
			}
		}
		if busy[x] {
			continue
		}
		chance, speed, length := 0.04, 0.35, 3
		if in.Real {
			e := v.levels[x]
			if e == 0 {
				continue
			}
			chance = (0.03 + 0.75*e) * gain
			speed = 0.25 + 1.75*e
			length = 1 + int(math.Round(1.1*e*float64(in.H)))
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

// burstDrop is the drop a hit starts in column x, if one does: a bass hit
// anywhere, a higher band's under it, in a column that sounds and whose
// newest drop has not just started; the stronger the hit, the likelier,
// faster and longer.
func (v rainViz) burstDrop(in vizInput, x int, hits []float64, bass float64, fresh bool) (rainDrop, bool) {
	e := v.levels[x]
	if fresh || e == 0 || len(hits) == 0 {
		return rainDrop{}, false
	}
	hit := max(bass, hits[min(x*len(hits)/max(in.W, 1), len(hits)-1)])
	if hit == 0 {
		return rainDrop{}, false
	}
	h := mix(in.Seed, saltRainBurst, v.tick, uint64(x))
	if unit(h) >= 0.1+0.7*hit {
		return rainDrop{}, false
	}
	strength := max(e, hit)
	return rainDrop{
		x:      x,
		speed:  0.6 + 1.4*strength,
		length: 1 + int(math.Round(0.9*strength*float64(in.H))),
		seed:   h,
	}, true
}

// bandLevel is the level of the band under column x of w, the bands
// spread evenly across the width; 0 without bands.
func bandLevel(bands []float64, x, w int) float64 {
	if len(bands) == 0 || w <= 0 {
		return 0
	}
	return bands[min(x*len(bands)/w, len(bands)-1)]
}

func (v rainViz) Render(w, h int) []string {
	c := newCanvas(w, h)
	n := uint64(len(rainGlyphs))
	for _, d := range v.drops {
		if d.x >= w {
			continue
		}
		// The head burns in the ink of its column's energy (dim red in a
		// drizzle), brighter in a flash; the trail steps down the bars'
		// colors to dim.
		head := inkMuted
		if d.x < len(v.levels) {
			head = rainHeadInk(v.levels[d.x], v.flash)
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

// rainHeadInk is the ink of a head over a column of energy e: muted when
// faint, red, bold red, then yellow when loud; a flash lifts it a step,
// two when strong.
func rainHeadInk(e, flash float64) uint8 {
	at := 3
	switch {
	case e >= 0.7:
		at = 0
	case e >= 0.35:
		at = 1
	case e >= 0.1:
		at = 2
	}
	switch {
	case flash >= 0.5:
		at -= 2
	case flash >= 0.2:
		at--
	}
	return rainRamp[max(at, 0)]
}

// trailInk is the ink of cell i of a trail length long under a head in
// ink head: one step down the ramp from it next to the head, then down to
// three more along the trail, never past the dimmest.
func trailInk(head uint8, i, length int) uint8 {
	at := max(slices.Index(rainRamp, head), 0)
	return rainRamp[min(at+1+i*3/max(length, 1), len(rainRamp)-1)]
}
