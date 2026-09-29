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
	// term is the term the shown results (or err) answer; empty when none.
	term    string
	results playback.SearchResults
	err     error
	loading bool
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
	if fresh {
		m.input.Reset()
		m.search = searchState{seq: m.search.seq + 1}
	}
	m.push(frame{kind: viewSearch, cursor: -1})
	return m, m.input.Focus()
}

// handleSearchKey handles keys while the search view is on top. Text keys
// go to the input, so the player's letter shortcuts are off here.
func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc:
		m.pop()
		m.input.Blur()
		return m, nil
	case keyTab:
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

// inputChanged invalidates pending searches and, for a long enough term,
// schedules a new one after the debounce delay.
func (m *Model) inputChanged() tea.Cmd {
	m.search.seq++
	m.setCursor(-1)
	if utf8.RuneCountInString(m.inputTerm()) < minSearchRunes {
		m.search = searchState{seq: m.search.seq}
		return nil
	}
	seq := m.search.seq
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg { return searchDebounceMsg{seq: seq} })
}

func (m Model) inputTerm() string { return strings.TrimSpace(m.input.Value()) }

func (m Model) onSearchDebounce(msg searchDebounceMsg) (tea.Model, tea.Cmd) {
	term := m.inputTerm()
	if msg.seq != m.search.seq || utf8.RuneCountInString(term) < minSearchRunes {
		return m, nil
	}
	cmd := m.startSearch(term)
	return m, cmd
}

// searchNow searches term without waiting for the debounce delay.
func (m Model) searchNow(term string) (Model, tea.Cmd) {
	m.search.seq++
	m.setCursor(-1)
	cmd := m.startSearch(term)
	return m, cmd
}

func (m *Model) startSearch(term string) tea.Cmd {
	m.search.loading = true
	seq := m.search.seq
	return func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		res, err := m.player.SearchCatalog(ctx, term, searchLimit)
		return catalogMsg{seq: seq, term: term, results: res, err: err}
	}
}

// onCatalog shows the answer to the latest search; older answers are
// dropped.
func (m Model) onCatalog(msg catalogMsg) Model {
	if msg.seq != m.search.seq {
		return m
	}
	m.search = searchState{seq: msg.seq, term: msg.term, results: msg.results, err: msg.err}
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
		term := m.inputTerm()
		if term == "" {
			return m, nil
		}
		save := m.remember(term)
		m, search := m.searchNow(term)
		return m, tea.Batch(search, save)
	}
	row := rows[cur]
	switch row.kind {
	case rowArtist:
		save := m.remember(m.search.term)
		m, open := m.openArtist(row.artist)
		return m, tea.Batch(open, save)
	case rowSong:
		save := m.remember(m.search.term)
		id := row.song.ID
		m.playSeq++
		play := m.playCmd(m.playSeq, "PLAY", "", func(ctx context.Context) error {
			return m.player.PlaySongs(ctx, []string{id}, 0)
		})
		return m, tea.Batch(play, save)
	}
	m.input.SetValue(row.term)
	m.input.CursorEnd()
	return m.searchNow(row.term)
}

// openArtist is where the ARTIST view is to be pushed; until it exists the
// selection is only announced.
func (m Model) openArtist(a playback.Artist) (Model, tea.Cmd) {
	m.setStatus("ARTIST PAGE COMING SOON // " + strings.ToUpper(a.Name))
	return m, nil
}

// searchRows lists the selectable rows: recent terms while the input is
// empty, otherwise the latest results as Apple Music orders them
// (suggestions, artists, songs).
func (m Model) searchRows() []searchRow {
	var rows []searchRow
	if m.inputTerm() == "" {
		for _, t := range m.recents {
			rows = append(rows, searchRow{kind: rowRecent, term: t})
		}
		return rows
	}
	if m.search.term == "" || m.search.err != nil {
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
	switch {
	case m.search.loading:
		return "SCANNING"
	case m.inputTerm() == "":
		return fmt.Sprintf("RECENT %02d", len(m.recents))
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
		w = listPanelWidth(w) - 2
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
	case utf8.RuneCountInString(term) < minSearchRunes:
		notice = stDim.Render("KEEP TYPING // 2+ CHARACTERS")
	case m.search.err != nil:
		notice = stYellow.Render("▲ SCAN FAILED")
	case m.search.term == "":
		notice = stDim.Render("SCANNING CATALOG...")
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
