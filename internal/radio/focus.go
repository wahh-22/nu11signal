package radio

import (
	tea "charm.land/bubbletea/v2"
)

// Keyboard focus is on one area of the screen: the list panel (the
// default), the NOW PLAYING player, or the nav tabs in the header rule. On
// the player, one control is selected: a transport button, or the progress
// bar above them. The expanded player hides the list, so it keeps the
// focus; giving the focus back to the list restores it. ↑ past the top of
// the list (the SEARCH input included) or of the player reaches the tabs,
// and ↓ goes back where it came from.

// focusArea is the area of the screen that takes the keys.
type focusArea int

const (
	areaList focusArea = iota
	areaPlayer
	areaTabs
)

// playerControl is a button of the player, in the order ← and → walk them.
type playerControl int

const (
	ctlPrev playerControl = iota
	ctlPlay
	ctlNext
	ctlExpand
)

// focusPlayer moves the focus to control on the player. Coming from the
// list, the search input stops taking keys (and showing its caret)
// meanwhile; focusList gives them back as they were.
func (m *Model) focusPlayer(control playerControl) {
	if m.focus == areaList {
		m.inputHadFocus = m.input.Focused()
		m.input.Blur()
	}
	m.focus, m.control, m.onBar = areaPlayer, control, false
}

// focusList gives the focus back to the list as the player or the tabs
// found it, restoring the expanded player: the search input takes the keys
// again only if it had them and SEARCH is still the view on top.
func (m *Model) focusList() tea.Cmd {
	away := m.focus != areaList
	hadInput := m.inputHadFocus
	m.focus, m.onBar, m.expanded, m.inputHadFocus = areaList, false, false, false
	if away && hadInput && m.top().kind == viewSearch {
		return m.input.Focus()
	}
	return nil
}

// focusTabs moves the focus to the nav tabs, on the tab of the view shown,
// keeping the area it leaves for leaveTabs. Coming from the list, the
// search input stops taking keys meanwhile. A layout without tabs keeps
// the focus where it is.
func (m *Model) focusTabs() {
	n := len(m.tabIDs())
	if m.focus == areaTabs || n == 0 {
		return
	}
	if m.focus == areaList {
		m.inputHadFocus = m.input.Focused()
		m.input.Blur()
	}
	m.tabsFrom, m.focus = m.focus, areaTabs
	m.tab = 0
	if m.top().kind != viewStations {
		m.tab = min(1, n-1) // SEARCH is lit on the search branch
	}
}

// leaveTabs gives the focus back to the area the tabs took it from, as it
// was: the player on its button or bar, or the list on its row or input.
func (m *Model) leaveTabs() tea.Cmd {
	if m.tabsFrom == areaPlayer {
		m.focus = areaPlayer
		return nil
	}
	return m.focusList()
}

// tabIDs are the zone IDs of the nav tabs drawn, left to right: STATIONS,
// SEARCH and, on a page, BACK, as many as the width holds.
func (m Model) tabIDs() []string {
	_, zs := m.layout()
	var ids []string
	for _, z := range zs {
		switch z.id {
		case zoneTabStations, zoneTabSearch, zoneBack:
			ids = append(ids, z.id)
		}
	}
	return ids
}

// handleTabsKey handles a key while the nav tabs have the focus; ok is
// false for the keys it leaves to the area the focus came from, which then
// takes it back.
func (m Model) handleTabsKey(k string) (next tea.Model, cmd tea.Cmd, ok bool) {
	ids := m.tabIDs()
	switch k {
	case keyLeft:
		m.tab = max(m.tab-1, 0)
	case keyRight:
		m.tab = max(min(m.tab+1, len(ids)-1), 0)
	case keyUp:
	case keyDown, keyEsc:
		cmd = m.leaveTabs()
	case keyEnter:
		// As a click on the tab: the list takes the focus.
		if m.tab >= len(ids) {
			return m, m.leaveTabs(), true
		}
		focus := m.focusList()
		next, cmd = m.clickListZone(zone{id: ids[m.tab]})
		return next, tea.Batch(focus, cmd), true
	default:
		return m, nil, false
	}
	return m, cmd, true
}

// toggleExpand is the one expand toggle, for the keys and the EXPAND
// button alike. Expanding focuses the player on the control it already
// has (PLAY when the list had the focus; EXPAND once its button is
// pressed); restoring gives the focus back to the list.
func (m Model) toggleExpand() (Model, tea.Cmd) {
	if m.expanded {
		return m, m.focusList()
	}
	if m.focus != areaPlayer {
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
		// Up from the buttons reaches the bar, if there is one to seek
		// in; up from there, the tabs.
		if !m.onBar && m.seekable() {
			m.onBar = true
		} else {
			m.focusTabs()
		}
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
	m.focusPlayer(c)
	switch c {
	case ctlExpand:
		return m.toggleExpand()
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
	return m.focus == areaPlayer && !m.onBar && m.control == c
}

// barFocused reports whether the player has the focus on the progress bar.
func (m Model) barFocused() bool { return m.focus == areaPlayer && m.onBar }
