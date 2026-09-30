package radio

// barsViz is the equalizer: the bars as they are (see eq), one every
// other column, yellow tips over bold and plain red rows.
type barsViz struct{ bars eq }

func (barsViz) Name() string { return vizNames[vizBars] }

// Step takes the bars' levels; the Model already moved them (stepBars).
func (v barsViz) Step(in vizInput) visualizer {
	v.bars = eq{}
	copy(v.bars[:], in.Bands)
	return v
}

func (v barsViz) Render(w, h int) []string {
	rows := v.bars.render(w, h)
	for i, row := range rows {
		style := stRed
		switch {
		case i == 0:
			style = stYellow
		case i < h/2:
			style = stRedBold
		}
		rows[i] = style.Render(row)
	}
	return rows
}

// Idle: stepped bars fall with the Model's (stepBars), so once flat a
// paused frame leaves them flat.
func (v barsViz) Idle() bool { return v.bars.flat() }
