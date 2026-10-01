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
// a dark fill. The roles are finer than NIGHT CITY needs (it draws most
// of them in its red, cyan and yellow) so that a richer palette can tell
// headings, names, numbers and states apart.
type theme struct {
	// name is the theme as SETTINGS lists it and config.json stores it.
	name string
	// label draws labels, hints and the wordmark; text the names of the
	// rows, the songs and the artists and the descriptions; number the
	// times, the clock and the volume's percentage.
	label, text, number string
	// frame draws the focused frames, dim the unfocused ones and what sits
	// furthest behind, muted what sits behind (codes, details).
	frame, dim, muted string
	// hi draws the highlights (the song playing's title, meters, the
	// transport, the song and playlist glyphs); accent the marks, glyphs
	// and key caps; heading the section and page headings (bold); warn
	// what went wrong or waits (▲ status, SIGNAL LOST, PAUSED); onAir the
	// station or track playing in a list; favorite the <3 mark; focus the
	// focus markers and the input cursor; ok what works (AUTH OK, the
	// signal bars, PLAYING).
	hi, accent, heading, warn, onAir, favorite, focus, ok string
	// fill fills the active button, its text in ink; selectBg fills the
	// selected row.
	fill, ink, selectBg string
	// alert draws the NO SIGNAL sign (bold), alertStatic the static
	// framing it.
	alert, alertStatic string
	// rainTip, rainBright (bold) and rainBody are the first steps of the
	// rain's trail, muted and dim the last (see rainRamp).
	rainTip, rainBright, rainBody string
	// noise colors the cells a burst corrupts.
	noise [4]string
}

// themes are the themes SETTINGS offers, the default first.
//
// NIGHT CITY draws every role in the colors the app had before themes:
// its baseline frames (view_night_city_ansi_80x24.golden,
// night_city_ansi_views_80x24.golden) hold them byte for byte.
//
// BLUE is the gentleman-blue palette, role by role:
//
//	label, alertStatic, rainBright  primary     #347AFF
//	text                            foreground  #DBE9FF
//	number                          orange      #FF9F1C
//	frame                           deep blue   #2A62CC
//	dim                             border      #1C2C54
//	muted                           muted       #4A5578
//	hi, accent, onAir, rainTip      cyan        #5CE1FF
//	heading, fill, alert, rainBody  violet      #7C5CFF
//	warn, focus                     yellow      #FFD23D
//	favorite                        red         #FF3D81
//	ok                              green       #4DFF88
//	selectBg                        userSurface #10182E
//	ink                             background  #05070F
//	noise                           primary, cyan, violet, border
var themes = []theme{
	{
		name:  "NIGHT CITY",
		label: "#FF5F57", text: "#FF5F57", number: "#FF5F57",
		frame: "#E8554E", dim: "#5A1E1E", muted: "#9A3B37",
		hi: "#5EF6FF", accent: "#FCEE0A", heading: "#FCEE0A", warn: "#FCEE0A",
		onAir: "#FCEE0A", favorite: "#FCEE0A", focus: "#FCEE0A", ok: "#5EF6FF",
		fill: "#FCEE0A", ink: "#0A0A0A", selectBg: "#0E2A2F",
		alert: "#FF5F57", alertStatic: "#FF5F57",
		rainTip: "#FCEE0A", rainBright: "#FF5F57", rainBody: "#FF5F57",
		noise: [4]string{"#FF5F57", "#5EF6FF", "#FCEE0A", "#5A1E1E"},
	},
	{
		name:  "BLUE",
		label: "#347AFF", text: "#DBE9FF", number: "#FF9F1C",
		frame: "#2A62CC", dim: "#1C2C54", muted: "#4A5578",
		hi: "#5CE1FF", accent: "#5CE1FF", heading: "#7C5CFF", warn: "#FFD23D",
		onAir: "#5CE1FF", favorite: "#FF3D81", focus: "#FFD23D", ok: "#4DFF88",
		fill: "#7C5CFF", ink: "#05070F", selectBg: "#10182E",
		alert: "#7C5CFF", alertStatic: "#347AFF",
		rainTip: "#5CE1FF", rainBright: "#347AFF", rainBody: "#7C5CFF",
		noise: [4]string{"#347AFF", "#5CE1FF", "#7C5CFF", "#1C2C54"},
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
// changing it. Each style is named after its role (see theme).
var (
	colLabel, colFrame, colHi, colAccent   color.Color
	colDim, colMuted, colSelectBg, colInk  color.Color
	colFocus                               color.Color
	stLabel, stLabelBold, stText, stNumber lipgloss.Style
	stFrame, stFrameDim, stDim, stMuted    lipgloss.Style
	stHi, stHiBold, stAccent, stAccentBold lipgloss.Style
	stHeading, stWarn, stWarnBold          lipgloss.Style
	stOnAir, stOnAirBold, stFav            lipgloss.Style
	stFocus, stFocusBold, stOK, stOKBold   lipgloss.Style
	stSelected, stAlertStatic, stAlertSign lipgloss.Style
	// stButtonOn fills the active button: ink on the fill; stFillEdge
	// draws the fill's color as text, the slants around a filled tab.
	stButtonOn, stFillEdge lipgloss.Style
)

func init() { applyTheme(themes[0]) }

// fg is a style drawing text in the color hex.
func fg(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)) }

