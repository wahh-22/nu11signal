package radio

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/history"
	"github.com/wahh-22/nu11signal/internal/playback"
)

// searchState is the catalog search behind the search view.
type searchState struct {
	// seq numbers input edits and immediate searches; debounce ticks and
	// answers carrying an older number are dropped.
	seq uint64
	// term is the term the results (or err) answer; empty when none. They
	// are shown only while the input still reads term.
	term    string
	results playback.SearchResults
	err     error
	loading bool
	// cancel cancels the search in flight; nil when none.
	cancel context.CancelFunc
}

// Messages of the search view.
type (
	recentsMsg struct {
		terms []string
		err   error
	}
	recentSavedMsg    struct{ err error }
	recentEditedMsg   struct{ err error }
	searchDebounceMsg struct{ seq uint64 }
	catalogMsg        struct {
		seq     uint64
		term    string
		results playback.SearchResults
		err     error
	}
)

// searchRowKind is what a search view row stands for.
type searchRowKind int

const (
	rowRecent searchRowKind = iota
	rowSuggestion
	rowArtist
	rowSong
)

// recentCrossMinWidth is the narrowest row that still gets a ✕ to delete
// its recent term; recentCross is that ✕, drawn in the last
// recentCrossWidth cells of the row, where its zone is.
const (
	recentCrossMinWidth = 8
	recentCross         = " ✕"
	recentCrossWidth    = 2
)

// searchRow is one selectable row of the search view.
type searchRow struct {
	kind   searchRowKind
	term   string // recent term or suggestion
	artist playback.Artist
	song   playback.Song
}

func (m Model) loadRecentsCmd() tea.Cmd {
	store := m.recentsStore
	return func() tea.Msg {
		terms, err := store.Load()
		return recentsMsg{terms: terms, err: err}
	}
}

// onRecents shows the stored terms. Terms remembered before they arrived
// stay in front.
func (m Model) onRecents(msg recentsMsg) Model {
	if msg.err != nil {
		m.setStatus("RECENT SEARCHES UNAVAILABLE // " + msg.err.Error())
		return m
	}
	terms := msg.terms
	for i := len(m.recents) - 1; i >= 0; i-- {
		terms = history.Push(terms, m.recents[i])
	}
	m.recents = terms
	return m
}

// remember records term as the most recent search. The list on screen
// changes at once; a failure to store it only reaches the status line.
func (m *Model) remember(term string) tea.Cmd {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil
	}
	m.recents = history.Push(m.recents, term)
	add := m.recentsWrites.write(func(r history.Recents) error { return r.Add(term) })
	return func() tea.Msg { return recentSavedMsg{err: add()} }
}

// deleteRecentAt deletes the recent term on search row i (a no-op on any
// other row). The list on screen changes at once. The cursor stays on the
// row that takes the deleted one's place, except after the last term,
// where it moves up to the new last term, and onto the input once no term
// is left. A failure to store the change only reaches the status line.
func (m Model) deleteRecentAt(i int) (Model, tea.Cmd) {
	rows := m.searchRows()
	if i < 0 || i >= len(rows) || rows[i].kind != rowRecent {
		return m, nil
	}
	term := rows[i].term
	m.recents = history.Without(m.recents, term)
	switch cur := m.cursor(); {
	case len(m.recents) == 0:
		m.setCursor(-1)
	case cur > i:
		m.setCursor(cur - 1)
	case cur == i:
		m.setCursor(min(i, len(m.recents)-1))
	}
	remove := m.recentsWrites.write(func(r history.Recents) error { return r.Remove(term) })
	return m, func() tea.Msg { return recentEditedMsg{err: remove()} }
}

