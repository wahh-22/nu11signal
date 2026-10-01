package radio

import (
	"math"
	"slices"
)

// rainViz is data rain: hex digits and half-width katakana fall down the
// columns (every other one), a bright head over a trail that dims, in the
// bars' colors. With the player's readings it plays the music:
//
//   - Each band's level is first stretched over the range it has been
//     moving in lately (see rainGain), so a flat, heavily compressed
//     track swinging between 0.5 and 0.6 rains as unevenly as its bars
//     move, while a steady level stays what it is and a loud song still
//     rains more than a quiet one.
//   - Each column follows its stretched band through rainEnergy, a curve
//     that keeps quiet bands dry and stands the loud ones out. The more
//     energy, the more often drops start there, the longer they fall and
//     the brighter their heads burn, up to yellow. Silence is completely
//     dry.
//   - The drops falling take their column's speed now, not the one they
//     started with: each frame they ease toward the speed its energy
//     calls for, so the rain speeds up and brakes with the music. A dry
//     column lets its drops fall out at their pace.
//   - A beat, total spectral flux (the bands' rises, the bass weighted)
//     over what it has been lately (see rainBeat), pulses the whole rain:
//     every drop falls a row further and every head burns brighter, less
//     so over the next frames.
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
	// lo and hi are each band's running floor and ceiling, the range
	// rainGain stretches it over; nil until a first reading.
	lo, hi []float64
	// prev are the bands of the last reading, what the flux rises from;
	// nil until a first reading.
	prev []float64
	// flux is the recent average of the spectral flux, what a beat rises
	// over; wait the frames left before another beat can pulse.
	flux float64
	wait int
	// pulse is 1 on a beat's frame and fades after: it pushes every drop
	// further and brightens every head.
	pulse float64
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
	// A band's ceiling jumps to a new high and sinks rainGainRelease of
	// the way back to the level each frame; its floor, the other way
	// round. Between them rainGain stretches the level so that the range
	// spans rainGainSpan, but never shrinks it, and never stretches a
	// range narrower than rainGainMinSpan more than one that wide, so
	// noise is not blown up; bands quieter than rainGainKnee stretch less,
	// down to not at all at rainFloor.
	rainGainRelease = 0.05
	rainGainSpan    = 0.5
	rainGainMinSpan = 0.15
	rainGainKnee    = 0.3
	// rainSpeedEase is how much of the way from its speed to its column's
	// a falling drop goes each frame.
	rainSpeedEase = 0.5
	// A beat is a spectral flux over rainBeatRatio times its recent average
	// (moving rainFluxRate of the way each frame) plus rainBeatMin, at
	// least rainBeatGap frames after the last one.
	rainFluxRate  = 0.15
	rainBeatRatio = 1.5
	rainBeatMin   = 0.02
	rainBeatGap   = 3
	// On a beat every drop falls rainPulseFall rows further and every head
	// burns as in a flash rainPulseFlash strong; rainPulseFade is what is
	// left of the pulse a frame later.
	rainPulseFall  = 1.0
	rainPulseFlash = 0.6
	rainPulseFade  = 0.45
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

// rainGain stretches bands over their recent ranges: levels are the
// stretched bands, lo and hi the ranges moved on by this reading. A level
// moves away from the middle of its range by as much more as the range
// is narrower than rainGainSpan, so a steady level stays put and the
// loudness of a song still shows through; a level at or under rainFloor
// stays where it is, dry. Without a history of as many bands (the first
// reading, a resize), each range starts at its band and nothing is
// stretched.
func rainGain(bands, lo, hi []float64) (levels, nlo, nhi []float64) {
	if len(lo) != len(bands) || len(hi) != len(bands) {
		return slices.Clone(bands), slices.Clone(bands), slices.Clone(bands)
	}
	levels = make([]float64, len(bands))
	nlo, nhi = make([]float64, len(bands)), make([]float64, len(bands))
	for i, b := range bands {
		l, h := lo[i]+(b-lo[i])*rainGainRelease, hi[i]+(b-hi[i])*rainGainRelease
		l, h = min(l, b), max(h, b)
		nlo[i], nhi[i] = l, h
		if b <= rainFloor {
			levels[i] = b
			continue
		}
		mid := (l + h) / 2
		stretch := max(rainGainSpan/max(h-l, rainGainMinSpan), 1)
		quiet := min(max((mid-rainFloor)/(rainGainKnee-rainFloor), 0), 1)
		stretch = 1 + (stretch-1)*quiet
		levels[i] = min(max(mid+(b-mid)*stretch, 0), 1)
	}
	return levels, nlo, nhi
}

