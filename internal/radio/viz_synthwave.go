package radio

import "math"

// synthViz is a synthwave horizon made of the spectrum. A horizon line
// runs across the middle; above it the bands stand as one solid mountain
// range (bass on the left), every column filled up to its band like the
// bars joined into a silhouette, in the bars' colors by height; below it
// the range is mirrored, dim and squashed, over a perspective floor grid
// whose lines scroll toward the viewer faster, and glow brighter, the
// more bass there is. The range eases toward the bands; paused, the grid
// stops and the range falls as the bars do.
type synthViz struct {
	// ridge are the range's levels, the bands eased.
	ridge []float64
	// phase is how far the grid scrolled; its fraction places the lines.
	phase float64
	// bass is the level of the lowest quarter of the bands.
	bass float64
}

const (
	synthEase    = 0.35 // per frame, of the way to the bands
	synthSpeed   = 0.04 // grid lines per frame in silence
	synthBoost   = 0.40 // more at full bass
	synthReflect = 0.6  // of the floor, the reflection of a full column
)

func (synthViz) Name() string { return vizNames[vizSynthwave] }

func (v synthViz) Step(in vizInput) visualizer {
	ridge := make([]float64, len(in.Bands))
	for i, b := range in.Bands {
		old := 0.0
		if i < len(v.ridge) {
			old = v.ridge[i]
		}
		next := old + (b-old)*synthEase
		if !in.Playing {
			next = old * eqDecay
		}
		if next < 0.01 {
			next = 0
		}
		ridge[i] = next
	}
	v.ridge = ridge
	v.bass = 0
	if low := in.Bands[:(len(in.Bands)+3)/4]; len(low) > 0 && in.Playing {
		v.bass = energy(low)
	}
	if in.Playing {
		v.phase = math.Mod(v.phase+synthSpeed+synthBoost*v.bass, 1)
	}
	return v
}

func (v synthViz) Idle() bool {
	for _, r := range v.ridge {
		if r != 0 {
			return false
		}
	}
	return v.bass == 0
}

func (v synthViz) Render(w, h int) []string {
	c := newCanvas(w, h)
	if w <= 0 || h <= 0 {
		return c.lines()
	}
	horizon := h / 2
	floor := h - 1 - horizon
	v.grid(c, horizon, floor)
	steps := len(eqGlyphs) - 1
	for x := range w {
		level := v.levelAt(x, w)
		// The range: eighths of a cell up from the horizon, as the bars.
		eighths := int(level*float64(horizon*steps) + 0.5)
		for y := horizon - 1; eighths > 0 && y >= 0; y-- {
			fill := min(eighths, steps)
			c.set(x, y, eqGlyphs[fill], barInk(y, horizon))
			eighths -= fill
		}
		// Its reflection: shaded cells down from the horizon, a lighter
		// shade at the tip.
		eighths = int(level*synthReflect*float64(floor*steps) + 0.5)
		for y := horizon + 1; eighths > 0 && y < h; y++ {
			g, k := '▒', inkMuted
			if eighths < steps {
				g, k = '░', inkDim
			}
			c.set(x, y, g, k)
			eighths -= steps
		}
	}
	for x := range w {
		c.set(x, horizon, '━', inkRedBold)
	}
	return c.lines()
}

// levelAt is the range's level over column x of w, the bands spread
// across the width.
func (v synthViz) levelAt(x, w int) float64 {
	if len(v.ridge) == 0 {
		return 0
	}
	t := 0.0
	if w > 1 {
		t = float64(x) / float64(w-1)
	}
	return min(max(sampleAt(v.ridge, t), 0), 1)
}

// grid draws the floor under the horizon: lines across it, closer
// together toward the horizon, moving down with the phase and lit by the
// bass, and dim lines running to the vanishing point in the middle.
func (v synthViz) grid(c canvas, horizon, rows int) {
	if rows <= 0 {
		return
	}
	lit := inkDim
	switch {
	case v.bass >= 0.6:
		lit = inkRed
	case v.bass >= 0.3:
		lit = inkMuted
	}
	cx := float64(c.w) / 2
	spacing := max(float64(c.w)/5, 4)
	for j := 1; j <= rows; j++ {
		y := horizon + j
		// Row j spans the depths g/(j+0.5) to g/(j-0.5); a line lies at
		// every whole depth plus the phase.
		near, far := float64(rows)/(float64(j)+0.5), float64(rows)/(float64(j)-0.5)
		if math.Floor(far+v.phase) > math.Floor(near+v.phase) {
			for x := range c.w {
				c.set(x, y, '─', lit)
			}
		}
		p := float64(j) / float64(rows)
		for n := -int(cx/(spacing*p)) - 1; n <= int(cx/(spacing*p))+1; n++ {
			x := int(math.Round(cx + float64(n)*spacing*p - 0.5))
			glyph := '│'
			switch {
			case n < 0:
				glyph = '╱'
			case n > 0:
				glyph = '╲'
			}
			c.set(x, y, glyph, inkDim)
		}
	}
}
