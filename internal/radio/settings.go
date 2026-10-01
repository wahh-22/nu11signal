package radio

import (
	"strconv"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/config"
)

// The SETTINGS overlay is drawn like the KEYS overlay, in a panel over
// the whole body. keySettings opens it wherever keyHelp opens KEYS (not
// where typing takes the key, see keysTyped), and keySettings or esc
// closes it. Its THEMES section lists the themes, the active one marked
// ◉: ↑↓ move the cursor, enter (or a click on a row) applies the theme
// at once, the overlay staying open to compare, and saves the choice to
// the settings file. Every other key is ignored, ctrl+c still quits; a
// click off the rows closes it.

// settingsOverlayHints replace the view's hints while SETTINGS is open.
var settingsOverlayHints = []hint{
	{"↑↓", "MOVE"},
	{"ENTER", "APPLY"},
	{"S/ESC", "CLOSE"},
	{"CTRL+C", "QUIT"},
}

// settingsHint is the footers' entry for keySettings, the first to go
// before KEYS and quit when the footer is too narrow.
var settingsHint = hint{"S", "SETTINGS"}

// zoneSettingsTheme prefixes the zone of a theme row; its index follows.
const zoneSettingsTheme = "settings:theme:"

// configSavedMsg reports a settings file save.
type configSavedMsg struct{ err error }

// settingsKey handles a key press while SETTINGS is open, or keySettings
// opening it; ok is false for every other key, which the view takes.
func (m Model) settingsKey(k string) (next Model, cmd tea.Cmd, ok bool) {
	switch {
	case m.settings:
		switch k {
		case keySettings, keyEsc:
			m.settings = false
		case keyUp:
			m.settingsCursor = max(m.settingsCursor-1, 0)
		case keyDown:
			m.settingsCursor = min(m.settingsCursor+1, len(themes)-1)
		case keyEnter:
			m, cmd = m.chooseTheme(themes[m.settingsCursor])
		}
		return m, cmd, true
	case k == keySettings && !m.help && m.auth != authFailed && !m.keysTyped():
		m.settings = true
		m.settingsCursor = 0
		for i, t := range themes {
			if t.name == m.theme {
				m.settingsCursor = i
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

// settingsClick handles a left press while SETTINGS is open: a theme row
// applies its theme, anywhere else closes the overlay.
func (m Model) settingsClick(x, y int) (Model, tea.Cmd) {
	_, zs := m.layout()
	if z, ok := zs.at(x, y); ok && strings.HasPrefix(z.id, zoneSettingsTheme) {
		if i, err := strconv.Atoi(strings.TrimPrefix(z.id, zoneSettingsTheme)); err == nil && i >= 0 && i < len(themes) {
			m.settingsCursor = i
			return m.chooseTheme(themes[i])
		}
	}
	m.settings = false
	return m, nil
}

// chooseTheme applies t and saves it; a theme chosen here wins over the
// settings file read late at startup (see onConfig).
func (m Model) chooseTheme(t theme) (Model, tea.Cmd) {
	m.themePicked = true
	if t.name == m.theme {
		return m, nil
	}
	m = m.setTheme(t)
	m.settingsFile.Theme = t.name
	return m, m.saveConfigCmd(m.settingsFile)
}

// setTheme applies t to the package styles (see applyTheme) and to the
// Model's text inputs, which hold copies of theirs.
func (m Model) setTheme(t theme) Model {
	applyTheme(t)
	m.theme = t.name
	m.input.SetStyles(inputStyles())
	m.nameInput.SetStyles(inputStyles())
	return m
}

// configSaves orders the settings saves, which run concurrently as
// commands: each is numbered when asked for, and one older than a save
// already written is dropped, so the file always ends with the latest
// choice. Shared by every copy of the Model.
type configSaves struct {
	mu            sync.Mutex
	asked, latest uint64
}

// saveConfigCmd writes c to the settings file unless a newer save got
// there first; nil without a source.
func (m Model) saveConfigCmd(c config.Config) tea.Cmd {
	src, saves := m.configSource, m.configSaves
	if src == nil || saves == nil {
		return nil
	}
	saves.mu.Lock()
	saves.asked++
	seq := saves.asked
	saves.mu.Unlock()
	return func() tea.Msg {
		saves.mu.Lock()
		defer saves.mu.Unlock()
		if seq < saves.latest {
			return configSavedMsg{}
		}
		err := src.Save(c)
		if err == nil {
			saves.latest = seq
		}
		return configSavedMsg{err: err}
	}
}

// settingsPanel frames the SETTINGS overlay, w x h cells, with the zones
// of its theme rows relative to the panel.
func (m Model) settingsPanel(w, h int) ([]string, zones) {
	body := []string{" " + stHeading.Render("▞ THEMES")}
	var zs zones
	iw := w - 2
	for i, t := range themes {
		mark := "○"
		if t.name == m.theme {
			mark = "◉"
		}
		text := "  " + mark + " " + t.name
		row := stText.Render(text)
		if i == m.settingsCursor {
			row = stSelected.Render(fit(text, max(iw, 0)))
		}
		zs.add(zoneSettingsTheme+strconv.Itoa(i), 1, 1+len(body), iw)
		body = append(body, row)
	}
	return panel("SETTINGS", "S/ESC CLOSE", body, w, h, true), zs.clip(w-1, h-1)
}
