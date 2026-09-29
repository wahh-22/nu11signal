package radio

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Library editing: favorites (love), adding a song to a library playlist
// and creating one. The Apple Music API offers nothing more: playlists
// cannot be renamed or deleted, nor songs removed from them.
//
// A page's songs have their favorite states read as the page loads, in
// one Favorites call for all of them, so its hearts show at once. The
// animation tick reads, one by one, only what that left unknown: the song
// playing when it is not on the page, or the selected song when the page
// read failed. Holding ↓ never floods the helper. A failed tick read is
// tried again favoriteRetryAfter later.
// States are cached per song; a change shows at once and is sent to the
// player, one at a time per song: presses meanwhile only move the state
// shown, and the latest one is sent when the change in flight answers (as
// the volume does). A refused change is dropped from the cache, so the next
// tick reads the truth back. Songs only in the library have no catalog id,
// which ratings refuse: they are neither loved nor added.
//
// ADD TO PLAYLIST and NEW PLAYLIST are an editor over the list panel, not
// a view on the navigation stack: esc closes it on the page it opened
// over, untouched. One library write (an add or a create) is in flight at
// a time. Those writes are not idempotent, and one that ran out of time
// may still be applied: it is never retried blindly. The editor closes and
// the user is told to check the library first.

// favoriteRetryAfter is how long a song whose favorite state failed to
// read waits before the ticks read it again.
const favoriteRetryAfter = 30 * time.Second

// favorite is what the UI knows of a song's favorite state.
type favorite struct {
	on, known bool
	// reading means a read is in flight; failedAt, when set, is when the
	// last one failed: the ticks read it again favoriteRetryAfter later.
	reading  bool
	failedAt time.Time
	// writing means a change is in flight; queued that the state was
	// changed again meanwhile, so on is sent once that change answers.
	writing, queued bool
	// seq is the latest read or change; a read answer carrying another is
	// stale.
	seq uint64
}

// editMode is what the library editor shows.
type editMode int

const (
	editClosed editMode = iota
	// editPick is the ADD TO PLAYLIST picker.
	editPick
	// editName is the NEW PLAYLIST name input.
	editName
)

// libraryEditor is the state of the editor over the list panel.
type libraryEditor struct {
	mode editMode
	// song is the song to add; empty when a playlist is created from the
	// PLAYLISTS root.
	song playback.Song
	// cursor is the selected picker row; 0 is + NEW PLAYLIST.
	cursor int
	// fromPicker means the name input came from the picker, which esc
	// goes back to.
	fromPicker bool
	// seq is the write (an add or a create) this editor started; its
	// answer closes the editor.
	seq uint64
	// inputHadFocus keeps whether the SEARCH input had the keys when the
	// editor opened, to give them back when it closes.
	inputHadFocus bool
}

// Answers to the library calls.
type (
	favoriteMsg struct {
		id  string
		seq uint64
		on  bool
		err error
	}
	// favoritesMsg answers the page read number seq, for ids.
	favoritesMsg struct {
		ids   []string
		seq   uint64
		loved map[string]bool
		err   error
	}
	setFavoriteMsg struct {
		song playback.Song
		on   bool
		err  error
	}
	addedMsg struct {
		seq      uint64
		song     playback.Song
		playlist playback.Playlist
		err      error
	}
	createdMsg struct {
		seq      uint64
		name     string
		song     playback.Song
		playlist playback.Playlist
		err      error
	}
)

// songActionsWidth is the room the ♥ and + controls take at the end of a
// song row, and songActionsMinWidth the narrowest row that gets them.
// heartTitleMinWidth is the narrowest NOW PLAYING title line that gets
// the ♥ button: the button and heartTitleMinRoom cells of title.
const (
	songActionsWidth    = 5
	songActionsMinWidth = 24
	heartTitleMinRoom   = 12
	heartTitleMinWidth  = heartTitleMinRoom + songActionsWidth
)

// createCheck is a create whose outcome is unknown, looked for in the
// next playlists read: a playlist named name that is not among known, the
// ids of its namesakes listed when the create failed.
type createCheck struct {
	name  string
	known []string
}

