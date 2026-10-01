package radio

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Mouse input is additive: every click or wheel turn does what a key
// already does, so both stay in step. Clicks act on the press of the left
// button (releases and other buttons are ignored); the zone under the
// pointer is found by laying out the frame on screen again.

// doubleClickGuard is how long left presses are ignored after one that
// changed the view, so the second press of a double click does not act on
// whatever sits under the pointer in the view the first one opened.
const doubleClickGuard = 400 * time.Millisecond

// handleMouse handles one mouse message.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.quitAsk {
		// The quit modal hides every zone but its own (see quitClick).
		if c, ok := msg.(tea.MouseClickMsg); ok && c.Button == tea.MouseLeft {
			return m.quitClick(c.X, c.Y)
		}
		return m, nil
	}
	if m.help {
		// The KEYS overlay hides the zones: a press closes it.
		if c, ok := msg.(tea.MouseClickMsg); ok && c.Button == tea.MouseLeft {
			m.help = false
		}
		return m, nil
	}
	if m.settings {
		if c, ok := msg.(tea.MouseClickMsg); ok && c.Button == tea.MouseLeft {
			return m.settingsClick(c.X, c.Y)
		}
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			return m.click(msg.X, msg.Y)
		}
	case tea.MouseWheelMsg:
		// The wheel moves the selection like the arrow keys, which scrolls
		// the list or page with it; the list takes the focus. The expanded
		// player hides the list, so there the wheel does nothing. The
		// wheel scrolls: at the top of the list it stops, where ↑ would
		// climb to the tabs.
		if m.expanded {
			return m, nil
		}
		var arrow tea.KeyPressMsg
		switch msg.Button {
		case tea.MouseWheelUp:
			arrow = tea.KeyPressMsg{Code: tea.KeyUp}
		case tea.MouseWheelDown:
			arrow = tea.KeyPressMsg{Code: tea.KeyDown}
		default:
			return m, nil
		}
		focus := m.focusList()
		if msg.Button == tea.MouseWheelUp && m.atListTop() {
			return m, focus
		}
		next, cmd := m.handleKey(arrow)
		return next, tea.Batch(focus, cmd)
	}
	return m, nil
}

// click acts on the zone at cell (x, y), if any. A press that changes
// the view (a push, pop or restore of the stack) holds off further presses
// for doubleClickGuard.
func (m Model) click(x, y int) (tea.Model, tea.Cmd) {
	if m.now().Before(m.pressGuardUntil) {
		return m, nil
	}
	depth, kind := len(m.stack), m.top().kind
	next, cmd := m.clickZone(x, y)
	nm, ok := next.(Model)
	if ok && (len(nm.stack) != depth || nm.top().kind != kind) {
		nm.pressGuardUntil = nm.now().Add(doubleClickGuard)
		return nm, cmd
	}
	return next, cmd
}

// clickZone acts on the zone at cell (x, y), if any.
func (m Model) clickZone(x, y int) (tea.Model, tea.Cmd) {
	_, zs := m.layout()
	z, ok := zs.at(x, y)
	if !ok {
		return m, nil
	}
	// A click on the player moves the focus there; anywhere else, to the
	// list.
	if c, ok := controlOf(z.id); ok {
		return m.pressControl(c)
	}
	if z.id == zonePanelPlayer {
		// Elsewhere on NOW PLAYING: the focus only, on PLAY coming from
		// elsewhere, else on the control it has.
		if m.focus != areaPlayer {
			m.focusPlayer(ctlPlay)
		}
		return m, nil
	}
	if z.id == zoneSeek {
		m.focusPlayer(m.control)
		m.onBar = true
		// The bar spans the whole song: its first cell is the start.
		target := m.state.Duration * time.Duration(x-z.x) / time.Duration(z.w)
		return m.seekTo(target)
	}
	// On the list panel off its rows, the focus only (clickListZone
	// ignores the panel).
	focus := m.focusList()
	next, cmd := m.clickListZone(z)
	return next, tea.Batch(focus, cmd)
}

// clickListZone acts on a zone outside the player, the list having the
// focus.
func (m Model) clickListZone(z zone) (tea.Model, tea.Cmd) {
	if m.editor.mode != editClosed {
		if next, cmd, ok := m.clickEditorZone(z); ok {
			return next, cmd
		}
	}
	if row, ok := rowOf(z.id); ok {
		// Select the row, then act on it as enter does.
		m.setCursor(row)
		return m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	if row, ok := recentDeleteOf(z.id); ok {
		return m.deleteRecentAt(row)
	}
	switch z.id {
	case zoneInput:
		m.setCursor(-1)
		return m, m.input.Focus()
	case zoneRetry:
		return m.handleKey(tea.KeyPressMsg{Code: 'r', Text: keyRetry})
	case zoneTabStations:
		if m.onPlaylistsBranch() {
			// Back to the list from a library playlist page.
			m.popToRoot()
			return m, nil
		}
		// tab parks the search branch, from SEARCH and from its pages.
		return m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	case zoneTabSearch:
		return m.searchTab()
	case zoneBack:
		return m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	case zoneRowFavorite:
		return m.loveTarget()
	case zoneRowAdd:
		return m.addTarget()
	case zoneNewPlaylist:
		m.setStationCursor(-1)
		return m.openName(playback.Song{}, false)
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
