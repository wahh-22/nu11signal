package radio

import "math"

// scopeViz is an oscilloscope: the waveform traced in braille dots across
// the width (each cell holds 2 x 4 dots), around a dim center axis, in the
// bars' colors by height: red near the axis, bold red further out, yellow
// at the edges. It draws the player's waveform when it sends one, else a
// smooth synthetic wave as loud as the bars; paused, the trace flattens
// onto the axis.
type scopeViz struct {
	// wave is the trace, scopePoints points of -1 to 1 once stepped.
	wave []float64
	// peak is the auto-gain: it jumps to the loudest point at once and
	// falls back slowly (scopeRelease), and the trace is drawn relative to
	// it, never more than 1/scopeMinPeak times larger. A loud passage
	// swings to the edges, a quiet one after it stays small for a few
	// seconds and then grows only so far; silence stays flat.
	peak float64
	// phase advances the synthetic wave.
	phase float64
}

const (
	scopePoints  = 64
	scopeMinPeak = 0.25
	scopeRelease = 0.985 // per frame, of peak: halved in about 5 s
)

func (scopeViz) Name() string { return vizNames[vizScope] }

func (v scopeViz) Step(in vizInput) visualizer {
	next := make([]float64, scopePoints)
	switch {
	case !in.Playing:
		// Flatten what is left, as the bars fall.
		for i := range v.wave {
			if next[i] = v.wave[i] * eqDecay; math.Abs(next[i]) < 0.02 {
				next[i] = 0
			}
		}
	case len(in.Wave) > 0:
		for i := range next {
			next[i] = sampleAt(in.Wave, float64(i)/float64(scopePoints-1))
		}
	default:
		v.phase += 0.35
		next = synthWave(next, v.phase, energy(in.Bands))
	}
	loudest := 0.0
	for _, p := range next {
		loudest = max(loudest, math.Abs(p))
	}
	if in.Playing {
		v.peak = max(v.peak*scopeRelease, loudest)
	}
	v.wave = next
	return v
}

// synthWave fills out with a smooth wave of two partials, amplitude amp.
func synthWave(out []float64, phase, amp float64) []float64 {
	for i := range out {
		x := float64(i) / float64(len(out)) * 2 * math.Pi
		out[i] = amp * (0.7*math.Sin(2*x+phase) + 0.3*math.Sin(5*x-1.7*phase))
	}
	return out
}

// sampleAt reads wave at t (0 to 1 across it), interpolating linearly.
func sampleAt(wave []float64, t float64) float64 {
	if len(wave) == 1 {
		return wave[0]
	}
	pos := t * float64(len(wave)-1)
	i := min(int(pos), len(wave)-2)
	f := pos - float64(i)
	return wave[i]*(1-f) + wave[i+1]*f
}

func (v scopeViz) Idle() bool {
	for _, p := range v.wave {
		if p != 0 {
			return false
		}
	}
	return true
}

func (v scopeViz) Render(w, h int) []string {
	c := newCanvas(w, h)
	if w <= 0 || h <= 0 {
		return c.lines()
	}
	g := newBraille(w, h)
	dotsH := 4 * h
	// The axis is dot row 2h, just under the middle.
	axis := newBraille(w, h)
	for x := range 2 * w {
		axis.set(x, 2*h)
	}
	if len(v.wave) > 0 {
		scale := 1 / max(v.peak, scopeMinPeak)
		prev := -1
		for x := range 2 * w {
			t := 0.0
			if w > 1 || x > 0 {
				t = float64(x) / float64(2*w-1)
			}
			p := min(max(sampleAt(v.wave, t)*scale, -1), 1)
			y := int(math.Floor((1-p)*float64(dotsH-1)/2 + 0.5))
			// Join the trace to the previous column, so steep edges stay
			// a line instead of scattered dots.
			lo, hi := y, y
			if prev >= 0 {
				lo, hi = min(y, prev+sign(y-prev)), max(y, prev+sign(y-prev))
			}
			for yy := lo; yy <= hi; yy++ {
				g.set(x, yy)
			}
			prev = y
		}
	}
	for y := range h {
		// The row's distance from the axis, 0 to 1, colors it as a bar
		// reaching that high.
		k := levelInk(math.Abs(float64(4*y+2-2*h)) / float64(2*h))
		for x := range w {
			if bits := g.bits(x, y); bits != 0 {
				c.set(x, y, brailleRune(bits|axis.bits(x, y)), k)
			} else if bits := axis.bits(x, y); bits != 0 {
				c.set(x, y, brailleRune(bits), inkDim)
			}
		}
	}
	return c.lines()
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

// braille is a grid of dots, 2 wide and 4 tall per cell, as the bits of
// Unicode braille patterns (U+2800 plus the bits of the dots raised).
type braille struct {
	w, h  int
	cells []uint8
}

func newBraille(w, h int) braille { return braille{w: w, h: h, cells: make([]uint8, w*h)} }

// brailleDots are the bits of the dots of a cell, by dot row and column:
// dots 1-2-3 down the left column, 4-5-6 down the right, then 7 and 8
// under them.
var brailleDots = [4][2]uint8{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

// set raises dot (x, y), 0 to 2w-1 across and 0 to 4h-1 down; outside
// the grid it does nothing.
func (b braille) set(x, y int) {
	if x < 0 || y < 0 || x >= 2*b.w || y >= 4*b.h {
		return
	}
	b.cells[(y/4)*b.w+x/2] |= brailleDots[y%4][x%2]
}

// bits are the dots raised in cell (x, y).
func (b braille) bits(x, y int) uint8 { return b.cells[y*b.w+x] }

func brailleRune(bits uint8) rune { return 0x2800 + rune(bits) }