// applyTheme rebuilds the palette, every style, the visualizer's inks
// and the burst noise from t. The text inputs copy their styles: the
// Model sets them again (see Model.setTheme).
func applyTheme(t theme) {
	colLabel = lipgloss.Color(t.label)
	colFrame = lipgloss.Color(t.frame)
	colHi = lipgloss.Color(t.hi)
	colAccent = lipgloss.Color(t.accent)
	colDim = lipgloss.Color(t.dim)
	colMuted = lipgloss.Color(t.muted)
	colSelectBg = lipgloss.Color(t.selectBg)
	colInk = lipgloss.Color(t.ink)
	colFocus = lipgloss.Color(t.focus)

	stLabel = lipgloss.NewStyle().Foreground(colLabel)
	stLabelBold = stLabel.Bold(true)
	stText = fg(t.text)
	stNumber = fg(t.number)
	stFrame = lipgloss.NewStyle().Foreground(colFrame)
	stFrameDim = lipgloss.NewStyle().Foreground(colDim)
	stDim = lipgloss.NewStyle().Foreground(colDim)
	stMuted = lipgloss.NewStyle().Foreground(colMuted)
	stHi = lipgloss.NewStyle().Foreground(colHi)
	stHiBold = stHi.Bold(true)
	stAccent = lipgloss.NewStyle().Foreground(colAccent)
	stAccentBold = stAccent.Bold(true)
	stHeading = fg(t.heading).Bold(true)
	stWarn = fg(t.warn)
	stWarnBold = stWarn.Bold(true)
	stOnAir = fg(t.onAir)
	stOnAirBold = stOnAir.Bold(true)
	stFav = fg(t.favorite).Bold(true)
	stFocus = lipgloss.NewStyle().Foreground(colFocus)
	stFocusBold = stFocus.Bold(true)
	stOK = fg(t.ok)
	stOKBold = stOK.Bold(true)
	stSelected = lipgloss.NewStyle().Foreground(colHi).Background(colSelectBg).Bold(true)
	stAlertStatic = fg(t.alertStatic)
	stAlertSign = fg(t.alert).Bold(true)
	stButtonOn = lipgloss.NewStyle().Foreground(colInk).Background(lipgloss.Color(t.fill)).Bold(true)
	stFillEdge = fg(t.fill)

	vizInks = []ink{
		inkNone:   {},
		inkTip:    inkOf(fg(t.rainTip)),
		inkBright: inkOf(fg(t.rainBright).Bold(true)),
		inkBody:   inkOf(fg(t.rainBody)),
		inkMuted:  inkOf(stMuted),
		inkDim:    inkOf(stDim),
	}
	noiseStyles = make([]lipgloss.Style, 0, len(t.noise))
	for _, c := range t.noise {
		noiseStyles = append(noiseStyles, fg(c))
	}
}

func inputStyles() textinput.Styles {
	s := textinput.DefaultDarkStyles()
	s.Focused.Text = stHi
	s.Focused.Placeholder = stDim
	s.Blurred.Text = stMuted
	s.Blurred.Placeholder = stDim
	s.Cursor.Color = colFocus
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
		frame, lbl, mark = stFrame, stLabelBold, stFocus.Render("▮")
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
