package radio

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// detailCallTimeout bounds loading an album, song album or playlist page.
// It is longer than the per-call default because a song's album takes the
// helper two catalog lookups; the helper's budget (CatalogBudget in
// helper/Sources/Nu11SignalProtocol/Catalog.swift) stays below it, as
// TestCatalogBudgetMatchesTheHelper checks.
const detailCallTimeout = 12 * time.Second

// trackPage is the state of one ALBUM, SONG or PLAYLIST view entry on the
// navigation stack. A SONG view is the ALBUM view of the album holding a
// song, opened with the cursor on that song.
type trackPage struct {
	// kind is the page's view: viewAlbum or viewPlaylist.
	kind viewKind
	// album (ALBUM), song (SONG) or playlist (PLAYLIST) is what was
	// selected; it heads the page until the detail arrives.
	album    playback.Album
	song     playback.Song
	playlist playback.CatalogPlaylist
	// seq numbers the load; answers for another number are dropped.
	seq            uint64
	loading        bool
	err            error
	albumDetail    playback.AlbumDetail
	playlistDetail playback.PlaylistDetail
	// notesOpen shows the full notes (MORE was selected).
	notesOpen bool
	// cancel cancels the load in flight; nil when none.
	cancel context.CancelFunc
}

// Answers to album (and song album) and playlist load number seq.
type (
	albumMsg struct {
		seq    uint64
		detail playback.AlbumDetail
		err    error
	}
	playlistMsg struct {
		seq    uint64
		detail playback.PlaylistDetail
		err    error
	}
)

// trackItem is one selectable row of a track page: the track at index, or
// the MORE/LESS toggle of the notes.
type trackItem struct {
	index int
	more  bool
}

// openAlbum pushes the ALBUM view for a and starts loading it.
func (m Model) openAlbum(a playback.Album) (Model, tea.Cmd) {
	m.push(frame{kind: viewAlbum})
	cmd := m.loadTracks(trackPage{album: a}, viewAlbum)
	return m, cmd
}

// openSong pushes the SONG view for s: the album holding it, with the song
// selected. The search view below keeps its state for esc.
func (m Model) openSong(s playback.Song) (Model, tea.Cmd) {
	m.input.Blur()
	m.push(frame{kind: viewAlbum})
	cmd := m.loadTracks(trackPage{song: s}, viewAlbum)
	return m, cmd
}

// openPlaylist pushes the PLAYLIST view for p and starts loading it.
func (m Model) openPlaylist(p playback.CatalogPlaylist) (Model, tea.Cmd) {
	m.push(frame{kind: viewPlaylist})
	cmd := m.loadTracks(trackPage{playlist: p}, viewPlaylist)
	return m, cmd
}

// loadTracks (re)loads the page on top of the stack, a kind view, for the
// selection in sel.
func (m *Model) loadTracks(sel trackPage, kind viewKind) tea.Cmd {
	m.detailSeq++
	seq := m.detailSeq
	ctx, cancel := context.WithTimeout(context.Background(), max(m.timeout, detailCallTimeout))
	page := trackPage{kind: kind, album: sel.album, song: sel.song, playlist: sel.playlist, seq: seq, loading: true, cancel: cancel}
	m.setTop(frame{kind: kind, tracks: page})
	player := m.player
	switch {
	case kind == viewPlaylist:
		id := sel.playlist.ID
		return func() tea.Msg {
			defer cancel()
			d, err := player.CatalogPlaylist(ctx, id)
			return playlistMsg{seq: seq, detail: d, err: err}
		}
	case sel.song.ID != "":
		id := sel.song.ID
		return func() tea.Msg {
			defer cancel()
			d, err := player.SongAlbum(ctx, id)
			return albumMsg{seq: seq, detail: d, err: err}
		}
	}
	id := sel.album.ID
	return func() tea.Msg {
		defer cancel()
		d, err := player.Album(ctx, id)
		return albumMsg{seq: seq, detail: d, err: err}
	}
}

// onAlbum fills the album page the answer belongs to, selecting the
// searched song; answers for a page that was left or reloaded are
// dropped.
func (m Model) onAlbum(msg albumMsg) Model {
	return m.settleTracks(viewAlbum, msg.seq, msg.err, func(p *trackPage) {
		p.albumDetail = cleanAlbumDetail(msg.detail)
	})
}

