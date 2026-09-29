package radio

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(m.inputWidth())
		m.nameInput.SetWidth(m.inputWidth())
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tickMsg:
		if msg.gen != m.tickGen {
			return m, nil // superseded chain
		}
		return m.onTick()
	case authMsg:
		return m.onAuth(msg)
	case playlistsMsg:
		if msg.err != nil {
			m.stationsFailed = true
			m.setStatus("[R] RETRY // PLAYLIST SCAN FAILED // " + msg.err.Error())
			return m, nil
		}
		m.stationsFailed = false
		return m.onPlaylists(cleanEach(msg.playlists, func(p playback.Playlist) playback.Playlist {
			p.Name = cleanLine(p.Name)
			return p
		})), nil
	case recentsMsg:
		return m.onRecents(msg), nil
	case recentSavedMsg:
		if msg.err != nil {
			m.setStatus("RECENT SEARCH NOT SAVED // " + msg.err.Error())
		}
		return m, nil
	case recentEditedMsg:
		if msg.err != nil {
			m.setStatus("RECENT CHANGE NOT SAVED // " + msg.err.Error())
		}
		return m, nil
	case searchDebounceMsg:
		return m.onSearchDebounce(msg)
	case catalogMsg:
		current := msg.seq == m.search.seq
		m = m.onCatalog(msg)
		if !current || msg.err != nil {
			return m, nil
		}
		return m.prefetchFavorites(msg.results.Songs)
	// A page that loaded reads the favorite states of its songs; an
	// answer for a page left or reloaded (not fresh) reads none.
	case artistMsg:
		var fresh bool
		m, fresh = m.onArtist(msg)
		if !fresh || msg.err != nil {
			return m, nil
		}
		return m.prefetchFavorites(msg.detail.TopSongs)
	case albumMsg:
		var fresh bool
		m, fresh = m.onAlbum(msg)
		if !fresh || msg.err != nil {
			return m, nil
		}
		songs := make([]playback.Song, len(msg.detail.Tracks))
		for i, t := range msg.detail.Tracks {
			songs[i] = t.Song
		}
		return m.prefetchFavorites(songs)
	case playlistMsg:
		var fresh bool
		m, fresh = m.onPlaylist(msg)
		if !fresh || msg.err != nil {
			return m, nil
		}
		return m.prefetchFavorites(msg.detail.Tracks)
	case resultsMsg:
		var fresh bool
		m, fresh = m.onResults(msg)
		if !fresh || msg.err != nil {
			return m, nil
		}
		var songs []playback.Song
		for _, it := range msg.found.Top {
			if it.Kind == playback.ItemSong {
				songs = append(songs, it.Song)
			}
		}
		return m.prefetchFavorites(append(songs, msg.found.Songs...))
	case actionMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("%s FAILED // %s", msg.op, msg.err))
		}
		return m, nil
	case playMsg:
		return m.onPlay(msg), nil
	case favoriteMsg:
		return m.onFavorite(msg), nil
	case favoritesMsg:
		return m.onFavorites(msg), nil
	case setFavoriteMsg:
		return m.onSetFavorite(msg)
	case addedMsg:
		return m.onAdded(msg)
	case createdMsg:
		return m.onCreated(msg)
	case seekMsg:
		return m.onSeek(msg), nil
	case loopMsg:
		return m.onLoop(msg), nil
	case volumeMsg:
		return m.onVolume(msg)
	case setVolumeMsg:
		return m.onSetVolume(msg)
	case stateMsg:
		m = m.onState(msg.state)
		m, reread := m.followVolumeMode(msg.state.VolumeMode)
		if m.animating() && !m.tickFast {
			return m, tea.Batch(m.waitStates(), m.scheduleTick(), reread)
		}
		return m, tea.Batch(m.waitStates(), reread)
	case statesClosedMsg:
		m.lostState = true
		return m, nil
	case playerErrMsg:
		m.setStatus("SIGNAL ERROR // " + msg.err.Error())
		return m, m.waitErrors()
	case errorsClosedMsg:
		m.lostErrs = true
		return m, nil
	}
	if m.editor.mode == editName {
		// Cursor blinks and other input internals of the name.
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
	if m.top().kind == viewSearch {
		// Cursor blinks and other input internals.
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) onAuth(msg authMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.auth, m.authDetail = authFailed, msg.err.Error()
		return m, nil
	case msg.status != playback.AuthAuthorized:
		m.auth, m.authDetail = authFailed, "music library access: "+string(msg.status)
		return m, nil
	}
	m.auth = authOK
	// States that failed to read without access are read again.
	m.clearFailedFavorites()
	return m, m.loadPlaylistsCmd()
}

