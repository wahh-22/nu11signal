package radio

// waterfallViz is a scrolling spectrogram: each frame the bands become a
// new row at the top and the older rows move down, so the recent music
// flows down the panel; a cell's shade and color (cyan through magenta to
// yellow) say how loud its band was. It keeps waterfallRows rows, the
// most the spectrum area shows, and only scrolls while playing: paused,
// it holds still.
type waterfallViz struct {
	// rows are the band levels of past frames, newest first.
	rows [][]float64
}

const waterfallRows = eqMaxRows

// waterfallShades draw a cell from quiet (blank) to loud.
var waterfallShades = []rune(" ░▒▓█")

func (waterfallViz) Name() string { return vizNames[vizWaterfall] }

func (v waterfallViz) Step(in vizInput) visualizer {
	if !in.Playing {
		return v
	}
	rows := make([][]float64, 0, waterfallRows)
	rows = append(rows, append([]float64(nil), in.Bands...))
	v.rows = append(rows, v.rows[:min(len(v.rows), waterfallRows-1)]...)
	return v
}

func (waterfallViz) Idle() bool { return true }

func (v waterfallViz) Render(w, h int) []string {
	c := newCanvas(w, h)
	for y := range min(h, len(v.rows)) {
		for x := range w {
			level := min(max(bandAt(v.rows[y], x, w), 0), 1)
			shade := int(level*float64(len(waterfallShades)-1) + 0.5)
			if shade == 0 {
				continue
			}
			c.set(x, y, waterfallShades[shade], inkGradient+uint8(min(int(level*gradientSteps), gradientSteps-1)))
		}
	}
	return c.lines()
}