// outcomeUnknown reports whether a failed write may still be applied: it
// ran out of time here, or in the helper, whose adapter marks those
// errors with an OutcomeUnknown method (see helper.CommandError).
func outcomeUnknown(err error) bool {
	var u interface{ OutcomeUnknown() bool }
	return errors.Is(err, context.DeadlineExceeded) || errors.As(err, &u) && u.OutcomeUnknown()
}

// libraryCtx bounds a library read or write: the helper goes through the
// Apple Music API for them (CatalogBudget.libraryEdit).
func (m Model) libraryCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), max(m.timeout, detailCallTimeout))
}

// notInCatalog is the notice for a song only in the library, which the
// helper cannot act on.
func notInCatalog(s playback.Song, cannot string) string {
	return strings.ToUpper(s.Title) + " IS NOT IN THE APPLE MUSIC CATALOG // IT CANNOT BE " + cannot
}

// selectedSong is the song on the selected row of the list, when that row
// is a song: a track, a top song, or a song among the results or the
// search rows.
func (m Model) selectedSong() (playback.Song, bool) {
	f := m.top()
	cur := f.cursor
	switch f.kind {
	case viewAlbum, viewPlaylist:
		items := m.trackItems()
		if cur < 0 || cur >= len(items) || items[cur].more || items[cur].playAll {
			return playback.Song{}, false
		}
		if songs := f.tracks.tracks(); items[cur].index < len(songs) {
			return songs[items[cur].index], true
		}
	case viewResults:
		if items := m.resultItems(); cur >= 0 && cur < len(items) && items[cur].Kind == playback.ItemSong {
			return items[cur].Song, true
		}
	case viewArtist:
		items := m.artistItems()
		if cur >= 0 && cur < len(items) && items[cur].kind == itemSong {
			return f.artist.detail.TopSongs[items[cur].index], true
		}
	case viewSearch:
		if rows := m.searchRows(); cur >= 0 && cur < len(rows) && rows[cur].kind == rowSong {
			return rows[cur].song, true
		}
	}
	return playback.Song{}, false
}

// playingSong is the song the player is on, playing or paused, when it
// names one.
func (m Model) playingSong() (playback.Song, bool) {
	if !m.hasState || m.signalLost() || m.state.SongID == "" || m.state.Status == playback.StatusStopped {
		return playback.Song{}, false
	}
	return playback.Song{ID: m.state.SongID, Title: m.state.Title, Artist: m.state.Artist, Album: m.state.Album}, true
}

// songTarget is the song l and a act on: the selected song row while the
// list has the focus, else the song playing.
func (m Model) songTarget() (playback.Song, bool) {
	if m.focus != areaPlayer {
		if s, ok := m.selectedSong(); ok {
			return s, true
		}
	}
	return m.playingSong()
}

// favoriteOf reports the cached favorite state of song id; known is false
// until it is read.
func (m Model) favoriteOf(id string) (on, known bool) {
	f := m.favs[id]
	return f.on && f.known, f.known
}

// setFavorite caches f for song id; the map is copied, as Models are
// values.
func (m *Model) setFavorite(id string, f favorite) {
	favs := maps.Clone(m.favs)
	if favs == nil {
		favs = map[string]favorite{}
	}
	favs[id] = f
	m.favs = favs
}

// readFavorites starts reading the favorite state of the selected song
// and the song playing, those not known, being read or changed, or failed
// less than favoriteRetryAfter ago.
func (m Model) readFavorites() (Model, tea.Cmd) {
	if m.auth != authOK || m.signalLost() {
		return m, nil
	}
	var cmds []tea.Cmd
	for _, s := range m.favoriteTargets() {
		f := m.favs[s.ID]
		failedLately := !f.failedAt.IsZero() && m.now().Before(f.failedAt.Add(favoriteRetryAfter))
		if f.known || f.reading || f.writing || failedLately {
			continue
		}
		m.favSeq++
		f.reading, f.seq = true, m.favSeq
		m.setFavorite(s.ID, f)
		id, seq, player := s.ID, f.seq, m.player
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := m.libraryCtx()
			defer cancel()
			on, err := player.Favorite(ctx, id)
			return favoriteMsg{id: id, seq: seq, on: on, err: err}
		})
	}
	return m, tea.Batch(cmds...)
}