// resumeOrOpenSearch brings back the parked search branch exactly as it
// was left (top page and cursors); with nothing parked, it pushes a fresh
// search view with an empty input (showing recent terms). / and tab from
// the stations both call it. Without a catalog it only says so.
func (m Model) resumeOrOpenSearch() (tea.Model, tea.Cmd) {
	if !m.canSearch() {
		m.setStatus(noSearch)
		return m, nil
	}
	if !m.restoreBranch() {
		m.resetSearch()
		m.input.Reset()
		m.push(frame{kind: viewSearch, cursor: -1})
		return m, m.input.Focus()
	}
	if m.top().kind != viewSearch {
		// The search resumes when esc or / goes back to it.
		return m, nil
	}
	return m, tea.Batch(m.input.Focus(), m.resumeSearch())
}

// searchAgain is / on a page of the search branch: the pages above its
// search view are dropped (cancelling their loads) and the input takes the
// keys again, with the term kept for editing, as starting a new search.
//
// On the PLAYLISTS branch (a library playlist page) no search view is
// below: the pages are dropped and the parked search, or a fresh one,
// opens.
func (m Model) searchAgain() (Model, tea.Cmd) {
	if !slices.ContainsFunc(m.stack, func(f frame) bool { return f.kind == viewSearch }) {
		m.popToRoot()
		next, cmd := m.resumeOrOpenSearch()
		return next.(Model), cmd
	}
	for m.top().kind != viewSearch {
		m.pop()
	}
	m.setCursor(-1)
	m.input.CursorEnd()
	return m, tea.Batch(m.input.Focus(), m.resumeSearch())
}

// resumeSearch runs the search for the typed term again when leaving the
// view stopped it before it answered.
func (m *Model) resumeSearch() tea.Cmd {
	if term := m.inputTerm(); longEnough(term) && term != m.search.term && !m.search.loading {
		return m.startSearch(term)
	}
	return nil
}

// handleSearchKey handles keys while the search view is on top. Text keys
// go to the input, so the player's letter shortcuts are off here. With a
// row selected, ctrl+d and delete delete a recent term instead of editing
// the input, and on a song row l and a love it and add it to a playlist,
// and g opens its SONG view.
func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc:
		m.stopSearch()
		m.pop()
		m.input.Blur()
		return m, nil
	case keyTab:
		// The branch is parked for / or tab from the stations; esc above
		// closes it instead.
		m.stopSearch()
		m.parkBranch()
		m.input.Blur()
		return m, nil
	case keyUp:
		// Up from the first row selects the input; from the input, the
		// tabs.
		if m.atListTop() {
			m.focusTabs()
			return m, nil
		}
		m.setCursor(m.cursor() - 1)
		return m, nil
	case keyDown:
		m.setCursor(min(m.cursor()+1, len(m.searchRows())-1))
		return m, nil
	case keyEnter:
		return m.searchEnter()
	case keyRight, keyLeft:
		// On the input the arrows move its caret; on a row, → crosses to
		// the player and ← has nowhere to go.
		if m.cursor() >= 0 {
			if msg.String() == keyRight {
				m.focusPlayer(ctlPlay)
			}
			return m, nil
		}
	case keyDelete, keyDeleteAlt:
		if cur := m.cursor(); cur >= 0 {
			return m.deleteRecentAt(cur)
		}
	case keyLove, keyAdd, keyAlbum:
		if s, ok := m.selectedSong(); ok {
			switch msg.String() {
			case keyLove:
				return m.loveTarget()
			case keyAdd:
				return m.addTarget()
			}
			save := m.remember(m.search.term)
			next, open := m.openSong(s)
			return next, tea.Batch(open, save)
		}
	}
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() == before {
		return m, cmd
	}
	return m, tea.Batch(cmd, m.inputChanged())
}

// inputChanged stops the pending search and, for a long enough term,
// schedules a new one after the debounce delay. Until it answers, the
// results of the previous term are hidden (see searchRows).
func (m *Model) inputChanged() tea.Cmd {
	m.setCursor(-1)
	if !longEnough(m.inputTerm()) {
		m.resetSearch()
		return nil
	}
	m.stopSearch()
	seq := m.search.seq
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg { return searchDebounceMsg{seq: seq} })
}

