package radio

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Layout thresholds. Below fullMin* the UI collapses into a single column
// without the EQ; below tinyMin* only the wordmark remains.
const (
	fullMinWidth  = 60
	fullMinHeight = 16
	tinyMinWidth  = 20
	tinyMinHeight = 5
	eqMaxRows     = 12
)

// View renders the radio in the alternate screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "NU11SIGNAL // NIGHT CITY RADIO"
	return v
}

// render returns the frame as a styled string of at most width x height.
func (m Model) render() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return ""
	}
	var lines []string
	switch {
	case w < tinyMinWidth || h < tinyMinHeight:
		lines = m.renderTiny()
	case m.auth == authFailed:
		lines = m.renderAuthError()
	case w < fullMinWidth || h < fullMinHeight:
		lines = m.renderCompact()
	default:
		lines = m.renderFull()
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderTiny() []string {
	return []string{stRedBold.Render("NU11SIGNAL"), m.statusTag()}
}

func (m Model) renderFull() []string {
	w, h := m.width, m.height
	bodyH := h - 4
	leftW := m.listPanelWidth(w)
	rightW := w - leftW - 1

	left := m.listPanel(leftW, bodyH)
	right := panel("NOW PLAYING", "NC-NET 0x2077", m.nowPlaying(rightW-2, bodyH-2), rightW, bodyH, true)

	lines := m.header(w)
	for i := range bodyH {
		lines = append(lines, left[i]+" "+right[i])
	}
	return append(lines, m.statusLine(w), m.hintLine(w))
}

// listPanelWidth is the width of the list panel in the full layout. The
// browse pages take more of the screen from NOW PLAYING: their rows pair a
// title with an album, year, curator or duration.
func (m Model) listPanelWidth(w int) int {
	if isPage(m.top().kind) {
		return artistPanelWidth(w)
	}
	return stationPanelWidth(w)
}

// stationPanelWidth is the list panel width of the stations and search
// views.
func stationPanelWidth(w int) int { return max(28, min(w*2/5, 44)) }

// artistPanelWidth is the list panel width of the browse pages; NOW
// PLAYING keeps at least 23 cells at the narrowest full layout.
func artistPanelWidth(w int) int { return max(28, min(w*3/5, 72)) }

// listBodyWidth is the width listView draws in: the list panel's inside in
// the full layout, the whole screen in the compact one.
func (m Model) listBodyWidth() int {
	if m.width >= fullMinWidth && m.height >= fullMinHeight {
		return m.listPanelWidth(m.width) - 2
	}
	return m.width
}

func (m Model) renderCompact() []string {
	w := m.width
	lines := []string{m.headerLeft(false) + "  " + m.statusTag(), stFrameDim.Render(strings.Repeat("─", w))}
	title, artist := m.titleLines()
	lines = append(lines, title, artist, m.progressLine(w))
	lines = append(lines, stFrameDim.Render(strings.Repeat("─", w)))
	listH := m.height - len(lines) - 2
	if listH > 0 {
		_, _, body := m.listView(w, listH)
		lines = append(lines, body...)
		for len(lines) < m.height-2 {
			lines = append(lines, "")
		}
	}
	return append(lines, m.statusLine(w), m.hintLine(w))
}

func (m Model) renderAuthError() []string {
	boxW := min(m.width, 66)
	body := []string{
		"",
		stYellowB.Render("▲ ACCESS DENIED"),
		"",
		stRed.Render("NU11SIGNAL CANNOT REACH YOUR APPLE MUSIC LIBRARY."),
		stMuted.Render(strings.ToUpper(m.authDetail)),
		"",
		stRed.Render("GRANT ACCESS IN SYSTEM SETTINGS › PRIVACY & SECURITY"),
		stRed.Render("› MEDIA & APPLE MUSIC, THEN RESTART."),
		"",
		keyCap("Q") + stRed.Render(" QUIT"),
	}
	box := panel("AUTH // ERROR", "ERR-403", body, boxW, min(len(body)+2, m.height), true)
	pad := strings.Repeat(" ", (m.width-boxW)/2)
	top := max((m.height-len(box))/2, 0)
	lines := make([]string, top, top+len(box))
	for _, l := range box {
		lines = append(lines, pad+l)
	}
	return lines
}

func (m Model) headerLeft(wide bool) string {
	sub := "NIGHT CITY RADIO"
	if wide {
		sub = spaced(sub)
	}
	return stYellow.Render("◢◤ ") + stRedBold.Render("NU11SIGNAL") + stMuted.Render(" // ") + stRed.Render(sub)
}

func (m Model) header(w int) []string {
	var auth string
	switch m.auth {
	case authOK:
		auth = stCyan.Render("AUTH OK")
	case authFailed:
		auth = stYellow.Render("AUTH FAIL")
	default:
		auth = stYellow.Render("LINKING")
	}
	sig := stCyan.Render("▂▄▆█")
	if m.signalLost() {
		sig = stYellow.Render("▂") + stDim.Render("▄▆█") + stYellow.Render(" SIGNAL LOST")
	}
	right := auth + stDim.Render("  ▮  ") + stMuted.Render("SIG ") + sig +
		stDim.Render("  ▮  ") + stRed.Render(m.now().Format("15:04:05"))

	left := m.headerLeft(true)
	if ansi.StringWidth(left)+ansi.StringWidth(right)+2 > w {
		left = m.headerLeft(false)
	}
	gap := max(w-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	top := left + strings.Repeat(" ", gap) + right

	code := " RDO-77 // NC-NET "
	rule := stYellow.Render("▓▒░") + stFrameDim.Render(strings.Repeat("─", max(w-3-len(code)-2, 0))) +
		stMuted.Render(code) + stFrameDim.Render("──")
	return []string{top, rule}
}

func (m Model) statusTag() string {
	switch {
	case m.signalLost():
		return stYellowB.Render("▮ SIGNAL LOST")
	case !m.hasState:
		return stMuted.Render("▮ STANDBY")
	}
	switch m.state.Status {
	case playback.StatusPlaying:
		return stCyanBold.Render("▮ PLAYING")
	case playback.StatusPaused:
		return stYellowB.Render("▮ PAUSED")
	case playback.StatusSeeking:
		return stCyan.Render("▮ SEEKING")
	}
	return stMuted.Render("▮ STANDBY")
}

// titleLines returns the styled title and artist lines of the now-playing
// display, including the signal-lost and idle states.
func (m Model) titleLines() (string, string) {
	switch {
	case m.signalLost():
		return stYellowB.Render("SIGNAL LOST"), stRed.Render("HELPER OFFLINE // RESTART NU11SIGNAL")
	case !m.hasState || m.state.Title == "":
		return stMuted.Render("NO CARRIER"), stDim.Render("TUNE A STATION WITH [ENTER]")
	}
	title := glitchText(strings.ToUpper(m.state.Title), m.glitch, mix(m.seed, m.frame))
	return stCyanBold.Render(title), stRed.Render(strings.ToUpper(m.state.Artist))
}

func (m Model) progressLine(w int) string {
	var dur = m.state.Duration
	pos := m.position()
	times := formatClock(pos) + " / " + formatClock(dur)
	barW := w - len(times) - 2
	if barW < 4 {
		return stRed.Render(times)
	}
	elapsed := progressBar(pos, dur, barW)
	filled := strings.Count(elapsed, "▮")
	return stCyan.Render(strings.Repeat("▮", filled)) + stDim.Render(strings.Repeat("▯", barW-filled)) +
		"  " + stRed.Render(times)
}

func (m Model) nowPlaying(iw, ih int) []string {
	title, artist := m.titleLines()
	album := ""
	if m.hasState && !m.signalLost() {
		album = stMuted.Render(strings.ToUpper(m.state.Album))
	}
	label := stMuted.Render(spaced("NOW PLAYING"))
	tag := m.statusTag()
	if ansi.StringWidth(label)+1+ansi.StringWidth(tag) > iw-1 {
		// The narrower panel beside the artist page keeps the status whole.
		label = stMuted.Render("NOW PLAYING")
	}
	head := label + strings.Repeat(" ", max(iw-1-ansi.StringWidth(label)-ansi.StringWidth(tag), 1)) + tag

	lines := []string{
		" " + head,
		"",
		" " + title,
		" " + artist,
		" " + album,
		"",
		" " + m.progressLine(iw-2),
		" " + m.feedLine(),
		"",
	}
	eqRows := min(ih-len(lines), eqMaxRows)
	if eqRows >= 2 {
		// Sit the spectrum on the bottom edge of the panel.
		for len(lines)+eqRows < ih {
			lines = append(lines, "")
		}
		for i, row := range m.bars.render(iw-2, eqRows) {
			style := stRed
			switch {
			case i == 0:
				style = stYellow
			case i < eqRows/2:
				style = stRedBold
			}
			lines = append(lines, " "+style.Render(row))
		}
	}
	return lines
}

// feedLine names where the music comes from: a station's frequency or the
// catalog.
func (m Model) feedLine() string {
	for i, s := range m.stations {
		if s.ID == m.playingStation {
			return stYellow.Render("▞ "+frequency(i)+" MHZ") + stMuted.Render(" // "+strings.ToUpper(s.Name))
		}
	}
	if m.hasState && m.state.Title != "" {
		return stYellow.Render("▞ CATALOG FEED") + stMuted.Render(" // DIRECT")
	}
	return stDim.Render("▞ ---.- MHZ // NO FEED")
}

// listPanel frames the view on top of the navigation stack.
func (m Model) listPanel(w, h int) []string {
	title, code, body := m.listView(w-2, h-2)
	return panel(title, code, body, w, h, true)
}

// listView is the one dispatch on the view on top of the navigation stack:
// its panel title and code, and its body rendered in w x h cells.
func (m Model) listView(w, h int) (title, code string, body []string) {
	switch m.top().kind {
	case viewSearch:
		return "SEARCH", m.searchCode(), m.searchBody(w, h)
	case viewArtist:
		return "ARTIST", m.artistCode(), m.artistBody(w, h)
	case viewAlbum, viewPlaylist:
		return m.trackTitle(), m.trackCode(), m.trackBody(w, h)
	}
	return "STATIONS", fmt.Sprintf("BAND FM // %02d CH", len(m.stations)), m.stationRows(w, h)
}

// stationRows renders the visible window of the station list, scrolled so
// the cursor stays on screen.
func (m Model) stationRows(w, h int) []string {
	if h <= 0 {
		return nil
	}
	n := len(m.stations)
	if n == 0 {
		msg := "SCANNING BANDS..."
		if m.stationsFailed {
			msg = "[R] RETRY // SCAN FAILED"
		} else if m.auth == authOK {
			msg = "NO STATIONS // LIBRARY EMPTY"
		}
		return []string{" " + stDim.Render(msg)}
	}
	cur := m.stationCursor()
	offset := max(0, cur-h+1)
	rows := make([]string, 0, h)
	for i := offset; i < n && len(rows) < h; i++ {
		rows = append(rows, m.stationRow(i, i == cur, w))
	}
	return rows
}

func (m Model) stationRow(i int, selected bool, w int) string {
	s := m.stations[i]
	onAir := s.ID == m.playingStation
	name := strings.ToUpper(s.Name)
	mark := "  "
	if onAir {
		mark = " ◉"
	}
	textWidth := max(w-ansi.StringWidth(mark), 0) // cells left for the row text before the mark
	if selected {
		text := "▌▶ " + frequency(i) + "  " + name
		text = fit(text, textWidth) + mark
		return stSelected.Render(fit(text, w))
	}
	nameStyle := stRed
	if onAir {
		nameStyle = stYellow
	}
	text := "   " + stMuted.Render(frequency(i)) + "  " + nameStyle.Render(name)
	return fit(fit(text, textWidth)+stYellow.Render(mark), w)
}

func (m Model) statusLine(w int) string {
	if m.status != "" {
		return stYellow.Render("▲ " + strings.ToUpper(m.status))
	}
	return stDim.Render(fit("░▒▓ SYS NOMINAL // BUF 0x5EF6", w))
}

func keyCap(k string) string { return stYellow.Render("[" + k + "]") }

// hintLine lays out key hints, dropping lower-priority ones (never the last,
// quit) until they fit.
func (m Model) hintLine(w int) string {
	hints := playerHints
	switch m.top().kind {
	case viewSearch:
		hints = searchHints
	case viewArtist:
		hints = artistHints
	case viewAlbum, viewPlaylist:
		hints = trackHints
	}
	render := func(hs []hint) string {
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = keyCap(h.key) + " " + stRed.Render(h.label)
		}
		return strings.Join(parts, "  ")
	}
	shown := append([]hint(nil), hints...)
	for len(shown) > 1 && ansi.StringWidth(render(shown)) > w {
		shown = append(shown[:len(shown)-2], shown[len(shown)-1])
	}
	return render(shown)
}