func (m Model) onState(s playback.State) Model {
	if s.SongID != m.state.SongID || s.Title != m.state.Title {
		m.glitch = glitchFrames
		m.seekPending = false // the pending target belonged to another song
	}
	m.state, m.hasState, m.stateAt = cleanState(s), true, m.now()
	m.confirmLoop()
	// With nothing left to seek in, the bar focus falls back to the
	// button below it; with no song, the ♥ focus to PLAY.
	m.onBar = m.onBar && m.seekable()
	if _, ok := m.playingSong(); !ok && m.control == ctlFav {
		m.control = ctlPlay
	}
	return m
}

func (m Model) onTick() (tea.Model, tea.Cmd) {
	m.frame++
	m.bars = m.bars.step(m.isPlaying(), m.seed, m.frame)
	if m.glitch > 0 {
		m.glitch--
	}
	if m.status != "" && !m.now().Before(m.statusUntil) {
		m.status = ""
	}
	// The favorite states shown are read here, not on every cursor move,
	// so a held arrow key never floods the player.
	m, read := m.readFavorites()
	return m, tea.Batch(m.scheduleTick(), read)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == keyCtrlC {
		return m, m.quitCmd()
	}
	if m.auth != authFailed {
		if m.focus == areaTabs {
			if next, cmd, ok := m.handleTabsKey(k); ok {
				return next, cmd
			}
			// Any other key belongs to the area the tabs took the focus
			// from, which takes it back.
			back := m.leaveTabs()
			next, cmd := m.handleKey(msg)
			return next, tea.Batch(back, cmd)
		}
		if m.focus == areaPlayer {
			if next, cmd, ok := m.handlePlayerKey(k); ok {
				return next, cmd
			}
			// Any other key belongs to the list, which takes the focus back.
			focus := m.focusList()
			next, cmd := m.handleKey(msg)
			return next, tea.Batch(focus, cmd)
		}
		// Keys no view takes, the SEARCH input included.
		switch k {
		case keySeekBack:
			return m.seek(-seekStep)
		case keySeekForward:
			return m.seek(seekStep)
		case keyVolumeUpAnywhere:
			return m.stepVolume(volumeStep)
		case keyVolumeDownAnywhere:
			return m.stepVolume(-volumeStep)
		case keyExpand:
			return m.toggleExpand()
		}
		if m.editor.mode != editClosed {
			return m.handleEditorKey(msg)
		}
	}
	if m.top().kind == viewSearch {
		return m.handleSearchKey(msg)
	}
	if isPage(m.top().kind) {
		if next, cmd, ok := m.handlePageKey(k); ok {
			return next, cmd
		}
	}
	if m.auth == authFailed {
		if k == keyQuit || k == keyEsc {
			return m, m.quitCmd()
		}
		return m, nil
	}

	switch k {
	case keyQuit:
		return m, m.quitCmd()
	case keyUp:
		if m.atListTop() {
			m.focusTabs()
			return m, nil
		}
		m.moveCursor(-1)
	case keyDown:
		m.moveCursor(1)
	case keyEnter:
		return m.openSelection()
	case keyRight:
		m.focusPlayer(ctlPlay)
	case keyExpandAlt:
		return m.toggleExpand()
	case keySearch, keyTab:
		// Both bring back the search branch tab left, else a fresh search.
		return m.resumeOrOpenSearch()
	case keyEsc:
		// Back navigation for views pushed over the stations; on the
		// stations root there is nothing to pop.
		m.pop()
	case keyRetry:
		if m.stationsFailed {
			m.stationsFailed = false
			m.setStatus("RESCANNING PLAYLISTS")
			return m, m.loadPlaylistsCmd()
		}
	case keyLove:
		// No song row here: the song playing.
		return m.loveTarget()
	case keyAdd:
		return m.addTarget()
	default:
		if next, cmd, ok := m.playerKey(k); ok {
			return next, cmd
		}
	}
	return m, nil
}