// favoriteTargets are the songs whose state the UI shows: the selected
// song and the song playing, those with a catalog id.
func (m Model) favoriteTargets() []playback.Song {
	var out []playback.Song
	add := func(s playback.Song, ok bool) {
		if ok && s.ID != "" && !s.LibraryOnly && !slices.ContainsFunc(out, func(o playback.Song) bool { return o.ID == s.ID }) {
			out = append(out, s)
		}
	}
	add(m.selectedSong())
	add(m.playingSong())
	return out
}

// prefetchFavorites reads, in one call, the favorite states of the songs
// of a page just loaded: those with a catalog id that are not known, being
// read or changed.
func (m Model) prefetchFavorites(songs []playback.Song) (Model, tea.Cmd) {
	if m.auth != authOK || m.signalLost() {
		return m, nil
	}
	var ids []string
	for _, s := range songs {
		f := m.favs[s.ID]
		if s.ID == "" || s.LibraryOnly || f.known || f.reading || f.writing || slices.Contains(ids, s.ID) {
			continue
		}
		ids = append(ids, s.ID)
	}
	if len(ids) == 0 {
		return m, nil
	}
	m.favSeq++
	seq, player := m.favSeq, m.player
	for _, id := range ids {
		f := m.favs[id]
		f.reading, f.seq = true, seq
		m.setFavorite(id, f)
	}
	return m, func() tea.Msg {
		ctx, cancel := m.libraryCtx()
		defer cancel()
		loved, err := player.Favorites(ctx, ids)
		return favoritesMsg{ids: ids, seq: seq, loved: loved, err: err}
	}
}

// onFavorites settles a page read, but for the songs a change or another
// read overtook. A failed read leaves the states unknown, and quietly:
// the tick then reads the ones it shows, one by one.
func (m Model) onFavorites(msg favoritesMsg) Model {
	for _, id := range msg.ids {
		f := m.favs[id]
		if f.seq != msg.seq {
			continue
		}
		f.reading = false
		if msg.err == nil {
			f.on, f.known, f.failedAt = msg.loved[id], true, time.Time{}
		}
		m.setFavorite(id, f)
	}
	return m
}

// onFavorite settles a read; one a change overtook is dropped.
func (m Model) onFavorite(msg favoriteMsg) Model {
	f := m.favs[msg.id]
	if f.seq != msg.seq {
		return m
	}
	f.reading = false
	if msg.err != nil {
		f.failedAt = m.now()
	} else {
		f.on, f.known, f.failedAt = msg.on, true, time.Time{}
	}
	m.setFavorite(msg.id, f)
	return m
}

// clearFailedFavorites lets the next tick read again the states that
// failed to read, as when the library access comes back.
func (m *Model) clearFailedFavorites() {
	favs := maps.Clone(m.favs)
	for id, f := range favs {
		f.failedAt = time.Time{}
		favs[id] = f
	}
	m.favs = favs
}

// loveTarget toggles the favorite state of the song l acts on.
func (m Model) loveTarget() (Model, tea.Cmd) {
	s, ok := m.songTarget()
	if !ok {
		m.setStatus("NO SONG TO LOVE")
		return m, nil
	}
	return m.toggleFavorite(s)
}

// toggleFavorite loves s, or unloves it when it is known to be loved: an
// unknown state counts as not loved. The row shows the change at once; it
// is sent now, or once the change in flight for s answers.
func (m Model) toggleFavorite(s playback.Song) (Model, tea.Cmd) {
	if s.LibraryOnly {
		m.setStatus(notInCatalog(s, "LOVED"))
		return m, nil
	}
	on, _ := m.favoriteOf(s.ID)
	f := m.favs[s.ID]
	m.favSeq++ // a read in flight is now stale
	f.on, f.known, f.reading, f.failedAt, f.seq = !on, true, false, time.Time{}, m.favSeq
	if f.writing {
		f.queued = true
		m.setFavorite(s.ID, f)
		return m, nil
	}
	return m.sendFavorite(s, f)
}

