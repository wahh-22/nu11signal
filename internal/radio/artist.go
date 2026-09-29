package radio

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// artistCallTimeout bounds loading an artist page. It is longer than the
// per-call default because the helper looks the artist up first and then
// loads its sections and origin/formed concurrently. The helper's budget
// for that (CatalogBudget in helper/Sources/Nu11SignalProtocol/Catalog.swift)
// stays below it; TestCatalogBudgetMatchesTheHelper pins both sides.
const artistCallTimeout = 15 * time.Second

// artistPage is the state of one ARTIST view entry on the navigation
// stack. It lives in its frame, so popping the page drops it.
type artistPage struct {
	// artist is the artist as selected; it heads the page until the
	// detail arrives.
	artist playback.Artist
	// seq numbers the load; answers for another number are dropped.
	seq     uint64
	loading bool
	err     error
	detail  playback.ArtistDetail
	// aboutOpen shows the full notes (MORE was selected).
	aboutOpen bool
	// cancel cancels the load in flight; nil when none.
	cancel context.CancelFunc
}

// artistMsg answers artist load number seq.
type artistMsg struct {
	seq    uint64
	detail playback.ArtistDetail
	err    error
}

// artistItemKind is what a selectable row of the artist page stands for.
type artistItemKind int

const (
	itemSong artistItemKind = iota
	itemAlbum
	itemPlaylist
	// itemMore toggles the full ABOUT notes.
	itemMore
)

// artistItem is one selectable row of the artist page.
type artistItem struct {
	kind artistItemKind
	// index is the song's position in TopSongs.
	index    int
	album    playback.Album
	playlist playback.CatalogPlaylist
}

// openArtist pushes the ARTIST view for a and starts loading its page. The
// search view below keeps its state for esc.
func (m Model) openArtist(a playback.Artist) (Model, tea.Cmd) {
	m.input.Blur()
	m.push(frame{kind: viewArtist})
	cmd := m.loadArtist(a)
	return m, cmd
}

// loadArtist (re)loads the page on top of the stack, which must be an
// artist page.
func (m *Model) loadArtist(a playback.Artist) tea.Cmd {
	m.artistSeq++
	seq := m.artistSeq
	ctx, cancel := context.WithTimeout(context.Background(), max(m.timeout, artistCallTimeout))
	m.setTop(frame{kind: viewArtist, artist: artistPage{artist: a, seq: seq, loading: true, cancel: cancel}})
	player, id := m.player, a.ID
	return func() tea.Msg {
		defer cancel()
		d, err := player.Artist(ctx, id)
		return artistMsg{seq: seq, detail: d, err: err}
	}
}

// onArtist fills the page the answer belongs to, even while parked;
// answers for a page that was left or reloaded are dropped.
func (m Model) onArtist(msg artistMsg) Model {
	m.settleFrame(func(f frame) bool {
		return f.kind == viewArtist && f.artist.seq == msg.seq && f.artist.loading
	}, func(f frame) frame {
		f.artist.loading, f.artist.cancel = false, nil
		f.artist.detail, f.artist.err = cleanArtistDetail(msg.detail), msg.err
		f.cursor = 0
		return f
	})
	return m
}

// artistEnter acts on the selected row: a top song plays the top songs
// from it, MORE toggles the notes, and albums and playlists open their
// views.
func (m Model) artistEnter() (Model, tea.Cmd) {
	items := m.artistItems()
	cur := m.cursor()
	if cur < 0 || cur >= len(items) {
		return m, nil
	}
	it := items[cur]
	switch it.kind {
	case itemSong:
		top := m.top().artist.detail.TopSongs
		ids := make([]string, len(top))
		for i, s := range top {
			ids[i] = s.ID
		}
		m.playSeq++
		return m, m.playCmd(m.playSeq, "PLAY", "", func(ctx context.Context) error {
			return m.player.PlaySongs(ctx, ids, it.index)
		})
	case itemMore:
		f := m.top()
		f.artist.aboutOpen = !f.artist.aboutOpen
		m.setTop(f)
		return m, nil
	case itemAlbum:
		return m.openAlbum(it.album)
	case itemPlaylist:
		return m.openPlaylist(it.playlist)
	}
	return m, nil
}

// artistItems lists the selectable rows of the artist page on top, at the
// width the page is drawn with (whether MORE is offered depends on how the
// notes wrap).
func (m Model) artistItems() []artistItem {
	_, items := m.artistLayout(m.listBodyWidth())
	return items
}

