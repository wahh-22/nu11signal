package radio

import (
	"fmt"
	"slices"
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
	// Clicks, releases and the wheel; motion is not needed (no hover).
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "NU11SIGNAL // NIGHT CITY RADIO"
	return v
}

// render returns the frame as a styled string of at most width x height.
func (m Model) render() string {
	lines, _ := m.layout()
	return strings.Join(lines, "\n")
}

// layout lays out the frame, with the signal effects drawn over it while
// they run (see glitch.go); they never move the zones.
func (m Model) layout() ([]string, zones) {
	lines, zs := m.baseLayout()
	if m.fxActive() {
		lines = m.decorate(lines)
	}
	return lines, zs
}

// baseLayout lays out the frame: its lines, at most width x height, and
// the clickable zones drawn on them. The tiny layout and the auth error
// screen have no zones: there is no room for buttons, or nothing to click.
func (m Model) baseLayout() ([]string, zones) {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return nil, nil
	}
	var lines []string
	var zs zones
	switch {
	case w < tinyMinWidth || h < tinyMinHeight:
		lines = m.renderTiny()
	case m.auth == authFailed:
		lines = m.renderAuthError()
	case w < fullMinWidth || h < fullMinHeight:
		lines, zs = m.renderCompact()
	default:
		lines, zs = m.renderFull()
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "")
	}
	return lines, zs.clip(w, h)
}

func (m Model) renderTiny() []string {
	return []string{stRedBold.Render("NU11SIGNAL"), m.statusTag()}
}

func (m Model) renderFull() ([]string, zones) {
	w, h := m.width, m.height
	bodyH := h - 4
	lines, zs := m.header(w)
	top := len(lines)
	// The panels go under the zones drawn in them, which stay on top.
	if m.expanded {
		// NOW PLAYING takes the list panel's place too.
		playing, playingZones := m.nowPlaying(w-2, bodyH-2)
		zs.addBox(zonePanelPlayer, 0, top, w, bodyH)
		zs.addAt(1, top+1, playingZones.clip(w-2, bodyH-2))
		lines = append(lines, panel("NOW PLAYING", "NC-NET 0x2077", playing, w, bodyH, true)...)
		return append(lines, m.statusLine(w), m.hintLine(w)), zs
	}
	leftW := listPanelWidthFor(w)
	rightW := w - leftW - 1

	left, leftZones := m.listPanel(leftW, bodyH)
	playing, playingZones := m.nowPlaying(rightW-2, bodyH-2)
	right := panel("NOW PLAYING", "NC-NET 0x2077", playing, rightW, bodyH, m.focus == areaPlayer)

	zs.addBox(zonePanelList, 0, top, leftW, bodyH)
	zs.addBox(zonePanelPlayer, leftW+1, top, rightW, bodyH)
	zs.addAt(0, top, leftZones)
	// Inside the NOW PLAYING frame, right of the list panel and the gap.
	zs.addAt(leftW+2, top+1, playingZones.clip(rightW-2, bodyH-2))
	for i := range bodyH {
		lines = append(lines, left[i]+" "+right[i])
	}
	return append(lines, m.statusLine(w), m.hintLine(w)), zs
}

// compactVolumeWidth is the widest volume row beside the transport
// buttons in the compact layout, and compactVolumeMin the narrowest, with
// its buttons and a bare percentage.
const (
	compactVolumeWidth = 26
	compactVolumeMin   = 21
)

// playerMinWidth is the narrowest NOW PLAYING panel beside the list: room
// for the four transport buttons as glyphs.
const playerMinWidth = 30

// listPanelWidthFor is the width of the list panel in the full layout, the
// same in every view: the browse pages' width, whose rows pair a title
// with an album, year, curator or duration, so that the panel never
// resizes when a page opens. NOW PLAYING keeps at least playerMinWidth
// cells beside it.
func listPanelWidthFor(w int) int { return max(28, min(w*3/5, 72, w-1-playerMinWidth)) }

