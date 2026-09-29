package radio

import (
	tea "charm.land/bubbletea/v2"
)

// Keyboard focus is on one side of the screen: the list panel (the
// default) or the NOW PLAYING player. On the player, one control is
// selected: a transport button, or the progress bar above them. The
// expanded player hides the list, so it always has the focus; giving the
// focus back to the list restores it.

// focusArea is the side of the screen that takes the keys.
type focusArea int

const (
	focusList focusArea = iota
	focusPlayer
)

// playerControl is a button of the player, in the order ← and → walk them.
type playerControl int

const (
	ctlPrev playerControl = iota
	ctlPlay
	ctlNext
	ctlExpand
)

// focusPlayer moves the focus to control on the player. The search input
// stops showing its caret meanwhile.
func (m *Model) focusPlayer(control playerControl) {
	m.focus, m.control, m.onBar = focusPlayer, control, false
	m.input.Blur()
}

// focusList gives the focus back to the list, restoring the expanded
// player; on SEARCH the input takes the keys again.
func (m *Model) focusList() tea.Cmd {
	wasPlayer := m.focus == focusPlayer
	m.focus, m.onBar, m.expanded = focusList, false, false
	if wasPlayer && m.top().kind == viewSearch {
		return m.input.Focus()
	}
	return nil
}

// toggleExpand expands the player to the full width, focusing it, or
// restores it, focusing the list.
func (m Model) toggleExpand() (Model, tea.Cmd) {
	if m.expanded {
		return m, m.focusList()
	}
	if m.focus != focusPlayer {
		m.focusPlayer(ctlPlay)
	}
	m.expanded = true
	return m, nil
}

// handlePlayerKey handles a key while the player has the focus; ok is false
// for the keys it leaves to the list, which then takes the focus back.
func (m Model) handlePlayerKey(k string) (next tea.Model, cmd tea.Cmd, ok bool) {
	switch k {
	case keyLeft:
		if m.onBar {
			next, cmd = m.seek(-seekStep)
			return next, cmd, true
		}
		switch {
		case m.control > ctlPrev:
			m.control--
		case !m.expanded:
			cmd = m.focusList()
		}
	case keyRight:
		if m.onBar {
			next, cmd = m.seek(seekStep)
			return next, cmd, true
		}
		m.control = min(m.control+1, ctlExpand)
	case keyUp:
		m.onBar = m.onBar || m.seekable()
	case keyDown:
		m.onBar = false
	case keyEnter:
		if m.onBar {
			return m, nil, true
		}
		next, cmd = m.pressControl(m.control)
		return next, cmd, true
	case keyEsc:
		cmd = m.focusList()
	case keySpace:
		cmd = m.togglePlay()
	case keyNext:
		cmd = m.action("NEXT", m.player.Next)
	case keyPrev:
		cmd = m.action("PREV", m.player.Previous)
	case keySeekBack, keySeekBackAlt:
		next, cmd = m.seek(-seekStep)
		return next, cmd, true
	case keySeekForward, keySeekForwardAlt:
		next, cmd = m.seek(seekStep)
		return next, cmd, true
	case keyExpand, keyExpandAlt:
		next, cmd = m.toggleExpand()
		return next, cmd, true
	default:
		return m, nil, false
	}
	return m, cmd, true
}

// pressControl acts as a click on a player button, leaving the focus on
// it (or on the list, when restoring the player).
func (m Model) pressControl(c playerControl) (Model, tea.Cmd) {
	if c == ctlExpand {
		m.focusPlayer(ctlExpand)
		m.expanded = !m.expanded
		if !m.expanded {
			return m, m.focusList()
		}
		return m, nil
	}
	m.focusPlayer(c)
	switch c {
	case ctlPrev:
		return m, m.action("PREV", m.player.Previous)
	case ctlNext:
		return m, m.action("NEXT", m.player.Next)
	}
	return m, m.togglePlay()
}

// controlZone is the zone ID of a player button.
func controlZone(c playerControl) string {
	return [...]string{zonePrev, zonePlay, zoneNext, zoneExpand}[c]
}

// controlOf is the player button with zone ID id.
func controlOf(id string) (playerControl, bool) {
	for c := ctlPrev; c <= ctlExpand; c++ {
		if controlZone(c) == id {
			return c, true
		}
	}
	return 0, false
}

// focused reports whether the player has the focus on button c.
func (m Model) focused(c playerControl) bool {
	return m.focus == focusPlayer && !m.onBar && m.control == c
}

// barFocused reports whether the player has the focus on the progress bar.
func (m Model) barFocused() bool { return m.focus == focusPlayer && m.onBar }