// onPlaylist fills the playlist page the answer belongs to.
func (m Model) onPlaylist(msg playlistMsg) Model {
	return m.settleTracks(viewPlaylist, msg.seq, msg.err, func(p *trackPage) {
		p.playlistDetail = cleanPlaylistDetail(msg.detail)
	})
}

func (m Model) settleTracks(kind viewKind, seq uint64, err error, fill func(*trackPage)) Model {
	m.settleFrame(func(f frame) bool {
		return f.kind == kind && f.tracks.seq == seq && f.tracks.loading
	}, func(f frame) frame {
		f.tracks.loading, f.tracks.cancel, f.tracks.err = false, nil, err
		fill(&f.tracks)
		f.cursor = max(f.tracks.highlight(), 0)
		return f
	})
	return m
}

// title names the page's view: ALBUM, SONG or PLAYLIST. It heads the
// panel and names the feed in the page's notices.
func (p trackPage) title() string {
	switch {
	case p.kind == viewPlaylist:
		return "PLAYLIST"
	case p.song.ID != "":
		return "SONG"
	}
	return "ALBUM"
}

// loneSong reports whether a SONG view offers only its song: its album
// failed to load, and the song must stay playable from search.
func (p trackPage) loneSong() bool {
	return p.kind == viewAlbum && p.song.ID != "" && p.err != nil && len(p.albumDetail.Tracks) == 0
}

// tracks lists the songs of the page, in order.
func (p trackPage) tracks() []playback.Song {
	if p.kind == viewPlaylist {
		return p.playlistDetail.Tracks
	}
	if p.loneSong() {
		return []playback.Song{p.song}
	}
	out := make([]playback.Song, len(p.albumDetail.Tracks))
	for i, t := range p.albumDetail.Tracks {
		out[i] = t.Song
	}
	return out
}

// highlight is the index of the track the page was opened for (a SONG
// view), or -1; it only places the cursor when the page loads. The catalog
// id decides; a song from another storefront may carry another id, so an
// equal title is the fallback.
func (p trackPage) highlight() int {
	if p.song.ID == "" {
		return -1
	}
	songs := p.tracks()
	for i, s := range songs {
		if s.ID == p.song.ID {
			return i
		}
	}
	for i, s := range songs {
		if p.song.Title != "" && strings.EqualFold(s.Title, cleanLine(p.song.Title)) {
			return i
		}
	}
	return -1
}

// playingIndex is the index of the page's track the player is on (playing
// or paused), or -1. The catalog id decides; the player may report another
// id for the same song (a library copy, another storefront), so an equal
// title and artist is the fallback. A track without an artist of its own
// (an album track) takes the album's.
func (m Model) playingIndex(p trackPage) int {
	if !m.hasState || m.signalLost() || m.state.Title == "" || m.state.Status == playback.StatusStopped {
		return -1
	}
	songs := p.tracks()
	if id := m.state.SongID; id != "" {
		for i, s := range songs {
			if s.ID == id {
				return i
			}
		}
	}
	for i, s := range songs {
		artist := s.Artist
		if artist == "" {
			artist = p.albumDetail.Album.Artist
		}
		if strings.EqualFold(s.Title, m.state.Title) && strings.EqualFold(artist, m.state.Artist) {
			return i
		}
	}
	return -1
}

// tracksEnter plays the page's tracks from the selected one, or toggles
// the notes on MORE.
func (m Model) tracksEnter() (Model, tea.Cmd) {
	items := m.trackItems()
	cur := m.cursor()
	if cur < 0 || cur >= len(items) {
		return m, nil
	}
	it := items[cur]
	if it.more {
		f := m.top()
		f.tracks.notesOpen = !f.tracks.notesOpen
		m.setTop(f)
		return m, nil
	}
	songs := m.top().tracks.tracks()
	ids := make([]string, len(songs))
	for i, s := range songs {
		ids[i] = s.ID
	}
	m.playSeq++
	return m, m.playCmd(m.playSeq, "PLAY", "", func(ctx context.Context) error {
		return m.player.PlaySongs(ctx, ids, it.index)
	})
}