// listBodyWidth is the width listView draws in: the list panel's inside in
// the full layout, the whole screen in the compact one.
func (m Model) listBodyWidth() int {
	if m.width >= fullMinWidth && m.height >= fullMinHeight {
		return listPanelWidthFor(m.width) - 2
	}
	return m.width
}

// renderCompact stacks the screen in one column: the nav bar takes the
// header rule, the title and artist lines end in the ♥ and LOOP buttons,
// then the transport buttons, the volume row while it fits and the rule
// over the list, which the expanded player leaves out.
func (m Model) renderCompact() ([]string, zones) {
	w := m.width
	nav, zs := m.navLine(w)
	zs = zs.shifted(0, 1)
	lines := []string{m.headerLeft(false) + "  " + m.statusTag(), nav}
	// The player lines (title to buttons) and the list below them are the
	// panels, under the zones drawn in them.
	var panels zones
	playerTop := len(lines)
	title, artist := m.titleLines()
	title, hz := m.heartTitle(title, w-1)
	zs.addAt(0, len(lines), hz)
	artist, lz := m.loopTail(artist, w-1)
	zs.addAt(0, len(lines)+1, lz)
	lines = append(lines, title, artist)
	progress, barW := m.progressLine(w - 1)
	if m.seekable() {
		zs.add(zoneSeek, 1, len(lines), barW)
	}
	lines = append(lines, m.barMark()+progress)
	// The volume row takes the room of the transport labels when it can
	// fit beside all four buttons that way.
	transport, tz := m.transportBar(w - 3 - compactVolumeMin)
	withVolume := len(tz) == int(ctlExpand)+1
	if !withVolume {
		transport, tz = m.transportBar(w - 1)
	}
	zs.addAt(1, len(lines), tz)
	controls := " " + transport + " "
	if withVolume {
		volume, vz := m.volumeBar(min(w-ansi.StringWidth(controls)-1, compactVolumeWidth))
		zs.addAt(ansi.StringWidth(controls), len(lines), vz)
		controls += volume + " "
	}
	lines = append(lines, controls+stFrameDim.Render(strings.Repeat("─", max(w-ansi.StringWidth(controls), 0))))
	panels.addBox(zonePanelPlayer, 0, playerTop, w, len(lines)-playerTop)
	listH := m.height - len(lines) - 2
	if listH > 0 && !m.expanded {
		_, _, body, bz := m.listView(w, listH)
		panels.addBox(zonePanelList, 0, len(lines), w, listH)
		zs.addAt(0, len(lines), bz.clip(w, listH))
		lines = append(lines, body...)
		for len(lines) < m.height-2 {
			lines = append(lines, "")
		}
	}
	return append(lines, m.statusLine(w), m.hintLine(w)), append(panels, zs...)
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

// header is the title line over the nav bar.
func (m Model) header(w int) ([]string, zones) {
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

	nav, zs := m.navLine(w)
	return []string{top, nav}, zs.shifted(0, 1)
}

// navLine is the header rule carrying the nav bar: the PLAYLISTS and
// SEARCH tabs and, on a page, BACK. The serial code stays
// at the right edge while there is room for it.
//
//	▓▒░ ╱ PLAYLISTS ╱ ╱ SEARCH ╱ ╱ BACK ╱ ── RDO-77 // NC-NET ──
func (m Model) navLine(w int) (string, zones) {
	const (
		mark    = "▓▒░"
		gap     = " "
		code    = " RDO-77 // NC-NET "
		codeEnd = "──"
		// minRule is the rule kept between the bar and the code.
		minRule = 1
	)
	lead := ansi.StringWidth(mark + gap)
	gapW := ansi.StringWidth(gap)
	bar, bz := buttonBar(m.navButtons(), w-lead-gapW)
	var zs zones
	zs.addAt(lead, 0, bz)
	rest := max(w-lead-ansi.StringWidth(bar)-gapW, 0)
	tail := stFrameDim.Render(strings.Repeat("─", rest))
	if codeW := ansi.StringWidth(code + codeEnd); rest >= codeW+minRule {
		tail = stFrameDim.Render(strings.Repeat("─", rest-codeW)) + stMuted.Render(code) + stFrameDim.Render(codeEnd)
	}
	return stYellow.Render(mark) + gap + bar + gap + tail, zs
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
		return stMuted.Render("NO CARRIER"), stDim.Render("OPEN A PLAYLIST WITH [ENTER]")
	}
	title := glitchText(strings.ToUpper(m.state.Title), m.glitch, mix(m.seed, m.frame))
	return stCyanBold.Render(title), stRed.Render(strings.ToUpper(m.state.Artist))
}