// rainFlux is the spectral flux from prev to bands: the average rise of
// the bands, falls counting nothing, the lowest quarter rainBassWeight
// times more, levels under rainFloor counting as rainFloor so noise under
// it is no beat. Without a history of as many bands it is 0.
func rainFlux(bands, prev []float64) float64 {
	if len(prev) != len(bands) || len(bands) == 0 {
		return 0
	}
	lows := max(len(bands)/4, 1)
	var sum, weights float64
	for i, b := range bands {
		wt := 1.0
		if i < lows {
			wt = rainBassWeight
		}
		sum += wt * max(max(b, rainFloor)-max(prev[i], rainFloor), 0)
		weights += wt
	}
	return sum / weights
}

// rainBeat moves the beat detector on by a frame of spectral flux: beat
// when flux rises over its recent average avg as rainBeatRatio and
// rainBeatMin say and wait, the frames left since the last beat, is
// over. It returns the average and wait moved on.
func rainBeat(flux, avg float64, wait int) (beat bool, navg float64, nwait int) {
	beat = wait <= 0 && flux > rainBeatRatio*avg+rainBeatMin
	nwait = max(wait-1, 0)
	if beat {
		nwait = rainBeatGap - 1
	}
	return beat, avg + (flux-avg)*rainFluxRate, nwait
}

// rainSpeed is the speed of a drop in a column of energy e.
func rainSpeed(e float64) float64 { return 0.25 + 1.75*e }

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
	v.pulse *= rainPulseFade
	if v.pulse < 0.05 {
		v.pulse = 0
	}
	if in.Real {
		hits, bass, v.env = rainOnsets(in.Bands, v.env)
		var stretched []float64
		stretched, v.lo, v.hi = rainGain(in.Bands, v.lo, v.hi)
		v.levels = make([]float64, w)
		for x := range v.levels {
			v.levels[x] = rainEnergy(bandLevel(stretched, x, w))
		}
		var beat bool
		beat, v.flux, v.wait = rainBeat(rainFlux(in.Bands, v.prev), v.flux, v.wait)
		v.prev = slices.Clone(in.Bands)
		if beat {
			v.pulse = 1
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
		v.lo, v.hi, v.prev, v.flux, v.wait, v.pulse = nil, nil, nil, 0, 0, 0
	}
	drops := make([]rainDrop, 0, len(v.drops)+w/2+1)
	// busy marks the columns whose newest drop has not left the top yet,
	// so drops in a column never overlap; fresh those whose newest drop
	// has just started, which a burst leaves alone.
	busy, fresh := make([]bool, w), make([]bool, w)
	for _, d := range v.drops {
		// A falling drop eases toward its column's speed, unless the
		// column is dry.
		if d.x < len(v.levels) && v.levels[d.x] > 0 {
			d.speed += (rainSpeed(v.levels[d.x]) - d.speed) * rainSpeedEase
		}
		d.y += d.speed + rainPulseFall*v.pulse
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
			speed = rainSpeed(e)
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
		// drizzle), brighter in a flash or on a beat; the trail steps down the bars'
		// colors to dim.
		head := inkMuted
		if d.x < len(v.levels) {
			head = rainHeadInk(v.levels[d.x], max(v.flash, rainPulseFlash*v.pulse))
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
