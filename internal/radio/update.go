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
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
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
			m.setStatus("[R] RETRY // STATION LIST FAILED // " + msg.err.Error())
			return m, nil
		}
		m.stationsFailed = false
		m.stations = cleanEach(msg.playlists, func(p playback.Playlist) playback.Playlist {
			p.Name = cleanLine(p.Name)
			return p
		})
		m.setStationCursor(min(m.stationCursor(), max(len(m.stations)-1, 0)))
		return m, nil
	case recentsMsg:
		return m.onRecents(msg), nil
	case recentSavedMsg:
		if msg.err != nil {
			m.setStatus("RECENT SEARCH NOT SAVED // " + msg.err.Error())
		}
		return m, nil
	case searchDebounceMsg:
		return m.onSearchDebounce(msg)
	case catalogMsg:
		return m.onCatalog(msg), nil
	case artistMsg:
		return m.onArtist(msg), nil
	case albumMsg:
		return m.onAlbum(msg), nil
	case playlistMsg:
		return m.onPlaylist(msg), nil
	case actionMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("%s FAILED // %s", msg.op, msg.err))
		}
		return m, nil
	case playMsg:
		return m.onPlay(msg), nil
	case seekMsg:
		return m.onSeek(msg), nil
	case stateMsg:
		m = m.onState(msg.state)
		if m.animating() && !m.tickFast {
			return m, tea.Batch(m.waitStates(), m.scheduleTick())
		}
		return m, m.waitStates()
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
	return m, m.loadPlaylistsCmd()
}

func (m Model) onState(s playback.State) Model {
	if s.SongID != m.state.SongID || s.Title != m.state.Title {
		m.glitch = glitchFrames
		m.seekPending = false // the pending target belonged to another song
	}
	m.state, m.hasState, m.stateAt = cleanState(s), true, m.now()
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
	return m, m.scheduleTick()
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == keyCtrlC {
		return m, m.quitCmd()
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
	case keyUp, keyUpAlt:
		m.moveCursor(-1)
	case keyDown, keyDownAlt:
		m.moveCursor(1)
	case keyEnter:
		return m.playSelection()
	case keySpace:
		if m.isPlaying() {
			return m, m.action("PAUSE", m.player.Pause)
		}
		return m, m.action("RESUME", m.player.Resume)
	case keyNext:
		return m, m.action("NEXT", m.player.Next)
	case keyPrev:
		return m, m.action("PREV", m.player.Previous)
	case keyBack:
		return m.seek(-seekStep)
	case keyForward:
		return m.seek(seekStep)
	case keySearch, keyTab:
		// Both bring back the search branch tab left, else a fresh search.
		return m.openSearch()
	case keyEsc:
		// Back navigation for views pushed over the stations; on the
		// stations root there is nothing to pop.
		m.pop()
	case keyRetry:
		if m.stationsFailed {
			m.stationsFailed = false
			m.setStatus("RESCANNING STATIONS")
			return m, m.loadPlaylistsCmd()
		}
	}
	return m, nil
}

// moveCursor moves the stations cursor, the only list handleKey drives.
func (m *Model) moveCursor(delta int) {
	m.setStationCursor(max(0, min(m.stationCursor()+delta, len(m.stations)-1)))
}

// playSelection tunes the selected station.
func (m Model) playSelection() (tea.Model, tea.Cmd) {
	if len(m.stations) == 0 {
		return m, nil
	}
	id := m.stations[m.stationCursor()].ID
	m.playSeq++
	return m, m.playCmd(m.playSeq, "TUNE", id, func(ctx context.Context) error {
		return m.player.PlayPlaylist(ctx, id)
	})
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

func (m Model) seek(delta time.Duration) (tea.Model, tea.Cmd) {
	if !m.hasState || m.state.Duration <= 0 {
		return m, nil
	}
	from := m.position()
	if m.seekPending {
		from = m.seekTarget
	}
	target := clampDuration(from+delta, 0, m.state.Duration)
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