// sendFavorite caches f for s and asks the player for its state.
func (m Model) sendFavorite(s playback.Song, f favorite) (Model, tea.Cmd) {
	f.writing, f.queued = true, false
	m.setFavorite(s.ID, f)
	on, player := f.on, m.player
	return m, func() tea.Msg {
		ctx, cancel := m.libraryCtx()
		defer cancel()
		return setFavoriteMsg{song: s, on: on, err: player.SetFavorite(ctx, s.ID, on)}
	}
}

// onSetFavorite settles a change. A change queued meanwhile is sent when
// it differs from the one answered, or when that one failed (the state is
// then unknown). Else a refused change is reported, and the state is
// forgotten so that the next tick reads it back.
func (m Model) onSetFavorite(msg setFavoriteMsg) (Model, tea.Cmd) {
	title := strings.ToUpper(msg.song.Title)
	f := m.favs[msg.song.ID]
	f.writing = false
	if msg.err != nil {
		m.setStatus("LOVE FAILED // " + msg.err.Error())
	}
	if f.queued && (msg.err != nil || f.on != msg.on) {
		return m.sendFavorite(msg.song, f)
	}
	f.queued = false
	switch {
	case msg.err != nil:
		f = favorite{}
	case msg.on:
		m.setStatus("♥ LOVED // " + title)
	default:
		m.setStatus("♡ UNLOVED // " + title)
	}
	m.setFavorite(msg.song.ID, f)
	return m, nil
}

// songActions renders the end of a song row, songActionsWidth cells: on
// the selected row its ♥ (or ♡) and + controls, elsewhere a ♥ when the
// song is known to be loved.
func (m Model) songActions(s playback.Song, selected bool) string {
	on, _ := m.favoriteOf(s.ID)
	heart := "♡"
	if on {
		heart = "♥"
	}
	if selected {
		return stSelected.Render(" " + heart + " + ")
	}
	if on {
		return " " + stYellow.Render(heart) + "   "
	}
	return strings.Repeat(" ", songActionsWidth)
}

// songRow renders a song row in w cells with row, ended by its controls
// when w leaves room for them; actions reports that the selected row's
// controls are drawn (see pageBody).
func (m Model) songRow(s playback.Song, selected bool, w int, row func(w int) string) (text string, actions bool) {
	if w < songActionsMinWidth {
		return row(w), false
	}
	return row(w-songActionsWidth) + m.songActions(s, selected), selected
}

// songLine is the page line of song s, selectable item n of a page w
// cells wide: its row drawn by render, ended by its controls (see
// songRow).
func (m Model) songLine(s playback.Song, n int, selected bool, w int, render func(selected bool, w int) string) pageLine {
	text, actions := m.songRow(s, selected, w, func(w int) string { return render(selected, w) })
	return pageLine{text: text, item: n, actions: actions}
}

// addActionZones registers the ♥ and + of a song row w cells wide drawn
// on line y.
func addActionZones(zs *zones, w, y int) {
	x := w - songActionsWidth
	zs.add(zoneRowFavorite, x, y, 2)
	zs.add(zoneRowAdd, x+2, y, 2)
}

// heartTitle ends the title line of NOW PLAYING, w cells, with the ♥
// button of the song playing, while there is one and room for it (w of at
// least heartTitleMinWidth); the zones are in the line's coordinates.
func (m Model) heartTitle(title string, w int) (string, zones) {
	s, ok := m.playingSong()
	if !ok || w < heartTitleMinWidth {
		return title, nil
	}
	b := button{id: zoneFavPlaying, label: "♡", tone: stMuted, focused: m.focused(ctlFav)}
	if on, _ := m.favoriteOf(s.ID); on {
		b.label, b.tone = "♥", stYellow
	}
	room := w - b.width() - 1
	title = ansi.Truncate(title, room, "…")
	var zs zones
	zs.add(b.id, room+1, 0, b.width())
	return title + strings.Repeat(" ", room-ansi.StringWidth(title)+1) + b.render(), zs
}

