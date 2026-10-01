package radio

import (
	"slices"

	tea "charm.land/bubbletea/v2"
)

// Keyboard focus is on one area of the screen: the list panel (the
// default), the NOW PLAYING player, or the nav tabs in the header rule. On
// the player, one control is selected: a transport button, the progress
// bar above them, or a volume button below them. The expanded player hides
// the list, so it keeps the focus; giving the focus back to the list
// restores it. ↑ past the top of the list (the SEARCH input included) or
// of the player reaches the tabs, and ↓ goes back where it came from.

// focusArea is the area of the screen that takes the keys.
type focusArea int

const (
	areaList focusArea = iota
	areaPlayer
	areaTabs
)

// playerControl is a button of the player. The buttons sit in rows, as
// the frame draws them (see hudControls): the favorite button of the song
// playing alone over the progress bar, then, under the bar, PREV, PLAY,
// NEXT, LOOP, VOL-, VOL+ and EXPAND on one row when the width holds them,
// else the transport row, PREV to EXPAND, over the volume row, VOL- and
// VOL+. ← and → walk the buttons of a row as drawn, left to right; ↑ and ↓
// cross between rows.
type playerControl int

const (
	ctlPrev playerControl = iota
	ctlPlay
	ctlNext
	ctlLoop
	ctlExpand
	ctlVolDown
	ctlVolUp
	ctlFav
)

// onVolumeRow reports whether c is a volume button.
func (c playerControl) onVolumeRow() bool { return c == ctlVolDown || c == ctlVolUp }

// below is the volume button under transport button c: VOL- under PREV
// and PLAY, VOL+ under NEXT, LOOP and EXPAND. above goes back up, VOL- to
// PREV and VOL+ to NEXT.
func (c playerControl) below() playerControl {
	if c <= ctlPlay {
		return ctlVolDown
	}
	return ctlVolUp
}

func (c playerControl) above() playerControl {
	if c == ctlVolDown {
		return ctlPrev
	}
	return ctlNext
}

// besideControl is the player button drawn next to c on its row: the
// nearest one to its left (dx -1) or right (dx +1). ok is false at the end
// of the row, or when c is not drawn.
func (m Model) besideControl(c playerControl, dx int) (next playerControl, ok bool) {
	_, zs := m.layout()
	at, drawn := zs.find(controlZone(c))
	if !drawn {
		return c, false
	}
	nearX := 0
	for _, z := range zs {
		n, isControl := controlOf(z.id)
		if !isControl || z.y != at.y || (z.x-at.x)*dx <= 0 {
			continue
		}
		if !ok || (z.x-nearX)*dx < 0 {
			next, nearX, ok = n, z.x, true
		}
	}
	return next, ok
}

// volumeBelow reports whether the volume buttons are drawn on a row of
// their own under the transport row (not beside it, as one row).
func (m Model) volumeBelow() bool {
	_, zs := m.layout()
	down, ok := zs.find(zoneVolDown)
	prev, okPrev := zs.find(zonePrev)
	return ok && okPrev && down.y > prev.y
}

// focusPlayer moves the focus to control on the player. Coming from the
// list, the search input (or the NEW PLAYLIST name) stops taking keys (and
// showing its caret) meanwhile; focusList gives them back as they were.
func (m *Model) focusPlayer(control playerControl) {
	if m.focus == areaList {
		m.inputHadFocus = m.input.Focused()
		m.input.Blur()
		m.nameInput.Blur()
	}
	m.focus, m.control, m.onBar = areaPlayer, control, false
}

// focusList gives the focus back to the list as the player or the tabs
// found it, restoring the expanded player: the search input takes the keys
// again only if it had them and SEARCH is still the view on top, under no
// editor; the NEW PLAYLIST name always does.
func (m *Model) focusList() tea.Cmd {
	away := m.focus != areaList
	hadInput := m.inputHadFocus
	m.focus, m.onBar, m.expanded, m.inputHadFocus = areaList, false, false, false
	switch {
	case away && m.editor.mode == editName:
		return m.nameInput.Focus()
	case away && hadInput && m.top().kind == viewSearch && m.editor.mode == editClosed:
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
		m.nameInput.Blur()
	}
	m.tabsFrom, m.focus = m.focus, areaTabs
	m.tab = max(slices.Index(m.tabIDs(), m.litTab()), 0)
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

// tabIDs are the zone IDs of the nav tabs drawn, left to right: PLAYLISTS,
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
		// Nothing is above the tabs: they keep the focus.
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
		// The player keys act here, keeping the focus on the tabs; on
		// SEARCH they would otherwise be typed in the input.
		return m.playerKey(k)
	}
	return m, cmd, true
}

