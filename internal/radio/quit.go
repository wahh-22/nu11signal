package radio

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The quit modal asks before nu11signal closes, so a stray q never ends
// the session. keyQuit, wherever it acted (not where typing takes it, see
// keysTyped, nor under the KEYS or SETTINGS overlay, which ignore it),
// keyCtrlC anywhere, and q or esc on the auth error screen open it: a
// small panel centered over whatever is on screen, the playback going on
// under it. y or enter quit, and so do q or ctrl+c pressed again (the
// double press is the fast way out); n or esc close it, giving the screen
// back as it was (a confirmed quit closes it for the shutdown splash, see
// shutdown.go). Every other key is ignored. Its QUIT and STAY buttons
// are HUD bracket keys like the player's transport (see button): QUIT,
// enter's action, filled as the primary one, STAY in the transport's
// cyan brackets. They are clickable; a click on the panel off them does
// nothing, anywhere else it closes the modal. It scrambles in and out
// like the overlays (see introRegions and withIntro). On the tiny layout
// there is no room for the panel: the question takes the status tag's
// line.
//
//	╱─▮ QUIT ──────────────────────┐
//	│                              │
//	│       QUIT NU11SIGNAL?       │
//	│                              │
//	│    [ Y QUIT ]  [ N STAY ]    │
//	└──────────────────── CONFIRM ─╱

// The modal's own keys, beside keyEnter, keyEsc, keyQuit and keyCtrlC;
// either case acts.
const (
	quitYes = "y"
	quitNo  = "n"
)

// quitModalWidth is the widest the modal is, frame included: its two
// buttons side by side, quitButtonGap apart, with a margin. A screen too
// narrow for them side by side stacks them.
const (
	quitModalWidth = 32
	quitButtonGap  = 2
)

// zoneQuitPanel covers the quit modal, frame included, under its buttons:
// a click there does nothing, and the content intro finds the modal's
// inside by it. zoneQuitYes and zoneQuitNo are its QUIT and STAY buttons.
const (
	zoneQuitPanel = "quit:panel"
	zoneQuitYes   = "quit:yes"
	zoneQuitNo    = "quit:no"
)

// quitModalHints replace every other footer while the modal asks.
var quitModalHints = []hint{
	{"Y/ENTER", "QUIT"},
	{"N/ESC", "STAY"},
}

// askQuit opens the quit modal.
func (m Model) askQuit() Model {
	m.quitAsk = true
	return m
}

// quitKey handles a key press while the quit modal asks: the quit path
// (the shutdown, see startShutdown) for yes, closed for no, nothing for
// any other key.
func (m Model) quitKey(k string) (Model, tea.Cmd) {
	switch strings.ToLower(k) {
	case quitYes, keyEnter, keyQuit, keyCtrlC:
		return m.startShutdown()
	case quitNo, keyEsc:
		m.quitAsk = false
	}
	return m, nil
}

// quitClick handles a left press while the quit modal asks: its buttons
// answer, the panel off them does nothing, anywhere else closes it.
func (m Model) quitClick(x, y int) (Model, tea.Cmd) {
	_, zs := m.layout()
	z, ok := zs.at(x, y)
	switch {
	case !ok:
		m.quitAsk = false
	case z.id == zoneQuitYes:
		return m.startShutdown()
	case z.id == zoneQuitNo:
		m.quitAsk = false
	}
	return m, nil
}

// quitButtons are the modal's QUIT and STAY buttons, HUD bracket keys
// (see button.render): QUIT active, filled in the primary fill; STAY in
// stHi, the transport buttons' cyan. A function, so the tone follows the
// theme applied.
func quitButtons() []button {
	return []button{
		{id: zoneQuitYes, label: " Y QUIT ", bracket: true, active: true},
		{id: zoneQuitNo, label: " N STAY ", tone: stHi, bracket: true},
	}
}

// quitPanel frames the quit modal in at most w x h cells, with the zones
// of its buttons and of the whole panel relative to it: the buttons side
// by side when they fit, else one above the other, and the blank rows
// around the question left out when the screen is too short for them.
func quitPanel(w, h int) ([]string, zones) {
	mw := min(w, quitModalWidth)
	iw := mw - 2
	var rows []string
	var rowZones []zones
	bs := quitButtons()
	if gaps := []int{0, quitButtonGap}; hudWidth(bs, gaps) <= iw-2 {
		bar, bz := hudRow(bs, gaps)
		rows, rowZones = []string{bar}, []zones{bz}
	} else {
		for _, b := range bs {
			bar, bz := buttonBar([]button{b}, iw)
			rows, rowZones = append(rows, bar), append(rowZones, bz)
		}
	}
	question := centered(stLabelBold.Render("QUIT NU11SIGNAL?"), iw)
	body := []string{"", question, ""}
	if len(body)+len(rows)+2 > h {
		body = []string{question}
	}
	var zs zones
	for i, row := range rows {
		pad := max((iw-ansi.StringWidth(row))/2, 0)
		zs.addAt(1+pad, 1+len(body), rowZones[i])
		body = append(body, strings.Repeat(" ", pad)+row)
	}
	mh := min(len(body)+2, h)
	var out zones
	out.addBox(zoneQuitPanel, 0, 0, mw, mh)
	out = append(out, zs...)
	return panel("QUIT", "CONFIRM", body, mw, mh, true), out.clip(mw, mh)
}

// centered pads the styled s on the left to center it in w cells.
func centered(s string, w int) string {
	return strings.Repeat(" ", max((w-ansi.StringWidth(s))/2, 0)) + s
}

// withQuitModal draws the quit modal centered over lines, the frame of a
// w x h screen, and returns its zones in place of the frame's: nothing
// under the modal is clickable. The tiny layout gets the question alone.
func (m Model) withQuitModal(lines []string) ([]string, zones) {
	w, h := m.width, m.height
	if w < tinyMinWidth || h < tinyMinHeight {
		return []string{m.wordmark(), stWarnBold.Render("QUIT? Y/N")}, nil
	}
	box, bz := quitPanel(w, h)
	if len(box) == 0 {
		return lines, nil
	}
	x, y := (w-ansi.StringWidth(box[0]))/2, (h-len(box))/2
	for len(lines) < y+len(box) {
		lines = append(lines, "")
	}
	for i, row := range box {
		lines[y+i] = drawOver(lines[y+i], x, row, w)
	}
	return lines, bz.shifted(x, y)
}

// drawOver writes the styled s over line from cell x, in a line of
// exactly w cells: line is padded first, and a wide character cut at
// either edge of s gives way to spaces (overlay, for the effects, keeps
// such a line as it was instead).
func drawOver(line string, x int, s string, w int) string {
	sw := ansi.StringWidth(s)
	return fit(fit(ansi.Truncate(line, x, ""), x)+ansi.ResetStyle+s+ansi.ResetStyle+
		ansi.TruncateLeft(fit(line, w), x+sw, ""), w)
}