// addTarget opens the picker for the song a acts on.
func (m Model) addTarget() (Model, tea.Cmd) {
	s, ok := m.songTarget()
	if !ok {
		m.setStatus("NO SONG TO ADD")
		return m, nil
	}
	return m.openPicker(s)
}

// searchTyping reports whether the SEARCH input takes the keys: it has
// them, or the list has lent the focus to the player or the nav tabs and
// the input gets them back when the list takes it again.
func (m Model) searchTyping() bool {
	if m.focus == areaList {
		return m.input.Focused()
	}
	return m.inputHadFocus && m.top().kind == viewSearch
}

// openPicker opens ADD TO PLAYLIST for s over the list, which takes the
// focus; the SEARCH input, if it was typing, gets the keys back when the
// picker closes.
func (m Model) openPicker(s playback.Song) (Model, tea.Cmd) {
	if s.LibraryOnly {
		m.setStatus(notInCatalog(s, "ADDED"))
		return m, nil
	}
	typing := m.searchTyping()
	focus := m.focusList()
	m.editor = libraryEditor{mode: editPick, song: s, inputHadFocus: typing}
	m.input.Blur()
	return m, focus
}

// openName opens the NEW PLAYLIST name input, empty; the new playlist
// will hold s, if any. Coming from the picker, esc goes back to it.
func (m Model) openName(s playback.Song, fromPicker bool) (Model, tea.Cmd) {
	typing := m.searchTyping()
	focus := m.focusList()
	if !fromPicker {
		m.editor = libraryEditor{inputHadFocus: typing}
	}
	m.editor.mode, m.editor.song, m.editor.fromPicker = editName, s, fromPicker
	m.input.Blur()
	m.nameInput.Reset()
	return m, tea.Batch(focus, m.nameInput.Focus())
}

// closeEditor closes the editor; the SEARCH input takes the keys back if
// it had them. A write in flight still reports its outcome.
func (m *Model) closeEditor() tea.Cmd {
	had := m.editor.inputHadFocus
	m.editor = libraryEditor{}
	m.nameInput.Blur()
	if had && m.top().kind == viewSearch {
		return m.input.Focus()
	}
	return nil
}

// editorBack is esc in the editor: back to the picker from a name input
// opened there, else closed.
func (m Model) editorBack() (Model, tea.Cmd) {
	if m.editor.mode == editName && m.editor.fromPicker {
		m.editor.mode = editPick
		m.nameInput.Blur()
		return m, nil
	}
	return m, m.closeEditor()
}

// editablePlaylists are the library playlists songs may be added to.
func (m Model) editablePlaylists() []playback.Playlist {
	var out []playback.Playlist
	for _, p := range m.stations {
		if p.Editable {
			out = append(out, p)
		}
	}
	return out
}

