package radio

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/config"
	"github.com/wahh-22/nu11signal/internal/playback"
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

// onConfig applies the settings. A file that cannot be read, or names no
// visualizer this knows, leaves the bars and says so once.
func (m Model) onConfig(msg configMsg) Model {
	mode, ok := parseVisualizer(msg.cfg.Visualizer)
	switch {
	case msg.err != nil:
		mode = vizMode{kind: vizBars}
		m.setStatus("CONFIG UNREADABLE // VISUALIZER BARS // " + msg.err.Error())
	case !ok:
		m.setStatus(fmt.Sprintf("UNKNOWN VISUALIZER %q // BARS", strings.TrimSpace(msg.cfg.Visualizer)))
	}
	m.vizMode = mode
	if mode.random {
		// No visualizer was picked for a song yet: any may come first.
		return m.setVisualizer(randomVisualizer(songKey(m.state), vizNone))
	}
	return m.setVisualizer(mode.kind)
}

// vizNone is no visualizer, for randomVisualizer's prev.
const vizNone vizKind = -1

// songKey identifies the song of s for the random visualizer: its id,
// else its title.
func songKey(s playback.State) string {
	if s.SongID != "" {
		return s.SongID
	}
	return s.Title
}

// pickForSong shows the random visualizer of the song in s, when the
// setting is random and it is another song than the one shown.
func (m Model) pickForSong(s playback.State) Model {
	if !m.vizMode.random || songKey(s) == songKey(m.state) {
		return m
	}
	return m.setVisualizer(randomVisualizer(songKey(s), m.vizKind))
}

// setVisualizer shows visualizer k, from a blank history.
func (m Model) setVisualizer(k vizKind) Model {
	m.vizKind, m.viz = k, newVisualizer(k)
	return m
}

// cycleVisualizer shows the next visualizer for the session and says so.
// In random mode the next song picks at random again.
func (m Model) cycleVisualizer() Model {
	m = m.setVisualizer((m.vizKind + 1) % vizCount)
	m.setStatus("VISUALIZER // " + strings.ToUpper(m.viz.Name()))
	return m
}

// stepViz advances the visualizer a frame, after the bars (stepBars).
func (m Model) stepViz() Model {
	m.viz = m.viz.Step(m.vizInput())
	return m
}

// vizInput is what the visualizer reads this frame.
func (m Model) vizInput() vizInput {
	w, h := m.vizSize()
	bands := make([]float64, m.eqBarCount())
	copy(bands, m.bars[:])
	in := vizInput{Bands: bands, Playing: m.isPlaying(), Frame: m.frame, Seed: m.seed, W: w, H: h}
	if reading, ok := m.liveSpectrum(); ok {
		in.Real, in.Wave = true, reading.Wave
	}
	return in
}

// activeViz is the visualizer to draw. The bars draw the Model's own bars
// as they are, even before a frame steps them onto a new panel size.
func (m Model) activeViz() visualizer {
	if m.vizKind == vizBars {
		return barsViz{bars: m.bars}
	}
	return m.viz
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