// artistLayout lays out the loaded page on top in w cells: its sections in
// Apple Music order, empty ones left out, and the selectable rows.
func (m Model) artistLayout(w int) ([]pageLine, []artistItem) {
	page := m.top().artist
	d := page.detail
	cur := m.cursor()
	var lines []pageLine
	var items []artistItem

	add := func(text string) { lines = append(lines, pageLine{text: text, item: -1}) }
	section := func(name string) {
		if len(lines) > 0 {
			add("")
		}
		add(" " + stYellow.Render("▞ ") + stMuted.Render(spaced(name)))
	}
	item := func(it artistItem, render func(selected bool) string) {
		n := len(items)
		items = append(items, it)
		lines = append(lines, pageLine{text: render(n == cur), item: n})
	}
	albumSection := func(name string, albums []playback.Album) {
		if len(albums) == 0 {
			return
		}
		section(name)
		for _, a := range albums {
			item(artistItem{kind: itemAlbum, album: a}, func(sel bool) string {
				return detailLine("◈", stYellow, strings.ToUpper(a.Title), yearOf(a), sel, w)
			})
		}
	}

	if len(d.TopSongs) > 0 {
		section("TOP SONGS")
		for i, s := range d.TopSongs {
			item(artistItem{kind: itemSong, index: i}, func(sel bool) string {
				return detailLine("♪", stCyan, strings.ToUpper(s.Title), strings.ToUpper(s.Album), sel, w)
			})
		}
	}
	albumSection("ESSENTIAL ALBUMS", d.EssentialAlbums)
	albumSection("ALBUMS", d.Albums)
	if len(d.Playlists) > 0 {
		section("ARTIST PLAYLISTS")
		for _, p := range d.Playlists {
			item(artistItem{kind: itemPlaylist, playlist: p}, func(sel bool) string {
				return detailLine("≡", stCyan, strings.ToUpper(p.Name), strings.ToUpper(p.Curator), sel, w)
			})
		}
	}
	albumSection("SINGLES & EPS", d.Singles)
	albumSection("COMPILATIONS", d.Compilations)

	about := d.About
	if about == (playback.ArtistAbout{}) {
		return lines, items
	}
	section("ABOUT")
	if about.Notes != "" {
		notes, more := wrapNotes(about.Notes, page.aboutOpen, w)
		for _, l := range notes {
			add(" " + stRed.Render(l))
		}
		if more {
			item(artistItem{kind: itemMore}, func(sel bool) string { return moreLine(page.aboutOpen, sel, w) })
		}
	}
	for _, fact := range []struct{ label, value string }{
		{"FROM", about.Origin}, {"FORMED", about.Formed}, {"GENRE", about.Genre},
	} {
		if fact.value != "" {
			add(" " + stMuted.Render(fmt.Sprintf("%-7s", fact.label)) + stRed.Render(strings.ToUpper(fact.value)))
		}
	}
	return lines, items
}

// yearOf is an album's year for its row, empty when unknown.
func yearOf(a playback.Album) string {
	if a.Year <= 0 {
		return ""
	}
	return fmt.Sprint(a.Year)
}

// detailLine lays out one page row in exactly w cells: a selection mark, a
// glyph, the text and, after a dot, a muted detail such as the album or
// the year.
func detailLine(glyph string, glyphStyle lipgloss.Style, text, detail string, selected bool, w int) string {
	if selected {
		plain := "▌" + glyph + " " + text
		if detail != "" {
			plain += " · " + detail
		}
		return stSelected.Render(fit(plain, w))
	}
	line := " " + glyphStyle.Render(glyph) + " " + stRed.Render(text)
	if detail != "" {
		line += stMuted.Render(" · " + detail)
	}
	return fit(line, w)
}

// artistTitle is the name heading the page: the loaded one, or the one
// selected while it loads.
func (p artistPage) artistTitle() string {
	if p.detail.Artist.Name != "" {
		return p.detail.Artist.Name
	}
	return p.artist.Name
}

// artistGenre is the artist's first genre, loaded or as selected; empty
// when unknown.
func (p artistPage) artistGenre() string {
	for _, g := range [][]string{p.detail.Artist.Genres, p.artist.Genres} {
		if len(g) > 0 {
			return g[0]
		}
	}
	return ""
}

// artistCode is the serial code in the artist panel's bottom edge.
func (m Model) artistCode() string {
	page := m.top().artist
	switch {
	case page.loading:
		return "LINKING"
	case page.err != nil:
		return "ERR"
	}
	return fmt.Sprintf("ITEMS %02d", len(m.artistItems()))
}

// artistBody renders the artist page in w x h cells: the name, a rule and
// the sections, scrolled to keep the cursor near the middle. Every line is
// exactly w cells wide.
func (m Model) artistBody(w, h int) []string {
	page := m.top().artist
	head := stYellowB.Render(strings.ToUpper(page.artistTitle()))
	if g := page.artistGenre(); g != "" {
		head += stMuted.Render("  " + strings.ToUpper(g))
	}
	lines, _ := m.artistLayout(w)
	notice := pageNotice(page.loading, page.err, "ARTIST", len(lines) == 0)
	return m.pageBody(w, h, []string{" " + head}, lines, notice)
}