// handleEditorKey handles a key while the editor is open and the list has
// the focus. The name input types every key but enter and esc; the picker
// leaves the player keys, → and quit their usual meaning.
func (m Model) handleEditorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.editor.mode == editName {
		switch k {
		case keyEnter:
			return m.createPlaylist()
		case keyEsc:
			return m.editorBack()
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
	n := len(m.editablePlaylists()) + 1
	switch k {
	case keyUp:
		if m.editor.cursor == 0 {
			m.focusTabs()
			return m, nil
		}
		m.editor.cursor--
	case keyDown:
		m.editor.cursor = min(m.editor.cursor+1, n-1)
	case keyEnter:
		return m.pickerEnter()
	case keyEsc:
		return m.editorBack()
	case keyRight:
		m.focusPlayer(ctlPlay)
	case keyExpandAlt:
		return m.toggleExpand()
	case keyQuit:
		return m, m.quitCmd()
	default:
		if next, cmd, ok := m.playerKey(k); ok {
			return next, cmd
		}
	}
	return m, nil
}

// startWrite numbers a library write for the editor, unless one is in
// flight: that one must answer first, whatever the editor showed since.
func (m *Model) startWrite() (seq uint64, ok bool) {
	if m.libraryWriting {
		m.setStatus("WRITING… // WAIT FOR THE LIBRARY TO ANSWER")
		return 0, false
	}
	m.editSeq++
	m.libraryWriting, m.editor.seq = true, m.editSeq
	return m.editSeq, true
}

// pickerEnter acts on the selected picker row: + NEW PLAYLIST asks for a
// name; a playlist gets the song. The picker closes once the song is in.
func (m Model) pickerEnter() (Model, tea.Cmd) {
	if m.editor.cursor <= 0 {
		return m.openName(m.editor.song, true)
	}
	editable := m.editablePlaylists()
	if m.editor.cursor > len(editable) {
		return m, nil
	}
	pl, s := editable[m.editor.cursor-1], m.editor.song
	seq, ok := m.startWrite()
	if !ok {
		return m, nil
	}
	player := m.player
	m.setStatus("ADDING " + strings.ToUpper(s.Title) + " TO " + strings.ToUpper(pl.Name))
	return m, func() tea.Msg {
		ctx, cancel := m.libraryCtx()
		defer cancel()
		return addedMsg{seq: seq, song: s, playlist: pl, err: player.AddToPlaylist(ctx, pl.ID, []string{s.ID})}
	}
}

// onAdded settles an add: the picker closes on success and stays open,
// to try again, on a refusal. An add of unknown outcome closes it too: a
// second one could add the song twice.
func (m Model) onAdded(msg addedMsg) (Model, tea.Cmd) {
	mine := m.editor.mode != editClosed && m.editor.seq == msg.seq
	m.libraryWriting = false
	switch {
	case msg.err != nil && outcomeUnknown(msg.err):
		m.setStatus("ADD OUTCOME UNKNOWN // CHECK " + strings.ToUpper(msg.playlist.Name) + " BEFORE TRYING AGAIN")
	case msg.err != nil:
		m.setStatus("ADD FAILED // " + msg.err.Error())
		return m, nil
	default:
		m.setStatus("ADDED " + strings.ToUpper(msg.song.Title) + " TO " + strings.ToUpper(msg.playlist.Name))
	}
	if mine {
		return m, m.closeEditor()
	}
	return m, nil
}

// createPlaylist creates a library playlist named as typed, holding the
// editor's song, if any.
func (m Model) createPlaylist() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.setStatus("NAME THE PLAYLIST FIRST")
		return m, nil
	}
	s := m.editor.song
	var ids []string
	if s.ID != "" {
		ids = []string{s.ID}
	}
	seq, ok := m.startWrite()
	if !ok {
		return m, nil
	}
	player := m.player
	m.setStatus("CREATING " + strings.ToUpper(name))
	return m, func() tea.Msg {
		ctx, cancel := m.libraryCtx()
		defer cancel()
		pl, err := player.CreatePlaylist(ctx, name, "", ids)
		return createdMsg{seq: seq, name: name, song: s, playlist: pl, err: err}
	}
}

