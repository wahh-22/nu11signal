package radio

import "math"

// synthViz is a synthwave horizon: a striped sun sinking behind a ridge
// of mountains raised by the spectrum (bass on the left), over a
// perspective grid that scrolls toward the viewer, faster the louder the
// music. The mountains ease toward the bands; paused, the grid stops and
// the mountains fall with the bars.
type synthViz struct {
	// ridge are the mountains' levels, the bands eased.
	ridge []float64
	// phase is how far the grid scrolled; its fraction places the lines.
	phase float64
}

const (
	synthEase   = 0.35 // per frame, of the way to the bands
	synthSpeed  = 0.06 // grid lines per frame in silence
	synthBoost  = 0.30 // more at full energy
	synthRidgeH = 0.65 // of the sky, at full level: the sun shows over it
)

func (synthViz) Name() string { return vizNames[vizSynthwave] }

func (v synthViz) Step(in vizInput) visualizer {
	ridge := make([]float64, len(in.Bands))
	for i, b := range in.Bands {
		old := 0.0
		if i < len(v.ridge) {
			old = v.ridge[i]
		}
		if ridge[i] = old + (b-old)*synthEase; ridge[i] < 0.01 {
			ridge[i] = 0
		}
	}
	v.ridge = ridge
	if in.Playing {
		v.phase = math.Mod(v.phase+synthSpeed+synthBoost*energy(in.Bands), 1)
	}
	return v
}

func (v synthViz) Idle() bool {
	for _, r := range v.ridge {
		if r != 0 {
			return false
		}
	}
	return true
}

func (v synthViz) Render(w, h int) []string {
	c := newCanvas(w, h)
	if w <= 0 || h <= 0 {
		return c.lines()
	}
	grid := (h - 1) / 2
	horizon := h - 1 - grid
	v.sun(c, horizon)
	v.mountains(c, horizon)
	for x := range w {
		c.set(x, horizon, '━', inkMagentaBold)
	}
	v.grid(c, horizon, grid)
	return c.lines()
}

// sun draws a half sun on the horizon, yellow at the top to magenta, its
// lower half striped, when the sky has room for one.
func (v synthViz) sun(c canvas, horizon int) {
	r := min(horizon, c.w/6)
	if r < 3 {
		return
	}
	cx := float64(c.w) / 2
	for y := horizon - r; y < horizon; y++ {
		up := horizon - y // 1 to r rows over the horizon
		dy := float64(up) - 0.5
		half := 2 * math.Sqrt(float64(r*r)-dy*dy) // cells are twice as tall as wide
		glyph := '█'
		if up <= r/2 && up%2 == 1 {
			glyph = '▄' // the stripes
		}
		k := inkGradient + uint8(gradientSteps/2+(gradientSteps/2-1)*up/r)
		for x := int(math.Ceil(cx - half)); x < int(cx+half); x++ {
			c.set(x, y, glyph, k)
		}
	}
}

// mountains draws the ridge over the sky, rising from the horizon.
func (v synthViz) mountains(c canvas, horizon int) {
	if len(v.ridge) == 0 || horizon <= 0 {
		return
	}
	steps := len(eqGlyphs) - 1
	for x := range c.w {
		t := 0.0
		if c.w > 1 {
			t = float64(x) / float64(c.w-1)
		}
		level := min(max(sampleAt(v.ridge, t), 0), 1)
		eighths := int(level*synthRidgeH*float64(horizon*steps) + 0.5)
		for row := 0; eighths > 0; row++ {
			y := horizon - 1 - row
			fill := min(eighths, steps)
			k := inkMagentaDim
			if eighths <= steps {
				k = inkMagenta // the crest
			}
			c.set(x, y, eqGlyphs[fill], k)
			eighths -= fill
		}
	}
}

// grid draws the floor under the horizon: lines across it, closer
// together toward the horizon and moving down with the phase, and lines
// running to the vanishing point in the middle.
func (v synthViz) grid(c canvas, horizon, rows int) {
	if rows <= 0 {
		return
	}
	cx := float64(c.w) / 2
	spacing := max(float64(c.w)/5, 4)
	for j := 1; j <= rows; j++ {
		y := horizon + j
		// Row j spans the depths g/(j+0.5) to g/(j-0.5); a line lies at
		// every whole depth plus the phase.
		near, far := float64(rows)/(float64(j)+0.5), float64(rows)/(float64(j)-0.5)
		if math.Floor(far+v.phase) > math.Floor(near+v.phase) {
			k := inkMagentaDim
			if j > rows/2 {
				k = inkMagenta
			}
			for x := range c.w {
				c.set(x, y, '─', k)
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
			c.set(x, y, glyph, inkCyanDim)
		}
	}
}
