package radio

import (
	"context"
	"fmt"
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
	store := m.recentsStore
	return func() tea.Msg { return recentSavedMsg{err: store.Add(term)} }
}

// openSearch pushes the search view with the input focused. fresh starts
// an empty query (showing recent terms); otherwise the last query and its
// results come back.
func (m Model) openSearch(fresh bool) (tea.Model, tea.Cmd) {
	var search tea.Cmd
	if fresh {
		m.resetSearch()
		m.input.Reset()
	} else if term := m.inputTerm(); longEnough(term) && term != m.search.term {
		// Leaving the view stopped the search for this term; resume it.
		search = m.startSearch(term)
	}
	m.push(frame{kind: viewSearch, cursor: -1})
	return m, tea.Batch(m.input.Focus(), search)
}

// handleSearchKey handles keys while the search view is on top. Text keys
// go to the input, so the player's letter shortcuts are off here.
func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc:
		m.stopSearch()
		m.pop()
		m.input.Blur()
		return m, nil
	case keyTab:
		m.stopSearch()
		m.popToRoot()
		m.input.Blur()
		return m, nil
	case keyUp:
		m.setCursor(max(m.cursor()-1, -1))
		return m, nil
	case keyDown:
		m.setCursor(min(m.cursor()+1, len(m.searchRows())-1))
		return m, nil
	case keyEnter:
		return m.searchEnter()
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
// input is selected.
func (m Model) searchEnter() (tea.Model, tea.Cmd) {
	rows := m.searchRows()
	cur := m.cursor()
	if cur < 0 || cur >= len(rows) {
		if !longEnough(m.inputTerm()) {
			return m, nil
		}
		save := m.remember(m.inputTerm())
		next, search := m.searchNow()
		return next, tea.Batch(search, save)
	}
	row := rows[cur]
	switch row.kind {
	case rowArtist:
		save := m.remember(m.search.term)
		next, open := m.openArtist(row.artist)
		return next, tea.Batch(open, save)
	case rowSong:
		save := m.remember(m.search.term)
		next, open := m.openSong(row.song)
		return next, tea.Batch(open, save)
	}
	m.input.SetValue(row.term)
	m.input.CursorEnd()
	return m.searchNow()
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
		w = stationPanelWidth(w) - 2
	}
	return max(w-3, 1)
}

// searchBody renders the search view in w x h cells: the input, a rule and
// the rows, scrolled so the cursor stays on screen. Every line is exactly w
// cells wide.
func (m Model) searchBody(w, h int) []string {
	if h <= 0 || w <= 0 {
		return nil
	}
	lines := []string{fit(stYellow.Render("⌕ ")+m.input.View(), w)}
	if h == 1 {
		return lines
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
		notice = stYellow.Render("▲ SCAN FAILED")
	case len(rows) == 0:
		notice = stDim.Render(fmt.Sprintf("NO SIGNAL FOR %q", strings.ToUpper(m.search.term)))
	}
	if notice != "" {
		if room > 0 {
			lines = append(lines, fit(" "+notice, w))
		}
		return lines
	}

	cur := m.cursor()
	offset := max(0, cur-room+1)
	for i := offset; i < len(rows) && i-offset < room; i++ {
		lines = append(lines, m.searchRowLine(rows[i], i == cur, w))
	}
	return lines
}

func (m Model) searchRowLine(r searchRow, selected bool, w int) string {
	switch r.kind {
	case rowArtist:
		tag := "ARTIST"
		if len(r.artist.Genres) > 0 {
			tag += " · " + strings.ToUpper(r.artist.Genres[0])
		}
		name := strings.ToUpper(r.artist.Name)
		return searchLine("◆", stYellow, name, styled(stCyan), tag, selected, w)
	case rowSong:
		title := strings.ToUpper(r.song.Title)
		tag := "SONG"
		if r.song.Artist != "" {
			tag += " · " + strings.ToUpper(r.song.Artist)
		}
		return searchLine("♪", stCyan, title, styled(stRed), tag, selected, w)
	case rowSuggestion:
		text := strings.ToUpper(r.term)
		prefix := strings.ToUpper(m.search.term)
		return searchLine("⌕", stMuted, text, func(t string) string { return highlightPrefix(t, prefix) }, "", selected, w)
	}
	return searchLine("⌕", stMuted, strings.ToUpper(r.term), styled(stRed), "", selected, w)
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
		return stRed.Render(s)
	}
	return stYellow.Render(s[:n]) + stRed.Render(s[n:])
}

// styled adapts a style to searchLine's text renderer.
func styled(st lipgloss.Style) func(string) string {
	return func(s string) string { return st.Render(s) }
}