// onCreated settles a create. The new playlist is listed and selected at
// once, the editor closes, and the list is read again; the API may take a
// moment to list a new playlist, so it is kept until it does. On a
// refusal the name input stays, with its name, to try again. A create of
// unknown outcome closes the editor instead, a second one could make a
// twin, and the list read again selects the playlist if it shows.
func (m Model) onCreated(msg createdMsg) (Model, tea.Cmd) {
	mine := m.editor.mode != editClosed && m.editor.seq == msg.seq
	m.libraryWriting = false
	if msg.err != nil && outcomeUnknown(msg.err) {
		m.setStatus("CREATE OUTCOME UNKNOWN // CHECK THE PLAYLISTS BEFORE TRYING AGAIN")
		name := cleanLine(msg.name)
		check := createCheck{name: name}
		for _, p := range m.stations {
			if strings.EqualFold(p.Name, name) {
				check.known = append(check.known, p.ID)
			}
		}
		m.createCheck = &check
		var closed tea.Cmd
		if mine {
			closed = m.closeEditor()
		}
		return m, tea.Batch(closed, m.loadPlaylistsCmd())
	}
	if msg.err != nil {
		m.setStatus("CREATE FAILED // " + msg.err.Error())
		return m, nil
	}
	pl := msg.playlist
	pl.Name = cleanLine(pl.Name)
	if pl.Name == "" {
		pl.Name = cleanLine(msg.name)
	}
	pl.Editable = true // the user's own
	m.created = append(slices.Clone(m.created), pl)
	m.stations = withCreated(m.stations, m.created)
	m.setStationCursor(slices.IndexFunc(m.stations, func(p playback.Playlist) bool { return p.ID == pl.ID }))
	notice := "CREATED " + strings.ToUpper(pl.Name)
	if msg.song.ID != "" {
		notice += " WITH " + strings.ToUpper(msg.song.Title)
	}
	m.setStatus(notice)
	var closed tea.Cmd
	if mine {
		closed = m.closeEditor()
	}
	return m, tea.Batch(closed, m.loadPlaylistsCmd())
}

// withCreated lists the playlists created here among pls, alphabetically,
// those pls lacks.
func withCreated(pls, created []playback.Playlist) []playback.Playlist {
	out := slices.Clone(pls)
	for _, c := range created {
		if slices.ContainsFunc(out, func(p playback.Playlist) bool { return p.ID == c.ID }) {
			continue
		}
		at := len(out)
		for i, p := range out {
			if strings.ToLower(p.Name) > strings.ToLower(c.Name) {
				at = i
				break
			}
		}
		out = slices.Insert(out, at, c)
	}
	return out
}

// onPlaylists lists the library playlists read, keeping the ones created
// here that the API does not list yet, and the selected one selected.
func (m Model) onPlaylists(pls []playback.Playlist) Model {
	var selected string
	if c := m.stationCursor(); c >= 0 && c < len(m.stations) {
		selected = m.stations[c].ID
	}
	m.created = slices.DeleteFunc(slices.Clone(m.created), func(c playback.Playlist) bool {
		return slices.ContainsFunc(pls, func(p playback.Playlist) bool { return p.ID == c.ID })
	})
	m.stations = withCreated(pls, m.created)
	if i := slices.IndexFunc(m.stations, func(p playback.Playlist) bool { return p.ID == selected }); selected != "" && i >= 0 {
		m.setStationCursor(i)
	} else {
		m.setStationCursor(min(m.stationCursor(), max(len(m.stations)-1, 0)))
	}
	return m.settleCreateCheck()
}

// settleCreateCheck looks for the playlist of a create of unknown outcome
// in the playlists just read, and selects it when it shows.
func (m Model) settleCreateCheck() Model {
	check := m.createCheck
	if check == nil {
		return m
	}
	m.createCheck = nil
	i := slices.IndexFunc(m.stations, func(p playback.Playlist) bool {
		return strings.EqualFold(p.Name, check.name) && !slices.Contains(check.known, p.ID)
	})
	if i >= 0 {
		m.setStationCursor(i)
		m.setStatus("FOUND " + strings.ToUpper(m.stations[i].Name) + " // IT WAS CREATED")
	}
	return m
}

// newPlaylistLine renders the + NEW PLAYLIST row in w cells.
func newPlaylistLine(selected bool, w int) string {
	const label = "+ NEW PLAYLIST"
	if selected {
		return stSelected.Render(fit("▌▶ "+label, w))
	}
	return fit("   "+stYellow.Render(label), w)
}

// editorTitle and editorCode head and foot the list panel while the
// editor is open.
func (m Model) editorTitle() string {
	if m.editor.mode == editName {
		return "NEW PLAYLIST"
	}
	return "ADD TO PLAYLIST"
}

func (m Model) editorCode() string {
	switch {
	case m.libraryWriting:
		return "WRITING"
	case m.editor.mode == editName:
		return "LIBRARY WRITE"
	}
	return fmt.Sprintf("EDITABLE %02d", len(m.editablePlaylists()))
}

