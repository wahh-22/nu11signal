package radio

import (
	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/config"
)

// configMsg carries the settings file, read once at startup.
type configMsg struct {
	cfg config.Config
	err error
}

// loadConfigCmd reads the settings; nil without a source (the defaults).
func (m Model) loadConfigCmd() tea.Cmd {
	src := m.configSource
	if src == nil {
		return nil
	}
	return func() tea.Msg {
		cfg, err := src.Load()
		return configMsg{cfg: cfg, err: err}
	}
}

// onConfig applies the settings. The only one, the visualizer, is read
// and ignored: the rain is the only visualizer (see viz.go), and a name
// from before, of a visualizer since retired, is no error. A file that
// cannot be read says so once.
func (m Model) onConfig(msg configMsg) Model {
	if msg.err != nil {
		m.setStatus("CONFIG UNREADABLE // " + msg.err.Error())
	}
	return m
}

// stepViz advances the rain a frame, after the bars (stepBars).
func (m Model) stepViz() Model {
	m.rain = m.rain.Step(m.vizInput())
	return m
}

// vizInput is what the rain reads this frame.
func (m Model) vizInput() vizInput {
	w, h := m.vizSize()
	bands := make([]float64, m.eqBarCount())
	copy(bands, m.bars[:])
	in := vizInput{Bands: bands, Playing: m.isPlaying(), Seed: m.seed, W: w, H: h}
	if reading, ok := m.liveSpectrum(); ok {
		in.Real, in.Wave = true, reading.Wave
	}
	return in
}

// vizSize is the size of the spectrum area: the bottom of NOW PLAYING in
// the full layout, under its controls (see nowPlaying); 0 x 0 where
// there is none.
func (m Model) vizSize() (w, h int) {
	if m.width < fullMinWidth || m.height < fullMinHeight || m.auth == authFailed {
		return 0, 0
	}
	ih := m.height - 6 // the panel's inside: the body less its frame
	return m.playerPanelWidth() - 2 - 2*nowPlayingMargin, vizRows(ih, nowPlayingControlRows(ih))
}

// nowPlayingControlRows is how many rows NOW PLAYING draws over the
// spectrum area in ih rows: head to LOOP, with the gap over the buttons
// once there is room for it.
func nowPlayingControlRows(ih int) int {
	if ih > 10 {
		return 12
	}
	return 11
}

// vizRows is the height of the spectrum area under used rows of ih, at
// most eqMaxRows; 0 when fewer than 2 rows are left.
func vizRows(ih, used int) int {
	if rows := min(ih-used, eqMaxRows); rows >= 2 {
		return rows
	}
	return 0
}
