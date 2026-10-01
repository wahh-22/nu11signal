package radio

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	// Text msg brought scrambles in (see intro.go), the tick raised for it.
	nm = nm.withIntro(m, msg)
	// A flip of the spectrum area between the emblem and the rain swaps
	// them over its own frames (see vizswap.go).
	nm, swapped := nm.withSwap(m)
	// A boot or a shutdown that starts needs its frames and its end on
	// time.
	booted := m.bootEnd.IsZero() && !nm.bootEnd.IsZero()
	shut := !m.shutdown && nm.shutdown
	if booted || shut || ((nm.intro.seq != m.intro.seq || swapped) && !nm.tickFast) {
		tick := nm.scheduleTick()
		return nm, tea.Batch(cmd, tick)
	}
	return nm, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m = m.startBoot()
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
		if m.shutdown {
			return m.onShutdownTick()
		}
		return m.onTick(msg)
	case closedMsg:
		return m.onClosed()
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
	case configMsg:
		return m.onConfig(msg), nil
	case releaseMsg:
		return m.onRelease(msg), nil
	case configSavedMsg:
		if msg.err != nil {
			m.setStatus("SETTINGS NOT SAVED // " + msg.err.Error())
		}
		return m, nil
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
		var reread tea.Cmd
		m, reread = m.followVolumeMode(msg.state.VolumeMode)
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
	// Refused access wins over the boot at once.
	switch {
	case msg.err != nil:
		m.auth, m.authDetail, m.boot = authFailed, msg.err.Error(), false
		return m, nil
	case msg.status != playback.AuthAuthorized:
		m.auth, m.authDetail, m.boot = authFailed, "music library access: "+string(msg.status), false
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
	m = m.trackPlay()
	// With nothing left to seek in, the bar focus falls back to the
	// button below it; with no song, the favorite focus to PLAY.
	m.onBar = m.onBar && m.seekable()
	if _, ok := m.playingSong(); !ok && m.control == ctlFav {
		m.control = ctlPlay
	}
	return m
}

func (m Model) onTick(msg tickMsg) (tea.Model, tea.Cmd) {
	m.frame++
	m = m.endBootOnTime()
	m.fx = m.fx.advance(m.now(), m.seed, m.fxActive())
	m = m.trackPlay().pollLevels()
	if m.animDue(msg) {
		m = m.animate()
	}
	if m.status != "" && !m.now().Before(m.statusUntil) {
		m.status = ""
	}
	// The favorite states shown are read here, not on every cursor move,
	// so a held arrow key never floods the player.
	m, read := m.readFavorites()
	return m, tea.Batch(m.scheduleTick(), read)
}

// animDue reports whether tick msg steps the animation. A tick at the
// animation's own interval always does (and sets its pace from now);
// a redraw the effects brought early (an intro at introTick, a burst at
// burstTick) only once the animation interval has passed since the
// last step was due, so the effects never change how fast the bars,
// the rain or the title glitch move. A step overdue by more than an
// interval (the chain slept, or was rescheduled) starts the pace anew
// instead of catching up.
func (m *Model) animDue(msg tickMsg) bool {
	now, d := m.now(), m.animInterval()
	if !msg.redraw || m.animAt.IsZero() {
		m.animAt = now
		return true
	}
	if now.Sub(m.animAt) < d {
		return false
	}
	m.animAt = m.animAt.Add(d)
	if now.Sub(m.animAt) >= d {
		m.animAt = now
	}
	return true
}

// animate takes one animation step: the bars, the rain after them, and
// the title's song-change glitch.
func (m Model) animate() Model {
	m.animFrame++
	m = m.stepBars().stepViz()
	if m.glitch > 0 {
		m.glitch--
	}
	return m
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.shutdown {
		return m.shutdownKey(k)
	}
	if m.quitAsk {
		return m.quitKey(k)
	}
	if m.boot {
		return m.bootKey(k)
	}
	if k == keyCtrlC {
		return m.askQuit(), nil
	}
	if next, ok := m.helpKey(k); ok {
		return next, nil
	}
	if next, cmd, ok := m.settingsKey(k); ok {
		return next, cmd
	}
	if k == keyQuit && !m.keysTyped() {
		// Before any area takes it, so the focus and the view stay put
		// under the quit modal and come back as they were.
		return m.askQuit(), nil
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
		if k == keyEsc {
			return m.askQuit(), nil
		}
		return m, nil
	}

	switch k {
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
	case keyEffects:
		return m.toggleEffects(), nil
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

// playCmd runs play request number seq, which queues the songs queue;
// the on-air station and queue change only once the player confirms it
// (see onPlay).
func (m Model) playCmd(seq uint64, op, station string, queue []playback.Song, request func(context.Context) (playback.QueueReport, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		report, err := request(ctx)
		return playMsg{seq: seq, op: op, station: station, queue: queue, report: report, err: err}
	}
}

// onPlay settles a play request. Failures are always reported, but only
// the latest request may set the on-air station or report the songs left
// out of its queue: a superseded tune that confirms late must not
// overwrite a newer one. The on-air queue is the songs it asked for, but
// those the report left out; none when only the start song plays.
func (m Model) onPlay(msg playMsg) Model {
	if msg.err != nil {
		m.setStatus(fmt.Sprintf("%s FAILED // %s", msg.op, msg.err))
		return m
	}
	if msg.seq == m.playSeq {
		m.playingStation = msg.station
		m.onAirQueue = queued(msg.queue, msg.report)
		if s := queueNotice(msg.report); s != "" {
			m.setStatus(s)
		}
	}
	return m
}

// queued is the songs of queue the player queued, as report tells: none
// when it started the first alone, else all but the missing and skipped.
func queued(queue []playback.Song, r playback.QueueReport) []playback.Song {
	if r.StartedAlone {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(queue), func(s playback.Song) bool {
		return slices.Contains(r.Missing, s.ID) || slices.Contains(r.Skipped, s.ID)
	})
}

// upNext is the song NEXT moves to in the on-air queue: the one after the
// song playing, or the first after the last while the player repeats.
// ok is false unless the song playing is in that queue exactly once (the
// player may have moved to a song the model did not queue), and when
// nothing follows it.
func (m Model) upNext() (playback.Song, bool) {
	cur, ok := m.playingSong()
	q := m.onAirQueue
	if !ok || len(q) < 2 {
		return playback.Song{}, false
	}
	same := func(s playback.Song) bool { return s.ID == cur.ID }
	i := slices.IndexFunc(q, same)
	switch {
	case i < 0 || slices.IndexFunc(q[i+1:], same) >= 0:
		return playback.Song{}, false
	case i+1 < len(q):
		return q[i+1], true
	case m.state.Repeat == playback.RepeatOff:
		return playback.Song{}, false
	}
	return q[0], true
}

// queueNotice is the status line for a play that left songs out of its
// queue: only the start song playing, or how many songs were left out;
// empty for a clean play.
func queueNotice(r playback.QueueReport) string {
	switch left := len(r.Missing) + len(r.Skipped); {
	case r.StartedAlone:
		return "PLAYING ALONE // QUEUE REFUSED"
	case left == 1:
		return "1 SONG SKIPPED // NOT IN QUEUE"
	case left > 1:
		return fmt.Sprintf("%d SONGS SKIPPED // NOT IN QUEUE", left)
	}
	return ""
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