// trackItems lists the selectable rows of the track page on top, at the
// width it is drawn with (whether MORE is offered depends on how the notes
// wrap).
func (m Model) trackItems() []trackItem {
	_, items := m.trackLayout(m.listBodyWidth())
	return items
}

// trackLayout lays out the loaded track page on top in w cells: the
// numbered tracks, the facts under them and the notes.
func (m Model) trackLayout(w int) ([]pageLine, []trackItem) {
	f := m.top()
	page := f.tracks
	cur := f.cursor
	var lines []pageLine
	var items []trackItem
	add := func(text string) { lines = append(lines, pageLine{text: text, item: -1}) }
	item := func(it trackItem, render func(selected bool) string) {
		n := len(items)
		items = append(items, it)
		lines = append(lines, pageLine{text: render(n == cur), item: n})
	}

	// ▶ marks the track playing, never the one searched for.
	on := m.playingIndex(page)
	var total time.Duration
	var notes string
	if f.kind == viewPlaylist {
		d := page.playlistDetail
		for i, s := range d.Tracks {
			total += s.Duration
			item(trackItem{index: i}, func(sel bool) string {
				return trackLine(i+1, strings.ToUpper(s.Title), strings.ToUpper(s.Artist), s.Duration, i == on, sel, w)
			})
		}
		notes = d.Notes
		if len(d.Tracks) > 0 {
			add("")
			add(" " + stMuted.Render(tracksSummary(len(d.Tracks), total)))
		}
	} else if page.loneSong() {
		s := page.song
		item(trackItem{index: 0}, func(sel bool) string {
			return trackLine(1, strings.ToUpper(s.Title), strings.ToUpper(s.Artist), s.Duration, on == 0, sel, w)
		})
	} else {
		d := page.albumDetail
		discs := 0
		for _, t := range d.Tracks {
			discs = max(discs, t.Disc)
		}
		disc := 0
		for i, t := range d.Tracks {
			if discs > 1 && t.Disc != disc {
				disc = t.Disc
				if len(lines) > 0 {
					add("")
				}
				add(" " + stYellow.Render("▞ ") + stMuted.Render(spaced(fmt.Sprintf("DISC %d", disc))))
			}
			total += t.Duration
			n := t.Number
			if n <= 0 {
				n = i + 1
			}
			item(trackItem{index: i}, func(sel bool) string {
				return trackLine(n, strings.ToUpper(t.Title), "", t.Duration, i == on, sel, w)
			})
		}
		notes = d.Notes
		if len(d.Tracks) > 0 || d.ReleaseDate != "" || d.Copyright != "" || d.RecordLabel != "" {
			add("")
		}
		if d.ReleaseDate != "" {
			add(" " + stMuted.Render(releaseLabel(d.ReleaseDate)))
		}
		if len(d.Tracks) > 0 {
			add(" " + stMuted.Render(tracksSummary(len(d.Tracks), total)))
		}
		if d.Copyright != "" {
			add(" " + stMuted.Render(strings.ToUpper(d.Copyright)))
		}
		if label := strings.ToUpper(d.RecordLabel); label != "" && !strings.Contains(strings.ToUpper(d.Copyright), label) {
			add(" " + stMuted.Render(label))
		}
	}

	if notes != "" {
		wrapped, more := wrapNotes(notes, page.notesOpen, w)
		add("")
		for _, l := range wrapped {
			add(" " + stRed.Render(l))
		}
		if more {
			item(trackItem{more: true}, func(sel bool) string { return moreLine(page.notesOpen, sel, w) })
		}
	}
	return lines, items
}

