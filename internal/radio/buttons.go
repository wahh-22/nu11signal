package radio

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// button is one clickable control drawn as a slanted neon plate, or, on
// the player (bracket), as a bracketed HUD key:
//
//	╱ PLAYLISTS ╱  (the active one fills in yellow: ╱█SEARCH█╱)
//	[◀◀]           (the active one fills in yellow: █[ ❚❚ PAUSE ]█)
type button struct {
	id    string
	label string
	// tone colors the plate; an active button is filled in yellow instead.
	tone   lipgloss.Style
	active bool
	// focused marks the button the keyboard is on: filled in yellow, with
	// a ▸ before its label (on a HUD key, in its leading space or else in
	// place of its opening bracket: [▸❚❚ PAUSE ], ▸◀◀]).
	focused bool
	bracket bool
}

func (b button) width() int {
	if b.bracket {
		return ansi.StringWidth(b.label) + 2
	}
	return ansi.StringWidth(b.label) + 4
}

func (b button) render() string {
	if b.bracket {
		switch {
		case b.focused && strings.HasPrefix(b.label, " "):
			return stButtonOn.Render("[▸" + b.label[1:] + "]")
		case b.focused:
			return stButtonOn.Render("▸" + b.label + "]")
		case b.active:
			return stButtonOn.Render("[" + b.label + "]")
		}
		return b.tone.Render("[" + b.label + "]")
	}
	if b.focused {
		return stYellow.Render("╱") + stButtonOn.Render("▸"+b.label+" ") + stYellow.Render("╱")
	}
	if b.active {
		return stYellow.Render("╱") + stButtonOn.Render(" "+b.label+" ") + stYellow.Render("╱")
	}
	return b.tone.Render("╱ " + b.label + " ╱")
}

// buttonBar lays buttons out left to right, one cell apart, in at most w
// cells; the buttons past the first that does not fit are left out. The
// zones are in the bar's coordinates.
func buttonBar(bs []button, w int) (string, zones) {
	var out strings.Builder
	var zs zones
	x := 0
	for _, b := range bs {
		gap := 0
		if x > 0 {
			gap = 1
		}
		if x+gap+b.width() > w {
			break
		}
		out.WriteString(strings.Repeat(" ", gap) + b.render())
		zs.add(b.id, x+gap, 0, b.width())
		x += gap + b.width()
	}
	return out.String(), zs
}

// navButtons are the PLAYLISTS and SEARCH tabs, the one of the branch
// shown lit, and BACK (esc) while a page is on top of a root view (the
// playlists or the SEARCH base) or the library editor is open. The tab the
// keyboard is on is marked. Every one is a tab the keyboard reaches: a
// control the mouse alone could reach does not belong here (+ NEW
// PLAYLIST is the row over the playlists, for both).
func (m Model) navButtons() []button {
	lit := m.litTab()
	bs := []button{
		{id: zoneTabStations, label: "PLAYLISTS", tone: stRed, active: lit == zoneTabStations},
		{id: zoneTabSearch, label: "SEARCH", tone: stRed, active: lit == zoneTabSearch},
	}
	if isPage(m.top().kind) || m.editor.mode != editClosed {
		bs = append(bs, button{id: zoneBack, label: "◀ BACK", tone: stYellow})
	}
	if m.focus == areaTabs && m.tab < len(bs) {
		bs[m.tab].focused = true
	}
	return bs
}

// hudLayout is one way to lay out the transport row: how PLAY or PAUSE
// is labelled, the gaps between PREV, PLAY and NEXT (near) and before LOOP
// (loop), the least gap before EXPAND, and whether LOOP names its mode.
type hudLayout struct {
	play               hudPlay
	near, loop, expand int
	loopMode           bool
}

// hudPlay is how PLAY or PAUSE is labelled: [ ❚❚ PAUSE ], [ ❚❚ ] or [❚❚].
type hudPlay int

const (
	playLabelled hudPlay = iota
	playSpaced
	playBare
)

// hudLayouts are the transport row's layouts, widest first: the PLAY
// label goes first, then the room around the buttons, then LOOP's mode;
// past the last one, the buttons at the end give way.
var hudLayouts = []hudLayout{
	{playLabelled, 2, 6, 2, true},
	{playSpaced, 2, 6, 2, true},
	{playSpaced, 1, 3, 1, true},
	{playBare, 1, 1, 1, true},
	{playBare, 1, 1, 1, false},
}

// transportBar lays out the transport row in at most w cells, with its
// zones: PREV, PLAY or PAUSE and NEXT close together, LOOP after a wider
// gap, and EXPAND or RESTORE at the right edge (right after LOOP when
// packed, as the compact layout draws it with more beside it). It takes
// the widest of hudLayouts that fits, else the narrowest with as many
// buttons as fit; a button is always drawn whole. The button the keyboard
// is on is marked.
//
//	[◀◀]  [ ❚❚ PAUSE ]  [▶▶]      [↻ OFF]              [⤢]
func (m Model) transportBar(w int, packed bool) (string, zones) {
	for _, l := range hudLayouts {
		bs, gaps := m.transportButtons(l)
		if hudWidth(bs, gaps) <= w {
			if !packed {
				// EXPAND takes the room left.
				last := len(gaps) - 1
				gaps[last] += w - hudWidth(bs, gaps)
			}
			return hudRow(bs, gaps)
		}
	}
	bs, gaps := m.transportButtons(hudLayouts[len(hudLayouts)-1])
	for len(bs) > 0 && hudWidth(bs, gaps) > w {
		bs, gaps = bs[:len(bs)-1], gaps[:len(gaps)-1]
	}
	return hudRow(bs, gaps)
}

