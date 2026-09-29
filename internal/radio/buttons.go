package radio

import (
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
}

func (b button) width() int { return ansi.StringWidth(b.label) + 4 }

func (b button) render() string {
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
// BACK (esc) above the stations root.
func (m Model) navButtons() []button {
	onStations := m.top().kind == viewStations
	bs := []button{
		{id: zoneTabStations, label: "STATIONS", tone: stRed, active: onStations},
		{id: zoneTabSearch, label: "SEARCH", tone: stRed, active: !onStations},
	}
	if len(m.stack) > 1 {
		bs = append(bs, button{id: zoneBack, label: "◀ BACK", tone: stYellow})
	}
	return bs
}

// transportBar lays out PREV, PLAY or PAUSE, and NEXT in at most w cells:
// labelled while they fit, else glyphs only, else as many as fit.
func (m Model) transportBar(w int) (string, zones) {
	full := []string{"◀◀ PREV", "▶ PLAY", "NEXT ▶▶"}
	short := []string{"◀◀", "▶", "▶▶"}
	if m.isPlaying() {
		full[1], short[1] = "❚❚ PAUSE", "❚❚"
	}
	if bar, zs := buttonBar(transportButtons(full), w); len(zs) == len(full) {
		return bar, zs
	}
	return buttonBar(transportButtons(short), w)
}

func transportButtons(labels []string) []button {
	ids := []string{zonePrev, zonePlay, zoneNext}
	bs := make([]button, len(labels))
	for i, l := range labels {
		bs[i] = button{id: ids[i], label: l, tone: stCyan}
	}
	return bs
}
