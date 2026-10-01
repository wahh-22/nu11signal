package radio

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A theme is the palette every style of the UI is drawn from, by role.
// The terminal's own background is left alone; only the selected row gets
// a dark fill.
type theme struct {
	// name is the theme as SETTINGS lists it and config.json stores it.
	name string
	// red draws titles, labels and hints; redDeep the focused frames;
	// cyan and yellow the highlights (yellow also the active fills);
	// dim and muted what sits behind; selectBg fills the selected row and
	// ink is the text on a fill; alert draws what must read as danger
	// (the NO SIGNAL sign).
	red, redDeep, cyan, yellow, dim, muted, selectBg, ink, alert string
}

// themes are the themes SETTINGS offers, the default first.
var themes = []theme{
	{
		name: "NIGHT CITY",
		red:  "#FF5F57", redDeep: "#E8554E", cyan: "#5EF6FF", yellow: "#FCEE0A",
		dim: "#5A1E1E", muted: "#9A3B37", selectBg: "#0E2A2F", ink: "#0A0A0A",
		alert: "#FF5F57",
	},
	{
		// BLUE is the gentleman-blue palette.
		name: "BLUE",
		red:  "#347AFF", redDeep: "#2A62CC", cyan: "#5CE1FF", yellow: "#FFD23D",
		dim: "#1C2C54", muted: "#4A5578", selectBg: "#10182E", ink: "#05070F",
		alert: "#FF3D81",
	},
}

// themeNamed is the theme called name, case aside.
func themeNamed(name string) (theme, bool) {
	for _, t := range themes {
		if strings.EqualFold(t.name, strings.TrimSpace(name)) {
			return t, true
		}
	}
	return theme{}, false
}

// The palette and the styles drawn from it are package state, rebuilt by
// applyTheme: every renderer reads them, and threading a theme through
// all of them would touch nearly every function of the package for a
// value that changes only when SETTINGS applies one. Only Update (the
// program's event loop, which also draws) and New change it; tests run
// one at a time (none calls t.Parallel) and put NIGHT CITY back after
// changing it.
var (
	colRed, colRedDeep, colCyan, colYellow  color.Color
	colDim, colMuted, colSelectBg, colInk   color.Color
	colAlert                                color.Color
	stRed, stRedBold, stFrame, stFrameDim   lipgloss.Style
	stCyan, stCyanBold, stYellow, stYellowB lipgloss.Style
	stDim, stMuted, stSelected, stAlert     lipgloss.Style
	stAlertBold                             lipgloss.Style
	// stButtonOn fills the active button: dark ink on the yellow.
	stButtonOn lipgloss.Style
)

func init() { applyTheme(themes[0]) }

// applyTheme rebuilds the palette, every style and the visualizer's
// inks from t. The text inputs copy their styles: the Model sets them
// again (see Model.setTheme).
func applyTheme(t theme) {
	colRed = lipgloss.Color(t.red)
	colRedDeep = lipgloss.Color(t.redDeep)
	colCyan = lipgloss.Color(t.cyan)
	colYellow = lipgloss.Color(t.yellow)
	colDim = lipgloss.Color(t.dim)
	colMuted = lipgloss.Color(t.muted)
	colSelectBg = lipgloss.Color(t.selectBg)
	colInk = lipgloss.Color(t.ink)
	colAlert = lipgloss.Color(t.alert)

	stRed = lipgloss.NewStyle().Foreground(colRed)
	stRedBold = stRed.Bold(true)
	stFrame = lipgloss.NewStyle().Foreground(colRedDeep)
	stFrameDim = lipgloss.NewStyle().Foreground(colDim)
	stCyan = lipgloss.NewStyle().Foreground(colCyan)
	stCyanBold = stCyan.Bold(true)
	stYellow = lipgloss.NewStyle().Foreground(colYellow)
	stYellowB = stYellow.Bold(true)
	stDim = lipgloss.NewStyle().Foreground(colDim)
	stMuted = lipgloss.NewStyle().Foreground(colMuted)
	stSelected = lipgloss.NewStyle().Foreground(colCyan).Background(colSelectBg).Bold(true)
	stAlert = lipgloss.NewStyle().Foreground(colAlert)
	stAlertBold = stAlert.Bold(true)
	stButtonOn = lipgloss.NewStyle().Foreground(colInk).Background(colYellow).Bold(true)

	vizInks = []ink{
		inkNone:    {},
		inkYellow:  inkOf(stYellow),
		inkRedBold: inkOf(stRedBold),
		inkRed:     inkOf(stRed),
		inkMuted:   inkOf(stMuted),
		inkDim:     inkOf(stDim),
	}
	noiseStyles = []lipgloss.Style{stRed, stCyan, stYellow, stFrameDim}
}

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
