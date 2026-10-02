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

// onConfig applies the settings. The theme is applied, the default for a
// name no theme has (silently: a theme since renamed is no error), unless
// SETTINGS already chose one. The visualizer is read and ignored: the
// rain is the only visualizer (see viz.go), and a name from before, of a
// visualizer since retired, is no error. A file that cannot be read says
// so once.
func (m Model) onConfig(msg configMsg) Model {
	if msg.err != nil {
		m.setStatus("CONFIG UNREADABLE // " + msg.err.Error())
		return m
	}
	m.settingsFile = msg.cfg
	if m.themePicked {
		m.settingsFile.Theme = m.theme
		return m
	}
	t, ok := themeNamed(msg.cfg.Theme)
	if !ok {
		t = defaultTheme
	}
	return m.setTheme(t)
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
	return m.playerPanelWidth() - 2 - 2*nowPlayingMargin, vizRows(ih, nowPlayingControlRows(m.playerPanelWidth()-2, ih))
}

// nowPlayingControlRows is how many rows NOW PLAYING, iw x ih inside,
// draws over the spectrum area: the head down to the progress bar, then
// the controls (one row or two, as hudRowCount says for the width inside
// the margins), with the gap over them once there is room for it.
func nowPlayingControlRows(iw, ih int) int {
	rows := npProgressRow + 1 + hudRowCount(iw-2*nowPlayingMargin)
	if ih > rows {
		rows++
	}
	return rows
}

// vizRows is the height of the spectrum area: every row of ih under the
// used ones; 0 when fewer than 2 rows are left.
func vizRows(ih, used int) int {
	if rows := ih - used; rows >= 2 {
		return rows
	}
	return 0
}
