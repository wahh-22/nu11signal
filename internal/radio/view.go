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
)

// View renders the radio in the alternate screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	// Clicks, releases and the wheel; motion is not needed (no hover).
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "NU11SIGNAL"
	return v
}

// render returns the frame as a styled string of at most width x height.
func (m Model) render() string {
	lines, _ := m.layout()
	return strings.Join(lines, "\n")
}

// layout lays out the frame, with the content intro, the boot glitch and
// the signal effects drawn over it while they run (see intro.go, boot.go
// and glitch.go); they never move the zones.
func (m Model) layout() ([]string, zones) {
	lines, zs := m.baseLayout()
	if m.introOn() {
		m.drawIntro(lines)
	}
	if m.bootGlitching() {
		m.bootGlitch(lines)
	}
	if m.fxActive() {
		lines = m.decorate(lines)
	}
	return lines, zs
}

// baseLayout lays out the frame: its lines, at most width x height, and
// the clickable zones drawn on them. The tiny layout and the auth error
// screen have no zones: there is no room for buttons, or nothing to click.
// The quit modal draws over any of them, its zones in place of theirs.
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
	if m.quitAsk {
		lines, zs = m.withQuitModal(lines)
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
	return []string{stLabelBold.Render("NU11SIGNAL"), m.statusTag()}
}