func (m Model) inputTerm() string { return strings.TrimSpace(m.input.Value()) }

// longEnough reports whether term is long enough to search.
func longEnough(term string) bool { return utf8.RuneCountInString(term) >= minSearchRunes }

func (m Model) onSearchDebounce(msg searchDebounceMsg) (tea.Model, tea.Cmd) {
	term := m.inputTerm()
	if msg.seq != m.search.seq || !longEnough(term) {
		return m, nil
	}
	cmd := m.startSearch(term)
	return m, cmd
}

// searchNow searches the typed term without waiting for the debounce
// delay; a term too short to search only keeps the keep-typing notice.
func (m Model) searchNow() (Model, tea.Cmd) {
	m.setCursor(-1)
	term := m.inputTerm()
	if !longEnough(term) {
		m.resetSearch()
		return m, nil
	}
	cmd := m.startSearch(term)
	return m, cmd
}

// startSearch stops the pending search and starts one for term.
func (m *Model) startSearch(term string) tea.Cmd {
	m.stopSearch()
	ctx, cancel := m.ctx()
	m.search.cancel = cancel
	m.search.loading = true
	seq := m.search.seq
	player := m.player
	return func() tea.Msg {
		defer cancel()
		res, err := player.SearchCatalog(ctx, term, searchLimit)
		return catalogMsg{seq: seq, term: term, results: res, err: err}
	}
}

// stopSearch cancels the search in flight and drops the answers and
// debounce ticks still on their way. The last results stay, for the term
// they answer.
func (m *Model) stopSearch() {
	if m.search.cancel != nil {
		m.search.cancel()
	}
	m.search.seq++
	m.search.cancel = nil
	m.search.loading = false
}

// resetSearch stops the search and forgets its results, as for an input
// too short to search.
func (m *Model) resetSearch() {
	m.stopSearch()
	m.search = searchState{seq: m.search.seq}
}

// resultsAnswerInput reports whether the results (or error) answer the
// term in the input. They do not while the term is too short to search or
// while the input changed and the new search has not answered yet; stale
// results are then neither shown nor selectable.
func (m Model) resultsAnswerInput() bool {
	term := m.inputTerm()
	return longEnough(term) && term == m.search.term
}

// onCatalog shows the answer to the latest search; older answers are
// dropped.
func (m Model) onCatalog(msg catalogMsg) Model {
	if msg.seq != m.search.seq {
		return m
	}
	m.search = searchState{seq: msg.seq, term: msg.term, results: cleanResults(msg.results), err: msg.err}
	if msg.err != nil {
		// The panel is narrow; the status line has room for the reason.
		m.setStatus("CATALOG SCAN FAILED // " + msg.err.Error())
	}
	if m.top().kind == viewSearch {
		m.setCursor(-1) // the rows changed underneath the cursor
	}
	return m
}

// searchEnter acts on the selected row, or on the typed term when the
// input is selected: the term, a recent term or a suggestion opens its
// RESULTS; an artist opens its page; a song plays, the other songs listed
// queued around it.
func (m Model) searchEnter() (tea.Model, tea.Cmd) {
	rows := m.searchRows()
	cur := m.cursor()
	if cur < 0 || cur >= len(rows) {
		if !longEnough(m.inputTerm()) {
			return m, nil
		}
		return m.openResults(m.inputTerm())
	}
	row := rows[cur]
	switch row.kind {
	case rowArtist:
		save := m.remember(m.search.term)
		next, open := m.openArtist(row.artist)
		return next, tea.Batch(open, save)
	case rowSong:
		// The song rows listed are queued in order and play from this
		// one, the search staying on screen.
		save := m.remember(m.search.term)
		var songs []playback.Song
		start := 0
		for i, r := range rows {
			if r.kind != rowSong {
				continue
			}
			if i == cur {
				start = len(songs)
			}
			songs = append(songs, r.song)
		}
		next, play := m.playSongs(songs, start)
		return next, tea.Batch(play, save)
	}
	m.input.SetValue(row.term)
	m.input.CursorEnd()
	if !longEnough(m.inputTerm()) {
		// A recent term too short to search only fills the input.
		return m.searchNow()
	}
	return m.openResults(m.inputTerm())
}