// playerKey handles the keys that drive the player from wherever the focus
// is, but for the SEARCH input, which types them: play/pause, next and
// previous, seek, volume and loop. ok is false for any other key.
func (m Model) playerKey(k string) (next Model, cmd tea.Cmd, ok bool) {
	switch k {
	case keySpace:
		return m, m.togglePlay(), true
	case keyNext:
		return m, m.action("NEXT", m.player.Next), true
	case keyPrev:
		return m, m.action("PREV", m.player.Previous), true
	case keySeekBack, keySeekBackAlt:
		next, cmd = m.seek(-seekStep)
		return next, cmd, true
	case keySeekForward, keySeekForwardAlt:
		next, cmd = m.seek(seekStep)
		return next, cmd, true
	case keyVolumeUp, keyVolumeUpAlt, keyVolumeUpLetter, keyVolumeUpAnywhere:
		next, cmd = m.stepVolume(volumeStep)
		return next, cmd, true
	case keyVolumeDown, keyVolumeDownLetter, keyVolumeDownAnywhere:
		next, cmd = m.stepVolume(-volumeStep)
		return next, cmd, true
	case keyLoop:
		next, cmd = m.cycleLoop()
		return next, cmd, true
	}
	return m, nil, false
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
		switch prev, ok := m.besideControl(m.control, -1); {
		case ok:
			m.control = prev
		case !m.expanded:
			cmd = m.focusList()
		}
	case keyRight:
		if m.onBar {
			next, cmd = m.seek(seekStep)
			return next, cmd, true
		}
		// The row stops at its last button drawn: a narrow row leaves the
		// last ones out.
		if next, ok := m.besideControl(m.control, 1); ok {
			m.control = next
		}
	case keyUp:
		// Up from the volume row reaches the transport row; from there (or
		// from the one row of controls), the bar, if there is one to seek
		// in; from there, the favorite of the song playing, if there is
		// one; from there, the tabs.
		lower := m.control.onVolumeRow() && m.volumeBelow()
		switch {
		case m.control == ctlFav:
			m.focusTabs()
		case (m.onBar || (!lower && !m.seekable())) && m.drawn(zoneFavPlaying):
			// From the bar, or from the transport row with no bar.
			m.favFrom = m.control
			m.control, m.onBar = ctlFav, false
		case m.onBar:
			m.focusTabs()
		case lower:
			m.control = m.control.above()
		case m.seekable():
			m.onBar = true
		default:
			m.focusTabs()
		}
	case keyDown:
		switch {
		case m.control == ctlFav:
			// Down to the bar, or with nothing to seek to the transport
			// button ↑ left (PLAY after a click on the favorite).
			m.control, m.onBar = m.favFrom, m.seekable()
		case m.onBar:
			m.onBar = false
		case !m.control.onVolumeRow() && m.volumeBelow():
			// The volume row is the bottom one; one row of controls has
			// none under it, and a narrow compact layout leaves it out.
			m.control = m.control.below()
		}
	case keyEnter:
		if m.onBar {
			return m, nil, true
		}
		next, cmd = m.pressControl(m.control)
		return next, cmd, true
	case keyEsc:
		cmd = m.focusList()
	case keyLove:
		// The song playing, the focus staying here.
		next, cmd = m.loveTarget()
		return next, cmd, true
	case keyAdd:
		// The song playing, in the picker, which takes the focus.
		next, cmd = m.addTarget()
		return next, cmd, true
	case keyExpand, keyExpandAlt:
		next, cmd = m.toggleExpand()
		return next, cmd, true
	default:
		return m.playerKey(k)
	}
	return m, cmd, true
}

// pressControl acts as a click on a player button, leaving the focus on
// it (or on the list, when restoring the player).
func (m Model) pressControl(c playerControl) (Model, tea.Cmd) {
	if c == ctlFav && !m.focused(ctlFav) {
		m.favFrom = ctlPlay // what ↓ from the favorite reaches
	}
	m.focusPlayer(c)
	switch c {
	case ctlExpand:
		return m.toggleExpand()
	case ctlPrev:
		return m, m.action("PREV", m.player.Previous)
	case ctlNext:
		return m, m.action("NEXT", m.player.Next)
	case ctlVolDown:
		return m.stepVolume(-volumeStep)
	case ctlVolUp:
		return m.stepVolume(volumeStep)
	case ctlLoop:
		return m.cycleLoop()
	case ctlFav:
		return m.loveTarget()
	}
	return m, m.togglePlay()
}

// controlZone is the zone ID of a player button.
func controlZone(c playerControl) string {
	return [...]string{zonePrev, zonePlay, zoneNext, zoneLoop, zoneExpand, zoneVolDown, zoneVolUp, zoneFavPlaying}[c]
}

// controlOf is the player button with zone ID id.
func controlOf(id string) (playerControl, bool) {
	for c := ctlPrev; c <= ctlFav; c++ {
		if controlZone(c) == id {
			return c, true
		}
	}
	return 0, false
}

// drawn reports whether the frame on screen has a zone with id.
func (m Model) drawn(id string) bool {
	_, zs := m.layout()
	_, ok := zs.find(id)
	return ok
}

// focused reports whether the player has the focus on button c.
func (m Model) focused(c playerControl) bool {
	return m.focus == areaPlayer && !m.onBar && m.control == c
}

// barFocused reports whether the player has the focus on the progress bar.
func (m Model) barFocused() bool { return m.focus == areaPlayer && m.onBar }
