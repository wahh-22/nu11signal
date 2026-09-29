package radio

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// Mouse input is additive: every click or wheel turn does what a key
// already does, so both stay in step. Clicks act on the press of the left
// button (releases and other buttons are ignored); the zone under the
// pointer is found by laying out the frame on screen again.

// handleMouse handles one mouse message.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			return m.click(msg.X, msg.Y)
		}
	case tea.MouseWheelMsg:
		// The wheel moves the selection like the arrow keys, which scrolls
		// the list or page with it.
		switch msg.Button {
		case tea.MouseWheelUp:
			return m.handleKey(tea.KeyPressMsg{Code: tea.KeyUp})
		case tea.MouseWheelDown:
			return m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown})
		}
	}
	return m, nil
}

// click acts on the zone at cell (x, y), if any.
func (m Model) click(x, y int) (tea.Model, tea.Cmd) {
	_, zs := m.layout()
	z, ok := zs.at(x, y)
	if !ok {
		return m, nil
	}
	if row, ok := rowOf(z.id); ok {
		// Select the row, then act on it as enter does.
		m.setCursor(row)
		return m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	switch z.id {
	case zoneInput:
		m.setCursor(-1)
		return m, m.input.Focus()
	case zoneRetry:
		return m.handleKey(tea.KeyPressMsg{Code: 'r', Text: keyRetry})
	case zoneTabStations:
		if m.top().kind == viewStations {
			return m, nil
		}
		// tab parks the search branch, from SEARCH and from its pages.
		return m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	case zoneTabSearch:
		return m.searchTab()
	case zoneBack:
		return m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	case zonePrev:
		return m, m.action("PREV", m.player.Previous)
	case zonePlay:
		return m, m.togglePlay()
	case zoneNext:
		return m, m.action("NEXT", m.player.Next)
	case zoneSeek:
		// The bar spans the whole song: its first cell is the start.
		target := m.state.Duration * time.Duration(x-z.x) / time.Duration(z.w)
		return m.seekTo(target)
	}
	return m, nil
}

// searchTab is the SEARCH tab: / from the stations or a page (the parked
// branch, or the input again); on SEARCH itself, where / would be typed,
// it only takes the input back.
func (m Model) searchTab() (tea.Model, tea.Cmd) {
	switch m.top().kind {
	case viewStations:
		return m.resumeOrOpenSearch()
	case viewSearch:
		m.setCursor(-1)
		return m, m.input.Focus()
	}
	return m.searchAgain()
}