func (m Model) renderFull() ([]string, zones) {
	w, h := m.width, m.height
	bodyH := h - 4
	lines, zs := m.header(w)
	top := len(lines)
	if m.help {
		// The KEYS overlay takes the whole body; nothing under it is
		// clickable (see handleMouse).
		zs.addBox(zonePanelOverlay, 0, top, w, bodyH)
		lines = append(lines, m.helpPanel(w, bodyH)...)
		return append(lines, m.statusLine(w), m.hintLine(w)), zs
	}
	if m.settings {
		// SETTINGS takes the whole body too; only its rows are clickable.
		panel, pz := m.settingsPanel(w, bodyH)
		zs.addBox(zonePanelOverlay, 0, top, w, bodyH)
		zs.addAt(0, top, pz)
		lines = append(lines, panel...)
		return append(lines, m.statusLine(w), m.hintLine(w)), zs
	}
	if m.boot {
		// The boot splash: nothing in the body is clickable.
		lines = append(lines, splash(w, bodyH)...)
		return append(lines, m.statusLine(w), m.hintLine(w)), zs
	}
	// The panels go under the zones drawn in them, which stay on top.
	if m.expanded {
		// NOW PLAYING takes the list panel's place too.
		playing, playingZones := m.nowPlaying(m.playerPanelWidth()-2, bodyH-2)
		zs.addBox(zonePanelPlayer, 0, top, w, bodyH)
		zs.addAt(1, top+1, playingZones.clip(w-2, bodyH-2))
		lines = append(lines, panel("NOW PLAYING", m.spectrumCode(), playing, w, bodyH, true)...)
		return append(lines, m.statusLine(w), m.hintLine(w)), zs
	}
	leftW := listPanelWidthFor(w)
	rightW := m.playerPanelWidth()

	left, leftZones := m.listPanel(leftW, bodyH)
	playing, playingZones := m.nowPlaying(rightW-2, bodyH-2)
	right := panel("NOW PLAYING", m.spectrumCode(), playing, rightW, bodyH, m.focus == areaPlayer)

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

// playerPanelWidth is the width of the NOW PLAYING panel, frame included,
// in the full layout: the whole screen when expanded, else what the list
// panel leaves.
func (m Model) playerPanelWidth() int {
	if m.expanded {
		return m.width
	}
	return m.width - listPanelWidthFor(m.width) - 1
}

// eqBarCount is how many bars the spectrum shows: one every other column
// inside the NOW PLAYING margins, at most eqBands.
func (m Model) eqBarCount() int {
	inner := m.playerPanelWidth() - 2 - 2*nowPlayingMargin
	return min(max((inner+1)/2, 0), eqBands)
}

// compactVolumeWidth is the widest volume row beside the transport
// buttons in the compact layout, and compactVolumeMin the narrowest, with
// its buttons and the narrowest meter.
const (
	compactVolumeWidth = 27
	compactVolumeMin   = 20
)

// playerMinWidth is the narrowest NOW PLAYING panel beside the list: room
// for the five transport buttons, tightly packed.
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
// header rule and, during the boot, the boot splash (see splash) the
// rest down to the status line; else the title line ends in
// the [<3] button, then the artist
// line, the transport row (packed), the volume row beside it while it fits
// and the rule over the list, which the expanded player leaves out. The
// one-row HUD of the full layout (hudRowCount) never fits here, narrower
// than fullMinWidth; the focus follows the zones drawn either way.
func (m Model) renderCompact() ([]string, zones) {
	w := m.width
	nav, zs := m.navLine(w)
	zs = zs.shifted(0, 1)
	lines := []string{m.wordmark() + "  " + m.statusTag(), nav}
	if m.help {
		zs.addBox(zonePanelOverlay, 0, len(lines), w, m.height-4)
		lines = append(lines, m.helpPanel(w, m.height-4)...)
		return append(lines, m.statusLine(w), m.hintLine(w)), zs
	}
	if m.settings {
		panel, pz := m.settingsPanel(w, m.height-4)
		zs.addBox(zonePanelOverlay, 0, len(lines), w, m.height-4)
		zs.addAt(0, len(lines), pz)
		lines = append(lines, panel...)
		return append(lines, m.statusLine(w), m.hintLine(w)), zs
	}
	if m.boot {
		lines = append(lines, splash(w, m.height-4)...)
		return append(lines, m.statusLine(w), m.hintLine(w)), zs
	}
	// The player lines (title to buttons) and the list below them are the
	// panels, under the zones drawn in them.
	var panels zones
	playerTop := len(lines)
	title, artist := m.titleLines()
	title, hz := m.heartTitle(title, w-1)
	zs.addAt(0, len(lines), hz)
	lines = append(lines, title, ansi.Truncate(artist, w-1, "…"))
	progress, barW := m.progressLine(w - 1)
	if m.seekable() {
		zs.add(zoneSeek, 1, len(lines), barW)
	}
	lines = append(lines, m.barMark()+progress)
	// The volume row takes the room of the PLAY label and the gaps when it
	// can fit beside all five buttons that way.
	transport, tz := m.transportBar(w-3-compactVolumeMin, true)
	withVolume := len(tz) == int(ctlExpand)+1
	if !withVolume {
		transport, tz = m.transportBar(w-1, true)
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
		stWarnBold.Render("▲ ACCESS DENIED"),
		"",
		stLabel.Render("NU11SIGNAL CANNOT REACH YOUR APPLE MUSIC LIBRARY."),
		stMuted.Render(strings.ToUpper(m.authDetail)),
		"",
		stLabel.Render("GRANT ACCESS IN SYSTEM SETTINGS › PRIVACY & SECURITY"),
		stLabel.Render("› MEDIA & APPLE MUSIC, THEN RESTART."),
		"",
		keyCap("Q") + stLabel.Render(" QUIT"),
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

// wordmark is the app's name as the header draws it.
func (m Model) wordmark() string { return stLabelBold.Render("NU11SIGNAL") }

// header is the title line over the nav bar.
func (m Model) header(w int) ([]string, zones) {
	var auth string
	switch m.auth {
	case authOK:
		auth = stOK.Render("AUTH OK")
	case authFailed:
		auth = stWarn.Render("AUTH FAIL")
	default:
		auth = stWarn.Render("LINKING")
	}
	sig := stOK.Render("▂▄▆█")
	if m.signalLost() {
		sig = stWarn.Render("▂") + stDim.Render("▄▆█") + stWarn.Render(" SIGNAL LOST")
	}
	right := auth + stDim.Render("  ▮  ") + stMuted.Render("SIG ") + sig +
		stDim.Render("  ▮  ") + stNumber.Render(m.now().Format("15:04:05"))

	// The wordmark sits centered, or as far right of center as the status
	// at the right edge leaves room for (left-aligned at the narrowest).
	mark := m.wordmark()
	markW, rightW := ansi.StringWidth(mark), ansi.StringWidth(right)
	at := max(min((w-markW)/2, w-rightW-1-markW), 0)
	gap := max(w-at-markW-rightW, 1)
	top := strings.Repeat(" ", at) + mark + strings.Repeat(" ", gap) + right

	nav, zs := m.navLine(w)
	return []string{top, nav}, zs.shifted(0, 1)
}

// navLine is the header rule carrying the nav bar: the PLAYLISTS and
// SEARCH tabs and, on a page, BACK. The net node readout (netNode) stays
// at the right edge, cut with … to the room left, while at least
// minNode cells of it fit.
//
//	◢◤◢◤ ╱ PLAYLISTS ╱ ╱ SEARCH ╱ ╱ BACK ╱ ─── NODE 7F // NC-GRID ──
func (m Model) navLine(w int) (string, zones) {
	const (
		mark    = "◢◤◢◤"
		gap     = " "
		nodeEnd = "──"
		// minRule is the rule kept between the bar and the readout, and
		// minNode the fewest cells of its text worth drawing.
		minRule = 1
		minNode = 6
	)
	lead := ansi.StringWidth(mark + gap)
	gapW := ansi.StringWidth(gap)
	bar, bz := buttonBar(m.navButtons(), w-lead-gapW)
	var zs zones
	zs.addAt(lead, 0, bz)
	rest := max(w-lead-ansi.StringWidth(bar)-gapW, 0)
	tail := stFrameDim.Render(strings.Repeat("─", rest))
	// The text sits between a space on each side and the closing rule.
	if room := rest - minRule - ansi.StringWidth(nodeEnd) - 2; room >= minNode {
		node := " " + ansi.Truncate(m.netNode(), room, "…") + " "
		tail = stFrameDim.Render(strings.Repeat("─", rest-ansi.StringWidth(node+nodeEnd))) +
			stMuted.Render(node) + stFrameDim.Render(nodeEnd)
	}
	return stAccent.Render(mark) + gap + bar + gap + tail, zs
}

// netNode is the nav bar's flavor text: the Night City net node the radio
// is patched through, one byte of the Model's seed in hex, so a session
// keeps its node and a fixed seed draws a fixed frame.
func (m Model) netNode() string {
	return fmt.Sprintf("NODE %02X // NC-GRID", mix(m.seed, netNodeSalt)&0xFF)
}

// netNodeSalt keeps netNode apart from the other hashes of the seed.
const netNodeSalt = 0x4E43

// spectrumCode is the NOW PLAYING panel's bottom label: where the rain's
// levels come from, the player's readings (LIVE), the decorative
// animation while playing (SIM), or nothing while paused or stopped
// (HOLD, the bars falling).
func (m Model) spectrumCode() string {
	switch _, live := m.liveSpectrum(); {
	case live:
		return "SPECTRUM LIVE"
	case m.isPlaying():
		return "SPECTRUM SIM"
	}
	return "SPECTRUM HOLD"
}

func (m Model) statusTag() string {
	switch {
	case m.signalLost():
		return stWarnBold.Render("▮ SIGNAL LOST")
	case !m.hasState:
		return stMuted.Render("▮ STANDBY")
	}
	switch m.state.Status {
	case playback.StatusPlaying:
		return stOKBold.Render("▮ PLAYING")
	case playback.StatusPaused:
		return stWarnBold.Render("▮ PAUSED")
	case playback.StatusSeeking:
		return stHi.Render("▮ SEEKING")
	}
	return stMuted.Render("▮ STANDBY")
}

// titleLines returns the styled title and artist lines of the now-playing
// display, including the signal-lost and idle states.
func (m Model) titleLines() (string, string) {
	switch {
	case m.signalLost():
		return stWarnBold.Render("SIGNAL LOST"), stLabel.Render("HELPER OFFLINE // RESTART NU11SIGNAL")
	case !m.hasState || m.state.Title == "":
		return stMuted.Render("NO CARRIER"), stDim.Render("OPEN A PLAYLIST WITH [ENTER]")
	}
	title := glitchText(strings.ToUpper(m.state.Title), m.glitch, mix(m.seed, m.animFrame))
	return stHiBold.Render(title), stText.Render(strings.ToUpper(m.state.Artist))
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
		return stNumber.Render(times), 0
	}
	elapsed := progressBar(pos, dur, barW)
	filled := strings.Count(elapsed, "▮")
	return stHi.Render(strings.Repeat("▮", filled)) + stDim.Render(strings.Repeat("▯", barW-filled)) +
		"  " + stNumber.Render(times), barW
}

// nowPlayingMargin is the room NOW PLAYING leaves on each side of the
// lines under its head.
const nowPlayingMargin = 1

// The rows of the NOW PLAYING inside from the head down to the progress
// bar, the others blank; the rows under them (the controls and the
// visualizer) depend on the width and the height. The head is the feed
// and the status tag: the panel's frame label already says NOW PLAYING.
// The content intro compares the artist, album and head rows (see
// introFieldRows).
const (
	npHeadRow = iota
	_
	npTitleRow
	npArtistRow
	npAlbumRow
	_
	npProgressRow
)

// nowPlaying renders the inside of the NOW PLAYING panel, iw x ih cells,
// with its zones: the progress bar (click to seek) and, under it, the
// controls: one row when the width holds them all, else the transport
// row (LOOP and EXPAND included) over the volume row (see hudControls).
func (m Model) nowPlaying(iw, ih int) ([]string, zones) {
	title, artist := m.titleLines()
	album := ""
	if m.hasState && !m.signalLost() {
		album = stMuted.Render(strings.ToUpper(m.state.Album))
	}
	feed, tag := m.feedLine(), m.statusTag()
	if room := m.headFeedWidth(iw); ansi.StringWidth(feed) > room {
		// The narrower panel beside the list cuts the feed and keeps the
		// status whole.
		feed = ansi.Truncate(feed, room, "…")
	}
	head := feed + strings.Repeat(" ", max(iw-1-ansi.StringWidth(feed)-ansi.StringWidth(tag), 1)) + tag

	// The lines under the head sit nowPlayingMargin cells in from each
	// side, and so do their zones.
	inner := iw - 2*nowPlayingMargin
	lines := make([]string, npProgressRow+1)
	lines[npHeadRow] = " " + head
	title, hz := m.heartTitle(title, inner)
	var zs zones
	zs.addAt(nowPlayingMargin, npTitleRow, hz)
	lines[npTitleRow], lines[npArtistRow], lines[npAlbumRow] = " "+title, " "+artist, " "+album
	progress, barW := m.progressLine(inner)
	if m.seekable() {
		zs.add(zoneSeek, nowPlayingMargin, npProgressRow, barW)
	}
	lines[npProgressRow] = m.barMark() + progress
	controls, cz := m.hudControls(inner)
	if ih > len(lines)+len(controls) {
		// The buttons keep their gap from the progress bar while it
		// leaves room for them all.
		lines = append(lines, "")
	}
	zs.addAt(nowPlayingMargin, len(lines), cz)
	for _, row := range controls {
		lines = append(lines, " "+row)
	}

	if eqRows := vizRows(ih, len(lines)); eqRows > 0 {
		// Sit the rain on the bottom edge of the panel.
		for len(lines)+eqRows < ih {
			lines = append(lines, "")
		}
		for _, row := range m.rain.Render(inner, eqRows) {
			lines = append(lines, " "+row)
		}
	}
	return lines, zs
}

// headFeedWidth is the room the head row of NOW PLAYING, iw cells inside,
// leaves the feed: all but the margin, the status tag and the space
// before it. The content intro compares those cells only, so a new
// status never scrambles (see introRegions).
func (m Model) headFeedWidth(iw int) int {
	return max(iw-nowPlayingMargin-ansi.StringWidth(m.statusTag())-1, 0)
}

// barMark is the cell before the progress bar: a marker while the bar has
// the focus.
func (m Model) barMark() string {
	if m.barFocused() && m.seekable() {
		return stFocusBold.Render("▸")
	}
	return " "
}

// feedLine names where the music comes from, at the head of NOW PLAYING:
// a station's frequency or the catalog.
func (m Model) feedLine() string {
	for i, s := range m.stations {
		if s.ID == m.playingStation {
			return stAccent.Render("▞ "+frequency(i)+" MHZ") + stMuted.Render(" // "+strings.ToUpper(s.Name))
		}
	}
	if m.hasState && m.state.Title != "" {
		return stAccent.Render("▞ CATALOG FEED") + stMuted.Render(" // DIRECT")
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
	nameStyle := stText
	if onAir {
		nameStyle = stOnAir
	}
	text := "   " + stMuted.Render(frequency(i)) + "  " + nameStyle.Render(name)
	return fit(fit(text, textWidth)+stOnAir.Render(mark), w)
}

func (m Model) statusLine(w int) string {
	if m.status != "" {
		return stWarn.Render("▲ " + strings.ToUpper(m.status))
	}
	return stDim.Render(fit("◢◤◢◤ "+m.idleStatus(), w))
}

// idleStatus is the status line without a message: the song NEXT moves
// to, when the model knows it (see upNext), else which volume the
// player drives (once it says) and whether the signal effects are on.
func (m Model) idleStatus() string {
	if s, ok := m.upNext(); ok {
		next := "UP NEXT // " + strings.ToUpper(cleanLine(s.Title))
		if s.Artist != "" {
			next += " · " + strings.ToUpper(cleanLine(s.Artist))
		}
		return next
	}
	fx := "FX OFF"
	if m.fx.on {
		fx = "FX ON"
	}
	switch m.volumeMode {
	case playback.VolumeApp:
		return "APP VOLUME // " + fx
	case playback.VolumeSystem:
		return "SYS VOLUME // " + fx
	}
	return fx
}

func keyCap(k string) string { return stAccent.Render("[" + k + "]") }

// hintLine lays out key hints, dropping lower-priority ones (never the
// last, quit, nor KEYS before it) until they fit.
func (m Model) hintLine(w int) string {
	hints := playerHints
	switch kind := m.top().kind; {
	case m.quitAsk:
		hints = quitModalHints
	case m.help:
		hints = helpOverlayHints
	case m.settings:
		hints = settingsOverlayHints
	case m.focus == areaTabs:
		hints = tabsFocusHints(m.keysTyped())
	case m.focus == areaPlayer:
		hints = playerFocusHints(m.expanded, m.keysTyped())
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
	return fitHints(hints, w)
}

// fitHints joins hints into a line of at most w cells, leaving out
// SETTINGS, then shortening their keys and then dropping lower-priority ones (never the last, nor the
// KEYS hint right before it, but when only the two are left) until it
// fits. Each hint is styled once: the View runs on every frame, and
// styling every candidate line anew made the hints most of its cost.
func fitHints(hints []hint, w int) string {
	parts, width := renderHints(hints)
	if i := slices.Index(hints, settingsHint); i >= 0 && width > w {
		// SETTINGS shows only where the whole footer fits: it goes
		// before any other hint is shortened or dropped.
		hints = slices.Delete(slices.Clone(hints), i, i+1)
		parts, width = renderHints(hints)
	}
	if width > w {
		parts, width = renderHints(shortHints(hints))
	}
	keep := 1
	if n := len(hints); n >= 2 && hints[n-2] == helpHint {
		keep = 2
	}
	for len(parts) > 1 && width > w {
		drop := max(len(parts)-1-keep, 0)
		width -= parts[drop].width + len(hintGap)
		parts = append(parts[:drop], parts[drop+1:]...)
	}
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = p.text
	}
	return strings.Join(cells, hintGap)
}

// hintGap separates two key hints.
const hintGap = "  "

// renderedHint is a styled key hint and its width in cells.
type renderedHint struct {
	text  string
	width int
}

// renderHints styles each hint once and returns them with the width of
// the whole line they make, joined by hintGap.
func renderHints(hs []hint) ([]renderedHint, int) {
	parts := make([]renderedHint, len(hs))
	width := 0
	for i, h := range hs {
		// The footer sits in the background: the content keeps the
		// accent and label colors.
		text := stMuted.Render("["+h.key+"]") + " " + stMuted.Render(h.label)
		parts[i] = renderedHint{text: text, width: ansi.StringWidth(text)}
		if i > 0 {
			width += len(hintGap)
		}
		width += parts[i].width
	}
	return parts, width
}

// shortHints names only DEL of the recent delete keys, the space key
// PLAY and the arrows walking the player or the tabs PICK: the room they
// free keeps ESC BACK (on the pages), SEEK (on the playlists) and F
// RESTORE (on the expanded player) beside KEYS and QUIT in an 80-column
// footer.
func shortHints(hs []hint) []hint {
	out := slices.Clone(hs)
	for i, h := range out {
		switch {
		case h.key == recentDeleteKeys:
			out[i].key = "DEL"
		case h.key == "SPACE" && h.label == "PLAY/PAUSE":
			out[i].label = "PLAY"
		case h.key == "←→" && h.label == "SELECT":
			out[i].label = "PICK"
		}
	}
	return out
}