// progressLine renders the progress bar and the times in w cells; barW
// is the width of the bar at the start of the line, 0 when there is no
// room for one.
func (m Model) progressLine(w int) (line string, barW int) {
	var dur = m.state.Duration
	pos := m.position()
	times := formatClock(pos) + " / " + formatClock(dur)
	barW = w - len(times) - 2
	if barW < 4 {
		return stRed.Render(times), 0
	}
	elapsed := progressBar(pos, dur, barW)
	filled := strings.Count(elapsed, "▮")
	return stCyan.Render(strings.Repeat("▮", filled)) + stDim.Render(strings.Repeat("▯", barW-filled)) +
		"  " + stRed.Render(times), barW
}

// nowPlayingMargin is the room NOW PLAYING leaves on each side of the
// lines under its head.
const nowPlayingMargin = 1

// nowPlaying renders the inside of the NOW PLAYING panel, iw x ih cells,
// with its zones: the progress bar (click to seek), the transport buttons
// under the feed, the volume row under them and LOOP under that.
func (m Model) nowPlaying(iw, ih int) ([]string, zones) {
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

	// The lines under the head sit nowPlayingMargin cells in from each
	// side, and so do their zones.
	inner := iw - 2*nowPlayingMargin
	lines := []string{" " + head, ""}
	title, hz := m.heartTitle(title, inner)
	var zs zones
	zs.addAt(nowPlayingMargin, len(lines), hz)
	lines = append(lines,
		" "+title,
		" "+artist,
		" "+album,
		"",
	)
	progress, barW := m.progressLine(inner)
	if m.seekable() {
		zs.add(zoneSeek, nowPlayingMargin, len(lines), barW)
	}
	lines = append(lines, m.barMark()+progress, " "+m.feedLine())
	if ih > len(lines)+2 {
		// The buttons keep their gap from the feed while it leaves room
		// for the volume row.
		lines = append(lines, "")
	}
	transport, tz := m.transportBar(inner)
	zs.addAt(nowPlayingMargin, len(lines), tz)
	lines = append(lines, " "+transport)
	volume, vz := m.volumeBar(inner)
	zs.addAt(nowPlayingMargin, len(lines), vz)
	lines = append(lines, " "+volume)
	loop, lz := m.loopBar(inner)
	zs.addAt(nowPlayingMargin, len(lines), lz)
	lines = append(lines, " "+loop)

	eqRows := min(ih-len(lines), eqMaxRows)
	if eqRows >= 2 {
		// Sit the spectrum on the bottom edge of the panel.
		for len(lines)+eqRows < ih {
			lines = append(lines, "")
		}
		for i, row := range m.bars.render(inner, eqRows) {
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
	return lines, zs
}

// barMark is the cell before the progress bar: a marker while the bar has
// the focus.
func (m Model) barMark() string {
	if m.barFocused() && m.seekable() {
		return stYellowB.Render("▸")
	}
	return " "
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

// listPanel frames the view on top of the navigation stack; its zones are
// in the panel's coordinates.
func (m Model) listPanel(w, h int) ([]string, zones) {
	title, code, body, zs := m.listView(w-2, h-2)
	return panel(title, code, body, w, h, m.focus == areaList), zs.clip(w-2, h-2).shifted(1, 1)
}

// listView is the one dispatch on the view on top of the navigation stack:
// its panel title and code, and its body rendered in w x h cells with the
// zones of its rows.
func (m Model) listView(w, h int) (title, code string, body []string, zs zones) {
	if m.editor.mode != editClosed {
		body, zs = m.editorBody(w, h)
		return m.editorTitle(), m.editorCode(), body, zs
	}
	switch m.top().kind {
	case viewSearch:
		body, zs = m.searchBody(w, h)
		return "SEARCH", m.searchCode(), body, zs
	case viewResults:
		body, zs = m.resultsBody(w, h)
		return "RESULTS", m.resultsCode(), body, zs
	case viewArtist:
		body, zs = m.artistBody(w, h)
		return "ARTIST", m.artistCode(), body, zs
	case viewAlbum, viewPlaylist:
		body, zs = m.trackBody(w, h)
		return m.trackTitle(), m.trackCode(), body, zs
	}
	body, zs = m.stationRows(w, h)
	return "PLAYLISTS", fmt.Sprintf("BAND FM // %02d CH", len(m.stations)), body, zs
}

// stationRows renders the visible window of the station list, the + NEW
// PLAYLIST row over it once the library is reachable, scrolled so the
// cursor stays on screen.
func (m Model) stationRows(w, h int) ([]string, zones) {
	if h <= 0 {
		return nil, nil
	}
	n := len(m.stations)
	cur := m.stationCursor()
	rows := make([]string, 0, h)
	var zs zones
	for i := max(m.firstStationRow(), cur-h+1); i < n && len(rows) < h; i++ {
		if i < 0 {
			zs.add(zoneNewPlaylist, 0, len(rows), w)
			rows = append(rows, newPlaylistLine(cur < 0, w))
			continue
		}
		zs.add(rowZone(i), 0, len(rows), w)
		rows = append(rows, m.stationRow(i, i == cur, w))
	}
	if n == 0 && len(rows) < h {
		msg := "SCANNING BANDS..."
		if m.stationsFailed {
			msg = "[R] RETRY // SCAN FAILED"
			zs.add(zoneRetry, 0, len(rows), w)
		} else if m.auth == authOK {
			msg = "NO PLAYLISTS // LIBRARY EMPTY"
		}
		rows = append(rows, " "+stDim.Render(msg))
	}
	return rows, zs
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
	if alert, ok := m.alertLine(w); ok {
		return alert
	}
	return stDim.Render(fit("░▒▓ SYS NOMINAL // BUF 0x5EF6", w))
}

func keyCap(k string) string { return stYellow.Render("[" + k + "]") }

// hintLine lays out key hints, dropping lower-priority ones (never the last,
// quit) until they fit.
func (m Model) hintLine(w int) string {
	hints := playerHints
	switch kind := m.top().kind; {
	case m.focus == areaTabs:
		hints = tabsFocusHints(kind == viewSearch || m.editor.mode == editName)
	case m.focus == areaPlayer:
		hints = playerFocusHints(m.expanded, kind == viewSearch || m.editor.mode == editName)
	case m.editor.mode == editPick:
		hints = pickerHints
	case m.editor.mode == editName:
		hints = nameHints
	case kind == viewSearch:
		hints = searchHints
		if m.recentSelected() {
			hints = recentHints
		} else if _, ok := m.selectedSong(); ok {
			hints = searchSongHints
		}
	case kind == viewResults:
		hints = resultsHints
	case kind == viewArtist:
		hints = artistHints
	case kind == viewAlbum, kind == viewPlaylist:
		hints = trackHints
	}
	render := func(hs []hint) string {
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = keyCap(h.key) + " " + stRed.Render(h.label)
		}
		return strings.Join(parts, "  ")
	}
	if ansi.StringWidth(render(hints)) > w {
		hints = shortHints(hints)
	}
	shown := append([]hint(nil), hints...)
	for len(shown) > 1 && ansi.StringWidth(render(shown)) > w {
		shown = append(shown[:len(shown)-2], shown[len(shown)-1])
	}
	return render(shown)
}

// shortHints names only DEL of the recent delete keys.
func shortHints(hs []hint) []hint {
	out := slices.Clone(hs)
	for i, h := range out {
		if h.key == recentDeleteKeys {
			out[i].key = "DEL"
		}
	}
	return out
}
