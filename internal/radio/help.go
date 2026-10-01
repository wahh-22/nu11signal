package radio

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The KEYS overlay lists every binding (helpGroups, in keys.go) in a panel
// over the whole body, the header and the footer left in place. keyHelp
// opens it wherever q quits, and keyHelp or esc closes it; while it is
// open every other key is ignored, q included, so a key pressed while
// reading never acts on the hidden view; ctrl+c still quits. A click
// closes it (see handleMouse).

// helpOverlayHints replace the view's hints while the KEYS overlay is
// open.
var helpOverlayHints = []hint{
	{"?/ESC", "CLOSE"},
	{"CTRL+C", "QUIT"},
}

// keysTyped reports whether keyHelp and q are typed rather than acting:
// on SEARCH, wherever the focus is (a key the tabs or the player leave
// goes to the input), and in the NEW PLAYLIST name. ADD TO PLAYLIST over
// SEARCH takes the keys itself.
func (m Model) keysTyped() bool {
	return m.editor.mode == editName || (m.editor.mode == editClosed && m.top().kind == viewSearch)
}

// helpKey handles a key press while the KEYS overlay is open, or keyHelp
// opening it; ok is false for every other key, which the view takes.
func (m Model) helpKey(k string) (next Model, ok bool) {
	switch {
	case m.help:
		if k == keyHelp || k == keyEsc {
			m.help = false
		}
		return m, true
	case k == keyHelp && !m.settings && m.auth != authFailed && !m.keysTyped():
		m.help = true
		return m, true
	}
	return m, false
}

// Layout of a help entry: a leading space, the keys in helpKeyWidth cells,
// a gap, then the label. helpColumnMin is the narrowest column that still
// shows a useful part of the labels.
const (
	helpKeyWidth  = 13
	helpColumnMin = 30
)

// helpPanel frames the KEYS overlay, w x h cells.
func (m Model) helpPanel(w, h int) []string {
	return panel("KEYS", "?/ESC CLOSE", helpBody(w-2, h-2), w, h, true)
}

// helpBody lays out helpGroups in w x h cells: each group, title then
// entries, flows down a column and starts the next column when it does
// not fit under the one before (a group taller than a column continues
// at the top of the next). There are as many columns as helpColumnMin
// allows; what does not fit them is left out, as in a narrow terminal.
func helpBody(w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	var cols [][]string
	for _, g := range helpGroups {
		block := []string{" " + stYellowB.Render("▞ "+g.name)}
		for _, e := range g.entries {
			block = append(block, " "+stYellow.Render(fit(e.show, helpKeyWidth))+" "+stRed.Render(e.label))
		}
		if len(cols) == 0 || len(cols[len(cols)-1])+len(block) > h {
			cols = append(cols, nil)
		}
		for _, line := range block {
			if len(cols[len(cols)-1]) >= h {
				cols = append(cols, nil)
			}
			cols[len(cols)-1] = append(cols[len(cols)-1], line)
		}
	}
	n := min(len(cols), max(w/helpColumnMin, 1))
	colW := w / n
	out := make([]string, h)
	for row := range out {
		var b strings.Builder
		for _, col := range cols[:n] {
			line := ""
			if row < len(col) {
				line = col[row]
			}
			b.WriteString(fit(line, colW))
		}
		out[row] = ansi.Truncate(b.String(), w, "")
	}
	return out
}