// togglePlay pauses while playing, else resumes.
func (m Model) togglePlay() tea.Cmd {
	if m.isPlaying() {
		return m.action("PAUSE", m.player.Pause)
	}
	return m.action("RESUME", m.player.Resume)
}

// moveCursor moves the stations cursor, the only list handleKey drives,
// from the + NEW PLAYLIST row to the last playlist.
func (m *Model) moveCursor(delta int) {
	m.setStationCursor(max(m.firstStationRow(), min(m.stationCursor()+delta, len(m.stations)-1)))
}

// openSelection opens the page of the selected library playlist, which
// plays it, or on + NEW PLAYLIST asks for the new playlist's name.
func (m Model) openSelection() (tea.Model, tea.Cmd) {
	if m.stationCursor() < 0 {
		return m.openName(playback.Song{}, false)
	}
	if len(m.stations) == 0 {
		return m, nil
	}
	return m.openLibraryPlaylist(m.stations[m.stationCursor()])
}

// playCmd runs play request number seq; the on-air station changes only
// once the player confirms it (see onPlay).
func (m Model) playCmd(seq uint64, op, station string, request func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		return playMsg{seq: seq, op: op, station: station, err: request(ctx)}
	}
}

// onPlay settles a play request. Failures are always reported, but only
// the latest request may set the on-air station: a superseded tune that
// confirms late must not overwrite a newer one.
func (m Model) onPlay(msg playMsg) Model {
	if msg.err != nil {
		m.setStatus(fmt.Sprintf("%s FAILED // %s", msg.op, msg.err))
		return m
	}
	if msg.seq == m.playSeq {
		m.playingStation = msg.station
	}
	return m
}

// seekable reports whether the song playing has a known length to seek
// in.
func (m Model) seekable() bool { return m.hasState && m.state.Duration > 0 }

// seek jumps delta from the position, or from the pending seek's target.
func (m Model) seek(delta time.Duration) (Model, tea.Cmd) {
	if !m.seekable() {
		return m, nil
	}
	from := m.position()
	if m.seekPending {
		from = m.seekTarget
	}
	return m.seekTo(from + delta)
}

// seekTo jumps to target, clamped to the song.
func (m Model) seekTo(target time.Duration) (Model, tea.Cmd) {
	if !m.seekable() {
		return m, nil
	}
	target = clampDuration(target, 0, m.state.Duration)
	m.seekSeq++
	m.seekPending, m.seekTarget = true, target
	seq := m.seekSeq
	return m, func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		return seekMsg{seq: seq, target: target, err: m.player.Seek(ctx, target)}
	}
}

// onSeek settles the latest seek; answers to superseded seeks only report
// failures.
func (m Model) onSeek(msg seekMsg) Model {
	if msg.err != nil {
		m.setStatus(fmt.Sprintf("SEEK FAILED // %s", msg.err))
	}
	if msg.seq != m.seekSeq || !m.seekPending {
		return m
	}
	m.seekPending = false
	if msg.err == nil {
		// The player confirmed the jump; show it until the next state.
		m.state.Position, m.stateAt = msg.target, m.now()
	}
	return m
}
