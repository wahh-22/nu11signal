package radio

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// button is one clickable control drawn as a slanted neon plate:
//
//	╱ STATIONS ╱  (the active one fills in yellow: ╱█SEARCH█╱)
type button struct {
	id    string
	label string
	// tone colors the plate; an active button is filled in yellow instead.
	tone   lipgloss.Style
	active bool
	// focused marks the button the keyboard is on: filled in yellow, with
	// a ▸ before its label.
	focused bool
}

func (b button) width() int { return ansi.StringWidth(b.label) + 4 }

func (b button) render() string {
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

// navButtons are the STATIONS and SEARCH tabs, the one showing lit, and
// BACK (esc) while a page is on top of a root view (the stations or the
// SEARCH base). The tab the keyboard is on is marked.
func (m Model) navButtons() []button {
	onStations := m.top().kind == viewStations
	bs := []button{
		{id: zoneTabStations, label: "STATIONS", tone: stRed, active: onStations},
		{id: zoneTabSearch, label: "SEARCH", tone: stRed, active: !onStations},
	}
	if isPage(m.top().kind) {
		bs = append(bs, button{id: zoneBack, label: "◀ BACK", tone: stYellow})
	}
	if m.focus == areaTabs && m.tab < len(bs) {
		bs[m.tab].focused = true
	}
	return bs
}

// transportBar lays out PREV, PLAY or PAUSE, NEXT and EXPAND or RESTORE in
// at most w cells: labelled while they fit, else EXPAND as a glyph, else
// glyphs only, else as many as fit. The button the keyboard is on is marked.
func (m Model) transportBar(w int) (string, zones) {
	full := []string{"◀◀ PREV", "▶ PLAY", "NEXT ▶▶", "⤢ EXPAND"}
	short := []string{"◀◀", "▶", "▶▶", "⤢"}
	if m.isPlaying() {
		full[ctlPlay], short[ctlPlay] = "❚❚ PAUSE", "❚❚"
	}
	if m.expanded {
		full[ctlExpand], short[ctlExpand] = "⤡ RESTORE", "⤡"
	}
	// EXPAND gives up its label first.
	mixed := append(slices.Clone(full[:ctlExpand]), short[ctlExpand])
	for _, labels := range [][]string{full, mixed} {
		if bar, zs := buttonBar(m.transportButtons(labels), w); len(zs) == len(labels) {
			return bar, zs
		}
	}
	return buttonBar(m.transportButtons(short), w)
}

// transportButtons are the player buttons with labels, in playerControl
// order.
func (m Model) transportButtons(labels []string) []button {
	bs := make([]button, len(labels))
	for i, l := range labels {
		c := playerControl(i)
		tone := stCyan
		if c == ctlExpand {
			// EXPAND stands apart from the transport controls.
			tone = stYellow
		}
		bs[i] = button{id: controlZone(c), label: l, tone: tone, focused: m.focused(c)}
	}
	return bs
}