// songHead is the line naming the editor's song: title and artist.
func songHead(s playback.Song, w int) string {
	line := " " + stCyan.Render("♪ ") + stYellowB.Render(strings.ToUpper(s.Title))
	if s.Artist != "" {
		line += stMuted.Render(" · " + strings.ToUpper(s.Artist))
	}
	return fit(line, w)
}

// editorBody renders the editor in w x h cells. Every line is exactly w
// cells wide.
func (m Model) editorBody(w, h int) ([]string, zones) {
	if w <= 0 || h <= 0 {
		return nil, nil
	}
	if m.editor.mode == editName {
		return m.nameBody(w, h)
	}
	return m.pickerBody(w, h)
}

// pickerBody renders ADD TO PLAYLIST: the song, a rule, then + NEW
// PLAYLIST and the editable playlists with their frequencies, scrolled so
// the cursor stays on screen. Its zones are the rows shown.
func (m Model) pickerBody(w, h int) ([]string, zones) {
	lines := []string{songHead(m.editor.song, w)}
	if h > 1 {
		lines = append(lines, stFrameDim.Render(strings.Repeat("─", w)))
	}
	editable := m.editablePlaylists()
	room := h - len(lines)
	cur := m.editor.cursor
	var zs zones
	for i := max(0, cur-room+1); i <= len(editable) && len(lines) < h; i++ {
		zs.add(rowZone(i), 0, len(lines), w)
		if i == 0 {
			lines = append(lines, newPlaylistLine(cur == 0, w))
			continue
		}
		p := editable[i-1]
		freq := frequency(slices.IndexFunc(m.stations, func(s playback.Playlist) bool { return s.ID == p.ID }))
		name := strings.ToUpper(p.Name)
		if i == cur {
			lines = append(lines, stSelected.Render(fit("▌▶ "+freq+"  "+name, w)))
		} else {
			lines = append(lines, fit("   "+stMuted.Render(freq)+"  "+stRed.Render(name), w))
		}
	}
	if len(editable) == 0 && len(lines) < h {
		lines = append(lines, fit(" "+stDim.Render("NO EDITABLE PLAYLISTS"), w))
	}
	return lines, zs
}

// nameBody renders NEW PLAYLIST: the name input, a rule, the song the
// playlist will hold, if any, and the CREATE and CANCEL buttons.
func (m Model) nameBody(w, h int) ([]string, zones) {
	lines := []string{fit(stYellow.Render("+ ")+m.nameInput.View(), w)}
	add := func(l string) {
		if len(lines) < h {
			lines = append(lines, fit(l, w))
		}
	}
	add(stFrameDim.Render(strings.Repeat("─", w)))
	if s := m.editor.song; s.ID != "" {
		add(" " + stMuted.Render("WITH"))
		add(songHead(s, w))
	}
	add("")
	var zs zones
	if len(lines) < h {
		bar, bz := buttonBar([]button{
			{id: zoneEditCreate, label: "CREATE", tone: stYellow},
			{id: zoneEditCancel, label: "CANCEL", tone: stRed},
		}, w-1)
		zs.addAt(1, len(lines), bz)
		add(" " + bar)
	}
	return lines, zs
}

// clickEditorZone acts on a click while the editor is open; ok is false
// for the zones it leaves to the list (the nav tabs close it first).
func (m Model) clickEditorZone(z zone) (next tea.Model, cmd tea.Cmd, ok bool) {
	if row, isRow := rowOf(z.id); isRow && m.editor.mode == editPick {
		m.editor.cursor = row
		next, cmd = m.pickerEnter()
		return next, cmd, true
	}
	switch z.id {
	case zoneEditCreate:
		next, cmd = m.createPlaylist()
		return next, cmd, true
	case zoneEditCancel:
		next, cmd = m.editorBack()
		return next, cmd, true
	case zoneTabStations, zoneTabSearch:
		closed := m.closeEditor()
		next, cmd := m.clickListZone(z)
		return next, tea.Batch(closed, cmd), true
	}
	return m, nil, false
}