// searchRows lists the selectable rows: recent terms while the input is
// empty, otherwise the results for the typed term as Apple Music orders
// them (suggestions, artists, songs). Results for another term (the input
// changed and the new search has not answered yet) are not rows.
func (m Model) searchRows() []searchRow {
	var rows []searchRow
	term := m.inputTerm()
	if term == "" {
		for _, t := range m.recents {
			rows = append(rows, searchRow{kind: rowRecent, term: t})
		}
		return rows
	}
	if !m.resultsAnswerInput() || m.search.err != nil {
		return nil
	}
	res := m.search.results
	for _, s := range res.Suggestions {
		rows = append(rows, searchRow{kind: rowSuggestion, term: s})
	}
	for _, a := range res.Artists {
		rows = append(rows, searchRow{kind: rowArtist, artist: a})
	}
	for _, s := range res.Songs {
		rows = append(rows, searchRow{kind: rowSong, song: s})
	}
	return rows
}

// recentSelected reports whether the cursor is on a recent term.
func (m Model) recentSelected() bool {
	rows, cur := m.searchRows(), m.cursor()
	return cur >= 0 && cur < len(rows) && rows[cur].kind == rowRecent
}

// searchCode is the serial code in the search panel's bottom edge.
func (m Model) searchCode() string {
	term := m.inputTerm()
	switch {
	case term == "":
		return fmt.Sprintf("RECENT %02d", len(m.recents))
	case m.search.loading || (longEnough(term) && !m.resultsAnswerInput()):
		return "SCANNING"
	case m.search.err != nil:
		return "ERR"
	}
	return fmt.Sprintf("HITS %02d", len(m.searchRows()))
}

// inputWidth is the width of the search input's text, which sits in the
// list panel (full layout) or across the screen (compact layout) after a
// two-cell glyph.
func (m Model) inputWidth() int {
	w := m.width
	if w >= fullMinWidth && m.height >= fullMinHeight {
		w = listPanelWidthFor(w) - 2
	}
	return max(w-3, 1)
}

// searchBody renders the search view in w x h cells: the input, a rule and
// the rows, scrolled so the cursor stays on screen. Every line is exactly w
// cells wide. Its zones are the input and the rows shown, with the ✕ of
// each recent term and the favorite mark and + of the selected song.
func (m Model) searchBody(w, h int) ([]string, zones) {
	if h <= 0 || w <= 0 {
		return nil, nil
	}
	var zs zones
	zs.add(zoneInput, 0, 0, w)
	lines := []string{fit(stAccent.Render("⌕ ")+m.input.View(), w)}
	if h == 1 {
		return lines, zs
	}
	lines = append(lines, stFrameDim.Render(strings.Repeat("─", w)))
	room := h - 2

	term := m.inputTerm()
	rows := m.searchRows()
	var notice string
	switch {
	case term == "":
		if room > 0 {
			lines = append(lines, fit(" "+stMuted.Render(spaced("RECENT")), w))
			room--
		}
		if len(rows) == 0 {
			notice = stDim.Render("NO RECENT SEARCHES")
		}
	case !longEnough(term):
		notice = stDim.Render(fmt.Sprintf("KEEP TYPING // %d+ CHARACTERS", minSearchRunes))
	case !m.resultsAnswerInput():
		notice = stDim.Render("SCANNING CATALOG...")
	case m.search.err != nil:
		notice = stWarn.Render("▲ SCAN FAILED")
	case len(rows) == 0:
		notice = stDim.Render(fmt.Sprintf("NO SIGNAL FOR %q", strings.ToUpper(m.search.term)))
	}
	if notice != "" {
		if room > 0 {
			lines = append(lines, fit(" "+notice, w))
		}
		return lines, zs
	}

	cur := m.cursor()
	offset := max(0, cur-room+1)
	for i := offset; i < len(rows) && i-offset < room; i++ {
		zs.add(rowZone(i), 0, len(lines), w)
		if rows[i].kind == rowRecent && w >= recentCrossMinWidth {
			// On top of the row: the ✕ in its last cells.
			zs.add(recentDeleteZone(i), w-recentCrossWidth, len(lines), recentCrossWidth)
		}
		if rows[i].kind == rowSong && i == cur && w >= songActionsMinWidth {
			addActionZones(&zs, w, len(lines))
		}
		lines = append(lines, m.searchRowLine(rows[i], i == cur, w))
	}
	return lines, zs
}

