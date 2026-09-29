package radio

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// resultsPage is the state of one RESULTS view entry on the navigation
// stack: the full catalog search for a term, as Apple Music shows it after
// a search is submitted. It lives in its frame, so popping the page drops
// it.
type resultsPage struct {
	// term is the term searched; it heads the page.
	term string
	// seq numbers the load; answers for another number are dropped.
	seq     uint64
	loading bool
	err     error
	found   playback.SearchResults
	// cancel cancels the load in flight; nil when none.
	cancel context.CancelFunc
}

// resultsMsg answers results load number seq.
type resultsMsg struct {
	seq   uint64
	found playback.SearchResults
	err   error
}

// openResults records term as a recent search and pushes the RESULTS view
// for it. The search view below keeps the term in its input for esc; its
// live search stops, as its rows no longer answer the input, and resumes
// on the way back (see resumeSearch).
func (m Model) openResults(term string) (Model, tea.Cmd) {
	save := m.remember(term)
	m.stopSearch()
	m.setCursor(-1)
	m.input.Blur()
	m.push(frame{kind: viewResults})
	load := m.loadResults(term)
	return m, tea.Batch(load, save)
}

// loadResults (re)loads the page on top of the stack, which must be a
// results page, for term.
func (m *Model) loadResults(term string) tea.Cmd {
	m.resultsSeq++
	seq := m.resultsSeq
	ctx, cancel := m.ctx()
	m.setTop(frame{kind: viewResults, results: resultsPage{term: term, seq: seq, loading: true, cancel: cancel}})
	player := m.player
	return func() tea.Msg {
		defer cancel()
		res, err := player.SearchCatalog(ctx, term, resultsLimit)
		return resultsMsg{seq: seq, found: res, err: err}
	}
}

// onResults fills the page the answer belongs to, even while parked;
// answers for a page that was left or reloaded are dropped.
func (m Model) onResults(msg resultsMsg) Model {
	m.settleFrame(func(f frame) bool {
		return f.kind == viewResults && f.results.seq == msg.seq && f.results.loading
	}, func(f frame) frame {
		f.results.loading, f.results.cancel = false, nil
		f.results.found, f.results.err = cleanResults(msg.found), msg.err
		f.cursor = 0
		return f
	})
	return m
}

// resultsEnter opens the view of the selected row: an artist's page, an
// album, a song's SONG view or a playlist.
func (m Model) resultsEnter() (Model, tea.Cmd) {
	items := m.resultItems()
	cur := m.cursor()
	if cur < 0 || cur >= len(items) {
		return m, nil
	}
	switch it := items[cur]; it.Kind {
	case playback.ItemArtist:
		return m.openArtist(it.Artist)
	case playback.ItemAlbum:
		return m.openAlbum(it.Album)
	case playback.ItemSong:
		return m.openSong(it.Song)
	case playback.ItemPlaylist:
		return m.openPlaylist(it.Playlist)
	}
	return m, nil
}

// resultItems lists the selectable rows of the results page on top.
func (m Model) resultItems() []playback.SearchItem {
	_, items := m.resultsLayout(m.listBodyWidth())
	return items
}

// resultsLayout lays out the loaded results page on top in w cells: its
// sections in Apple Music order, empty ones left out, and the selectable
// rows. A top result is tagged with its kind; the rows of the other
// sections need no tag.
func (m Model) resultsLayout(w int) ([]pageLine, []playback.SearchItem) {
	res := m.top().results.found
	cur := m.cursor()
	var lines []pageLine
	var items []playback.SearchItem

	section := func(name string, n int) {
		if n == 0 {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, pageLine{text: "", item: -1})
		}
		lines = append(lines, pageLine{text: " " + stYellow.Render("▞ ") + stMuted.Render(spaced(name)), item: -1})
	}
	item := func(it playback.SearchItem, tagged bool) {
		n := len(items)
		items = append(items, it)
		lines = append(lines, pageLine{text: resultLine(it, tagged, n == cur, w), item: n})
	}

	var top []playback.SearchItem
	for _, it := range res.Top {
		if openable(it.Kind) {
			top = append(top, it)
		}
	}
	section("TOP RESULTS", len(top))
	for _, it := range top {
		item(it, true)
	}
	section("ARTISTS", len(res.Artists))
	for _, a := range res.Artists {
		item(playback.SearchItem{Kind: playback.ItemArtist, Artist: a}, false)
	}
	section("ALBUMS", len(res.Albums))
	for _, a := range res.Albums {
		item(playback.SearchItem{Kind: playback.ItemAlbum, Album: a}, false)
	}
	section("SONGS", len(res.Songs))
	for _, s := range res.Songs {
		item(playback.SearchItem{Kind: playback.ItemSong, Song: s}, false)
	}
	section("PLAYLISTS", len(res.Playlists))
	for _, p := range res.Playlists {
		item(playback.SearchItem{Kind: playback.ItemPlaylist, Playlist: p}, false)
	}
	return lines, items
}

// openable reports whether a top result of kind opens a view; the player
// should send no other, but a row that does nothing must not show.
func openable(kind playback.SearchItemKind) bool {
	switch kind {
	case playback.ItemArtist, playback.ItemAlbum, playback.ItemSong, playback.ItemPlaylist:
		return true
	}
	return false
}

// resultLine renders one results row in exactly w cells: the glyph of its
// kind, the name and its details (artist · year of an album, artist of a
// song, curator of a playlist, genre of an artist), after the kind when
// tagged.
func resultLine(it playback.SearchItem, tagged, selected bool, w int) string {
	var glyph, text string
	var style lipgloss.Style
	var details []string
	switch it.Kind {
	case playback.ItemArtist:
		glyph, style, text = "◆", stYellow, it.Artist.Name
		if len(it.Artist.Genres) > 0 {
			details = append(details, it.Artist.Genres[0])
		}
	case playback.ItemAlbum:
		glyph, style, text = "◈", stYellow, it.Album.Title
		details = append(details, it.Album.Artist, yearOf(it.Album))
	case playback.ItemSong:
		glyph, style, text = "♪", stCyan, it.Song.Title
		details = append(details, it.Song.Artist)
	case playback.ItemPlaylist:
		glyph, style, text = "≡", stCyan, it.Playlist.Name
		details = append(details, it.Playlist.Curator)
	}
	if tagged {
		details = append([]string{string(it.Kind)}, details...)
	}
	var shown []string
	for _, d := range details {
		if d != "" {
			shown = append(shown, strings.ToUpper(d))
		}
	}
	return detailLine(glyph, style, strings.ToUpper(text), strings.Join(shown, " · "), selected, w)
}

// resultsCode is the serial code in the results panel's bottom edge.
func (m Model) resultsCode() string {
	page := m.top().results
	switch {
	case page.loading:
		return "SCANNING"
	case page.err != nil:
		return "ERR"
	}
	return fmt.Sprintf("HITS %02d", len(m.resultItems()))
}

// resultsBody renders the results page on top in w x h cells: the term, a
// rule and the sections, scrolled to keep the cursor near the middle.
// Every line is exactly w cells wide.
func (m Model) resultsBody(w, h int) []string {
	page := m.top().results
	head := stYellow.Render("⌕ ") + stYellowB.Render(strings.ToUpper(cleanLine(page.term)))
	lines, _ := m.resultsLayout(w)
	notice := pageNotice(page.loading, page.err, "SEARCH", len(lines) == 0)
	return m.pageBody(w, h, []string{" " + head}, lines, notice)
}
