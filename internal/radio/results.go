package radio

import (
	"context"
	"fmt"
	"slices"
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
// answers for a page that was left or reloaded are dropped (fresh is
// false).
func (m Model) onResults(msg resultsMsg) (_ Model, fresh bool) {
	fresh = m.settleFrame(func(f frame) bool {
		return f.kind == viewResults && f.results.seq == msg.seq && f.results.loading
	}, func(f frame) frame {
		f.results.loading, f.results.cancel = false, nil
		f.results.found, f.results.err = cleanResults(msg.found), msg.err
		f.cursor = 0
		return f
	})
	return m, fresh
}

// resultsEnter acts on the selected row: a song plays with the rest of
// its list (see resultsQueue); an artist, album or playlist opens its
// view.
func (m Model) resultsEnter() (Model, tea.Cmd) {
	items := m.resultItems()
	cur := m.cursor()
	if cur < 0 || cur >= len(items) {
		return m, nil
	}
	it := items[cur]
	if it.Kind == playback.ItemSong {
		return m.playSongs(m.resultsQueue(cur))
	}
	kind, ok := resultKinds[it.Kind]
	if !ok {
		return m, nil
	}
	return kind.open(m, it)
}

// resultsAlbum is the album key on the RESULTS page: the selected song's
// SONG view; ok is false on any other row.
func (m Model) resultsAlbum() (next Model, cmd tea.Cmd, ok bool) {
	items := m.resultItems()
	cur := m.cursor()
	if cur < 0 || cur >= len(items) || items[cur].Kind != playback.ItemSong {
		return m, nil, false
	}
	next, cmd = m.openSong(items[cur].Song)
	return next, cmd, true
}

// resultsQueue is the queue for song row cur of the RESULTS page: the
// songs of the SONGS section, started at the song. A top result plays the
// SONGS section too, from its copy there; a top song missing from SONGS
// plays first, the section after it.
func (m Model) resultsQueue(cur int) (songs []playback.Song, start int) {
	var top *playback.Song
	n := 0
	for _, s := range resultSections(m.top().results.found) {
		for _, it := range s.items {
			if n == cur {
				if s.tagged {
					song := it.Song
					top = &song
				} else {
					start = len(songs)
				}
			}
			if !s.tagged && it.Kind == playback.ItemSong {
				songs = append(songs, it.Song)
			}
			n++
		}
	}
	if top != nil {
		start = slices.IndexFunc(songs, func(s playback.Song) bool { return s.ID == top.ID })
		if start < 0 {
			songs, start = append([]playback.Song{*top}, songs...), 0
		}
	}
	return songs, start
}

// resultKind is what the RESULTS page knows of one kind of row: how it
// looks and which view it opens. The kinds without an entry have no view;
// the player should send no other, but a row that does nothing must not
// show.
type resultKind struct {
	glyph string
	style lipgloss.Style
	// describe is the row's name and its details (artist · year of an
	// album, artist of a song, curator of a playlist, genre of an artist).
	describe func(playback.SearchItem) (name string, details []string)
	open     func(Model, playback.SearchItem) (Model, tea.Cmd)
}

var resultKinds = map[playback.SearchItemKind]resultKind{
	playback.ItemArtist: {
		glyph: "◆", style: stYellow,
		describe: func(it playback.SearchItem) (string, []string) {
			var genre []string
			if len(it.Artist.Genres) > 0 {
				genre = it.Artist.Genres[:1]
			}
			return it.Artist.Name, genre
		},
		open: func(m Model, it playback.SearchItem) (Model, tea.Cmd) { return m.openArtist(it.Artist) },
	},
	playback.ItemAlbum: {
		glyph: "◈", style: stYellow,
		describe: func(it playback.SearchItem) (string, []string) {
			return it.Album.Title, []string{it.Album.Artist, yearOf(it.Album)}
		},
		open: func(m Model, it playback.SearchItem) (Model, tea.Cmd) { return m.openAlbum(it.Album) },
	},
	playback.ItemSong: {
		glyph: "♪", style: stCyan,
		describe: func(it playback.SearchItem) (string, []string) {
			return it.Song.Title, []string{it.Song.Artist}
		},
		open: func(m Model, it playback.SearchItem) (Model, tea.Cmd) { return m.openSong(it.Song) },
	},
	playback.ItemPlaylist: {
		glyph: "≡", style: stCyan,
		describe: func(it playback.SearchItem) (string, []string) {
			return it.Playlist.Name, []string{it.Playlist.Curator}
		},
		open: func(m Model, it playback.SearchItem) (Model, tea.Cmd) { return m.openPlaylist(it.Playlist) },
	},
}

// openable reports whether a top result of kind opens a view.
func openable(kind playback.SearchItemKind) bool {
	_, ok := resultKinds[kind]
	return ok
}

// resultSection is one non-empty section of the RESULTS page. A top result
// is tagged with its kind; the rows of the other sections need no tag.
type resultSection struct {
	name   string
	items  []playback.SearchItem
	tagged bool
}

// resultSections is the item model of the RESULTS page: its sections in
// Apple Music order, empty ones left out. It renders nothing, so counting
// or selecting rows costs no layout.
func resultSections(res playback.SearchResults) []resultSection {
	var top []playback.SearchItem
	for _, it := range res.Top {
		if openable(it.Kind) {
			top = append(top, it)
		}
	}
	sections := []resultSection{{name: "TOP RESULTS", items: top, tagged: true}}
	add := func(name string, n int, item func(int) playback.SearchItem) {
		s := resultSection{name: name}
		for i := range n {
			s.items = append(s.items, item(i))
		}
		sections = append(sections, s)
	}
	add("ARTISTS", len(res.Artists), func(i int) playback.SearchItem {
		return playback.SearchItem{Kind: playback.ItemArtist, Artist: res.Artists[i]}
	})
	add("ALBUMS", len(res.Albums), func(i int) playback.SearchItem {
		return playback.SearchItem{Kind: playback.ItemAlbum, Album: res.Albums[i]}
	})
	add("SONGS", len(res.Songs), func(i int) playback.SearchItem {
		return playback.SearchItem{Kind: playback.ItemSong, Song: res.Songs[i]}
	})
	add("PLAYLISTS", len(res.Playlists), func(i int) playback.SearchItem {
		return playback.SearchItem{Kind: playback.ItemPlaylist, Playlist: res.Playlists[i]}
	})
	var shown []resultSection
	for _, s := range sections {
		if len(s.items) > 0 {
			shown = append(shown, s)
		}
	}
	return shown
}

// resultItems lists the selectable rows of the results page on top, in
// order.
func (m Model) resultItems() []playback.SearchItem {
	var items []playback.SearchItem
	for _, s := range resultSections(m.top().results.found) {
		items = append(items, s.items...)
	}
	return items
}

// resultsLayout renders the loaded results page on top in w cells: each
// section's header, then its rows.
func (m Model) resultsLayout(w int) []pageLine {
	cur := m.cursor()
	var lines []pageLine
	n := 0
	for _, s := range resultSections(m.top().results.found) {
		if len(lines) > 0 {
			lines = append(lines, pageLine{text: "", item: -1})
		}
		lines = append(lines, pageLine{text: " " + stYellow.Render("▞ ") + stMuted.Render(spaced(s.name)), item: -1})
		for _, it := range s.items {
			if it.Kind == playback.ItemSong {
				lines = append(lines, m.songLine(it.Song, n, n == cur, w, func(sel bool, w int) string {
					return resultLine(it, s.tagged, sel, w)
				}))
			} else {
				lines = append(lines, pageLine{text: resultLine(it, s.tagged, n == cur, w), item: n})
			}
			n++
		}
	}
	return lines
}

// resultLine renders one results row in exactly w cells: the glyph of its
// kind, the name and its details, after the kind when tagged.
func resultLine(it playback.SearchItem, tagged, selected bool, w int) string {
	kind := resultKinds[it.Kind]
	var name string
	var details []string
	if kind.describe != nil {
		name, details = kind.describe(it)
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
	return detailLine(kind.glyph, kind.style, strings.ToUpper(name), strings.Join(shown, " · "), selected, w)
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
func (m Model) resultsBody(w, h int) ([]string, zones) {
	page := m.top().results
	head := stYellow.Render("⌕ ") + stYellowB.Render(strings.ToUpper(cleanLine(page.term)))
	lines := m.resultsLayout(w)
	notice := pageNotice(page.loading, page.err, "SEARCH", len(lines) == 0)
	return m.pageBody(w, h, []string{" " + head}, lines, notice)
}