func (m Model) searchRowLine(r searchRow, selected bool, w int) string {
	switch r.kind {
	case rowArtist:
		tag := "ARTIST"
		if len(r.artist.Genres) > 0 {
			tag += " · " + strings.ToUpper(r.artist.Genres[0])
		}
		name := strings.ToUpper(r.artist.Name)
		return searchLine("◆", stAccent, name, styled(stHi), tag, selected, w)
	case rowSong:
		title := strings.ToUpper(r.song.Title)
		tag := "SONG"
		if r.song.Artist != "" {
			tag += " · " + strings.ToUpper(r.song.Artist)
		}
		line, _ := m.songRow(r.song, selected, w, func(w int) string {
			return searchLine("♪", stHi, title, styled(stText), tag, selected, w)
		})
		return line
	case rowSuggestion:
		text := strings.ToUpper(r.term)
		prefix := strings.ToUpper(m.search.term)
		return searchLine("⌕", stMuted, text, func(t string) string { return highlightPrefix(t, prefix) }, "", selected, w)
	}
	text := strings.ToUpper(r.term)
	if w < recentCrossMinWidth {
		return searchLine("⌕", stMuted, text, styled(stText), "", selected, w)
	}
	// A recent term ends in the ✕ that deletes it.
	cross := stLabel.Render(recentCross)
	if selected {
		cross = stSelected.Render(recentCross)
	}
	return searchLine("⌕", stMuted, text, styled(stText), "", selected, w-recentCrossWidth) + cross
}

// searchLine lays out one row in exactly w cells: a selection mark, a
// glyph, the text and a muted tag such as "ARTIST · ROCK". The text keeps
// priority, but is shortened so that the tag's first word stays readable.
func searchLine(glyph string, glyphStyle lipgloss.Style, text string, style func(string) string, tag string, selected bool, w int) string {
	head := " " + glyph + " "
	room := w - ansi.StringWidth(head)
	if tag != "" {
		kind, _, _ := strings.Cut(tag, " ")
		if textRoom := room - 2 - ansi.StringWidth(kind); textRoom > 0 {
			text = ansi.Truncate(text, textRoom, "…")
		}
	}
	if selected {
		plain := "▌" + glyph + " " + text
		if tag != "" {
			plain += "  " + tag
		}
		return stSelected.Render(fit(plain, w))
	}
	line := " " + glyphStyle.Render(glyph) + " " + style(text)
	if tag != "" {
		line += "  " + stMuted.Render(tag)
	}
	return fit(line, w)
}

// highlightPrefix renders s with the part matching prefix in yellow, as
// Apple Music bolds what was typed.
func highlightPrefix(s, prefix string) string {
	n := min(len(prefix), len(s))
	if n == 0 || s[:n] != prefix[:n] {
		return stText.Render(s)
	}
	return stAccent.Render(s[:n]) + stText.Render(s[n:])
}

// styled adapts a style to searchLine's text renderer.
func styled(st lipgloss.Style) func(string) string {
	return func(s string) string { return st.Render(s) }
}