// trackLine lays out one track row in exactly w cells: the selection mark,
// a ▶ on the song playing, the number, the title with a muted detail
// (a playlist track's artist) and the duration at the right edge.
func trackLine(num int, title, detail string, d time.Duration, playing, selected bool, w int) string {
	mark := " "
	if playing {
		mark = "▶"
	}
	number := fmt.Sprintf("%2d", num)
	right := ""
	if t := trackTime(d); t != "" {
		right = "  " + t + " "
	}
	room := max(w-2-ansi.StringWidth(number)-2-ansi.StringWidth(right), 0)
	title = ansi.Truncate(title, room, "…")
	if rest := room - ansi.StringWidth(title); detail != "" && rest > 3 {
		detail = ansi.Truncate(" · "+detail, rest, "…")
	} else {
		detail = ""
	}
	pad := strings.Repeat(" ", room-ansi.StringWidth(title)-ansi.StringWidth(detail))

	if selected {
		return stSelected.Render(fit("▌"+mark+number+"  "+title+detail+pad+right, w))
	}
	titleStyle := stRed
	if playing {
		titleStyle = stYellowB
	}
	line := " " + stYellow.Render(mark) + stMuted.Render(number) + "  " + titleStyle.Render(title) +
		stMuted.Render(detail) + pad + stMuted.Render(right)
	return fit(line, w)
}

// trackTime renders a track duration as Apple Music does: 5:20, 1:02:03;
// empty when unknown.
func trackTime(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	s := int(d / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// releaseLabel renders a "2006-01-02" release date as "27 JUN 1975"; any
// other text is shown as it is.
func releaseLabel(date string) string {
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return strings.ToUpper(t.Format("2 Jan 2006"))
	}
	return strings.ToUpper(date)
}

// tracksSummary is the "14 SONGS, 1 HOUR 1 MINUTE" line under a track
// list; the length is left out when no duration is known.
func tracksSummary(n int, total time.Duration) string {
	s := plural(n, "SONG")
	if total <= 0 {
		return s
	}
	mins := max(int((total+30*time.Second)/time.Minute), 1)
	length := plural(mins%60, "MINUTE")
	switch {
	case mins >= 60 && mins%60 == 0:
		length = plural(mins/60, "HOUR")
	case mins >= 60:
		length = plural(mins/60, "HOUR") + " " + length
	}
	return s + ", " + length
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %sS", n, word)
}

// trackTitle is the panel title of the track page on top.
func (m Model) trackTitle() string { return m.top().tracks.title() }

// trackCode is the serial code in the track panel's bottom edge.
func (m Model) trackCode() string {
	page := m.top().tracks
	switch {
	case page.loading:
		return "LINKING"
	case page.err != nil:
		return "ERR"
	}
	return fmt.Sprintf("TRACKS %02d", len(page.tracks()))
}

// trackHead is the head of the track page on top: the title, then the
// artist (or curator), then the genre and year of an album.
func (m Model) trackHead() []string {
	f := m.top()
	p := f.tracks
	var title, by, facts string
	if f.kind == viewPlaylist {
		pl := p.playlistDetail.Playlist
		if pl.Name == "" {
			pl = p.playlist
		}
		title, by, facts = pl.Name, pl.Curator, "PLAYLIST"
	} else {
		d := p.albumDetail
		a := d.Album
		switch {
		case a.Title != "":
		case p.song.ID != "":
			// The song's album heads the page until it loads; a song row
			// may not know its album, then the song stands in.
			a = playback.Album{Title: p.song.Album, Artist: p.song.Artist}
			if a.Title == "" {
				a.Title = p.song.Title
			}
		default:
			a = p.album
		}
		title, by = a.Title, a.Artist
		year := yearOf(a)
		if year == "" && len(d.ReleaseDate) >= 4 {
			year = d.ReleaseDate[:4]
		}
		var parts []string
		for _, s := range []string{strings.ToUpper(d.Genre), year} {
			if s != "" {
				parts = append(parts, s)
			}
		}
		facts = strings.Join(parts, " · ")
	}
	head := []string{" " + stYellowB.Render(strings.ToUpper(title))}
	if by != "" {
		head = append(head, " "+stCyan.Render(strings.ToUpper(by)))
	}
	if facts != "" {
		head = append(head, " "+stMuted.Render(facts))
	}
	return head
}

// trackBody renders the track page on top in w x h cells. Every line is
// exactly w cells wide.
func (m Model) trackBody(w, h int) ([]string, zones) {
	f := m.top()
	lines, _ := m.trackLayout(w)
	notice := pageNotice(f.tracks.loading, f.tracks.err, f.tracks.title(), len(lines) == 0)
	return m.pageBody(w, h, m.trackHead(), lines, notice)
}