// transportButtons are the transport row's buttons in playerControl
// order, PREV to EXPAND, labelled as l says, and the gap before each.
func (m Model) transportButtons(l hudLayout) ([]button, []int) {
	plays := [...]string{playLabelled: " ▶ PLAY ", playSpaced: " ▶ ", playBare: "▶"}
	if m.isPlaying() {
		plays = [...]string{playLabelled: " ❚❚ PAUSE ", playSpaced: " ❚❚ ", playBare: "❚❚"}
	}
	loop := "↻"
	if l.loopMode {
		loop += " " + loopName(m.loopMode())
	}
	expand := "⤢"
	if m.expanded {
		expand = "⤡"
	}
	labels := [...]string{ctlPrev: "◀◀", ctlPlay: plays[l.play], ctlNext: "▶▶", ctlLoop: loop, ctlExpand: expand}
	bs := make([]button, len(labels))
	for i, label := range labels {
		c := playerControl(i)
		bs[i] = button{id: controlZone(c), label: label, tone: stCyan, bracket: true, focused: m.focused(c)}
	}
	// PLAY or PAUSE is the primary action, always filled; EXPAND stands
	// apart from the transport controls.
	bs[ctlPlay].active = true
	bs[ctlExpand].tone = stYellow
	return bs, []int{0, l.near, l.near, l.loop, l.expand}
}

// hudWidth is the width of buttons bs laid out after gaps.
func hudWidth(bs []button, gaps []int) int {
	w := 0
	for i, b := range bs {
		w += gaps[i] + b.width()
	}
	return w
}

// hudRow draws buttons bs, each after its gap, with their zones in the
// row's coordinates.
func hudRow(bs []button, gaps []int) (string, zones) {
	var out strings.Builder
	var zs zones
	x := 0
	for i, b := range bs {
		out.WriteString(strings.Repeat(" ", gaps[i]) + b.render())
		zs.add(b.id, x+gaps[i], 0, b.width())
		x += gaps[i] + b.width()
	}
	return out.String(), zs
}

// The player's controls take one row when it holds them all, the volume
// between LOOP and EXPAND with a meter of at least hudMeterMin cells:
//
//	[◀◀]  [ ❚❚ PAUSE ]  [▶▶]  [↻ OFF]  VOL [−] ▮▮▮▮▮▮▮▮ [+] 90%  [⤢]
//
// and else two, the transport row over the volume row. hudRowCount alone
// decides it for a width, so the drawing (hudControls), the rows NOW
// PLAYING counts (nowPlayingControlRows) and the zones the focus walks
// always agree. The row is laid out for PAUSE, the wider label, so the
// volume and EXPAND stay put when the song pauses.
const (
	hudMeterMin = 4
	hudGap      = 2
	// hudTransportMax is PREV, PAUSE, NEXT and LOOP, hudGap apart.
	hudTransportMax = 4 + hudGap + 12 + hudGap + 4 + hudGap + 7
	// hudVolumeBare is the volume row without its meter: the label, [−],
	// [+], the percentage and the spaces between them (see volumeBar).
	hudVolumeBare = 4 + 3 + 1 + 1 + 3 + 1 + volumePctWidth
	// hudExpandWidth is EXPAND or RESTORE.
	hudExpandWidth = 3
	// hudOneRowMin is the narrowest row that holds every control.
	hudOneRowMin = hudTransportMax + hudGap + hudVolumeBare + hudMeterMin + hudGap + hudExpandWidth
)

// hudRowCount is how many rows the player's controls take in w cells: 1
// from hudOneRowMin on, else 2.
func hudRowCount(w int) int {
	if w >= hudOneRowMin {
		return 1
	}
	return 2
}

// hudControls draws the player's controls in w cells, one row or two as
// hudRowCount says, with their zones (y the row among them). In one row
// the zones are in walking order: PREV, PLAY, NEXT, LOOP, VOL−, VOL+ and
// EXPAND at the right edge.
func (m Model) hudControls(w int) ([]string, zones) {
	if hudRowCount(w) == 2 {
		transport, zs := m.transportBar(w, false)
		volume, vz := m.volumeBar(w)
		zs = append(zs, vz.shifted(0, 1)...)
		return []string{transport, volume}, zs
	}
	bs, gaps := m.transportButtons(hudLayout{play: playLabelled, near: hudGap, loop: hudGap, loopMode: true})
	row, zs := hudRow(bs[:ctlExpand], gaps[:ctlExpand])
	volX := hudTransportMax + hudGap
	volW := min(w-volX-hudGap-hudExpandWidth, hudVolumeBare+1+volumeMeterMax)
	volume, vz := m.volumeBar(volW)
	zs.addAt(volX, 0, vz)
	row += strings.Repeat(" ", volX-ansi.StringWidth(row)) + volume
	expand := bs[ctlExpand]
	expandX := w - expand.width()
	zs.add(expand.id, expandX, 0, expand.width())
	row += strings.Repeat(" ", expandX-ansi.StringWidth(row)) + expand.render()
	return []string{row}, zs
}
