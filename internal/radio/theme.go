package radio

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Night City palette. The terminal's own background is left alone; only the
// selected row gets a dark fill.
var (
	colRed      = lipgloss.Color("#FF5F57")
	colRedDeep  = lipgloss.Color("#E8554E")
	colCyan     = lipgloss.Color("#5EF6FF")
	colYellow   = lipgloss.Color("#FCEE0A")
	colDim      = lipgloss.Color("#5A1E1E")
	colMuted    = lipgloss.Color("#9A3B37")
	colSelectBg = lipgloss.Color("#0E2A2F")
	colInk      = lipgloss.Color("#0A0A0A")
)

var (
	stRed      = lipgloss.NewStyle().Foreground(colRed)
	stRedBold  = stRed.Bold(true)
	stFrame    = lipgloss.NewStyle().Foreground(colRedDeep)
	stFrameDim = lipgloss.NewStyle().Foreground(colDim)
	stCyan     = lipgloss.NewStyle().Foreground(colCyan)
	stCyanBold = stCyan.Bold(true)
	stYellow   = lipgloss.NewStyle().Foreground(colYellow)
	stYellowB  = stYellow.Bold(true)
	stDim      = lipgloss.NewStyle().Foreground(colDim)
	stMuted    = lipgloss.NewStyle().Foreground(colMuted)
	stSelected = lipgloss.NewStyle().Foreground(colCyan).Background(colSelectBg).Bold(true)
	// stButtonOn fills the active button: dark ink on neon yellow.
	stButtonOn = lipgloss.NewStyle().Foreground(colInk).Background(colYellow).Bold(true)
)

func inputStyles() textinput.Styles {
	s := textinput.DefaultDarkStyles()
	s.Focused.Text = stCyan
	s.Focused.Placeholder = stDim
	s.Blurred.Text = stMuted
	s.Blurred.Placeholder = stDim
	s.Cursor.Color = colYellow
	return s
}

// spaced widens a label by putting a space between its characters and
// three between its words: "NOW PLAYING" -> "N O W   P L A Y I N G".
func spaced(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.Join(strings.Split(w, ""), " ")
	}
	return strings.Join(words, "   ")
}

// fit truncates a possibly styled string to w cells and pads it with spaces
// to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "")
	return s + strings.Repeat(" ", w-ansi.StringWidth(s))
}

// panel draws an angular frame with clipped top-left and bottom-right
// corners, a label in the top edge and a serial code in the bottom edge;
// an unfocused panel is dimmed, its ▮ hollowed to ▯:
//
//	╱─▮ PLAYLISTS ─────┐
//	│ ...              │
//	└──────── BAND FM ─╱
//
// The result is exactly w cells wide and h lines tall (nothing when either
// is too small to hold a frame).
func panel(label, code string, body []string, w, h int, focused bool) []string {
	if w < 6 || h < 2 {
		return nil
	}
	frame, lbl, mark := stFrameDim, stMuted, stMuted.Render("▯")
	if focused {
		frame, lbl, mark = stFrame, stRedBold, stYellow.Render("▮")
	}
	iw, ih := w-2, h-2

	head := frame.Render("╱─") + mark + " " + lbl.Render(label) + " "
	head = ansi.Truncate(head, w-1, "")
	head += frame.Render(strings.Repeat("─", max(w-1-ansi.StringWidth(head), 0)) + "┐")

	tail := " " + code + " ─"
	if ansi.StringWidth(tail) > iw {
		tail = ""
	}
	foot := frame.Render("└"+strings.Repeat("─", iw-ansi.StringWidth(tail))) + stMuted.Render(tail) + frame.Render("╱")

	// The side is styled once, not twice per row: panels are drawn on
	// every frame.
	side := frame.Render("│")
	out := make([]string, 0, h)
	out = append(out, head)
	for i := range ih {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		out = append(out, side+fit(line, iw)+side)
	}
	return append(out, foot)
}
