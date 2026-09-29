// Package radio is the Nu11Signal terminal UI: a Cyberpunk 2077 style car
// radio driving a playback.Player. It depends only on the playback port.
package radio

import (
	"context"
	"slices"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/history"
	"github.com/wahh-22/nu11signal/internal/playback"
)

// Options configures a radio Model. Zero values select sensible defaults.
type Options struct {
	// Now is the clock; defaults to time.Now. Tests inject a fixed clock.
	Now func() time.Time
	// Seed drives the decorative EQ and glitch animations.
	Seed uint64
	// CallTimeout bounds every Player call (default 8s).
	CallTimeout time.Duration
	// CloseTimeout bounds how long quitting waits for Player.Close before
	// the UI exits anyway (default 3s).
	CloseTimeout time.Duration
	// Recents keeps recent search terms; nil keeps them in memory only.
	Recents history.Recents
}

const (
	// searchLimit caps each kind of the live rows under the search input,
	// a dropdown kept short; resultsLimit caps each section of the RESULTS
	// page, which scrolls and so can hold more.
	searchLimit         = 10
	resultsLimit        = 25
	searchDebounce      = 250 * time.Millisecond
	minSearchRunes      = 2
	seekStep            = 10 * time.Second
	statusTTL           = 4 * time.Second
	glitchFrames        = 6
	fastTick            = 100 * time.Millisecond
	idleTick            = time.Second
	defaultCallTimeout  = 8 * time.Second
	defaultCloseTimeout = 3 * time.Second
)

type authPhase int

const (
	authPending authPhase = iota
	authOK
	authFailed
)

// viewKind names what the list panel shows.
type viewKind int

const (
	// viewStations is the root of the navigation stack.
	viewStations viewKind = iota
	// viewSearch is the catalog search; its state lives in Model.search so
	// that it survives being parked by tab and restored.
	viewSearch
	// viewArtist is an artist page; its state lives in its frame.
	viewArtist
	// viewAlbum is an album page, or a song's (the SONG view); its state
	// lives in its frame.
	viewAlbum
	// viewPlaylist is a catalog playlist page; its state lives in its
	// frame.
	viewPlaylist
	// viewResults is the full results of a search term; its state lives
	// in its frame.
	viewResults
)

// frame is one entry of the navigation stack: a view and its cursor. Browse
// pages keep the state they own per entry here, so that popping one
// reveals the previous entry untouched.
type frame struct {
	kind viewKind
	// cursor is the selected row; in the search view -1 selects the input.
	cursor int
	// artist is the page of a viewArtist entry.
	artist artistPage
	// tracks is the page of a viewAlbum or viewPlaylist entry.
	tracks trackPage
	// results is the page of a viewResults entry.
	results resultsPage
}

// Model is the Bubble Tea model of the radio.
type Model struct {
	player       playback.Player
	now          func() time.Time
	seed         uint64
	timeout      time.Duration
	closeTimeout time.Duration

	width, height int

	auth       authPhase
	authDetail string

	stations []playback.Playlist
	// stack is the navigation stack; stack[0] is the stations list and the
	// list panel shows the top entry.
	stack []frame
	// parked is the search branch (the SEARCH entry and the pages opened
	// from it) that tab left for the stations; / or tab brings it back as
	// it was. Its pages keep loading and settle here while parked.
	parked []frame
	// playingStation is the station the player confirmed tuning to.
	playingStation string
	// playSeq numbers play requests; only the answer to the latest one
	// may change playingStation.
	playSeq uint64
	// stationsFailed means loading the station list failed; r retries.
	stationsFailed bool

	input        textinput.Model
	search       searchState
	recents      []string
	recentsStore history.Recents
	// recentsWrites orders the changes to recentsStore; the Models of one
	// program share it.
	recentsWrites *recentsWriter
	// artistSeq numbers artist page loads (see artistPage.seq).
	artistSeq uint64
	// detailSeq numbers album and playlist page loads (see trackPage.seq).
	detailSeq uint64
	// resultsSeq numbers results page loads (see resultsPage.seq).
	resultsSeq uint64

	// seekPending holds the target of the latest seek (seekSeq) until the
	// player answers it, so rapid seeks accumulate instead of restarting
	// from a stale reported position.
	seekPending bool
	seekTarget  time.Duration
	seekSeq     uint64

	state     playback.State
	hasState  bool
	stateAt   time.Time
	lostState bool
	lostErrs  bool

	status      string
	statusUntil time.Time

	// pressGuardUntil ignores left presses until then, after one that
	// changed the view (see doubleClickGuard).
	pressGuardUntil time.Time

	// focus is the area taking the keys; on the player, control is the
	// selected button, or onBar the progress bar above them. The expanded
	// player takes the full width and keeps the focus, but for the nav
	// tabs (see focus.go). favFrom is the transport button ↑ left for the
	// ♥ of the song playing, which ↓ goes back to. inputHadFocus keeps,
	// while the list does not have the focus, whether the search input had
	// the keys before. On the nav tabs, tab is the selected one and
	// tabsFrom the area ↓ returns to.
	focus         focusArea
	control       playerControl
	favFrom       playerControl
	onBar         bool
	expanded      bool
	inputHadFocus bool
	tab           int
	tabsFrom      focusArea

	// favs caches the favorite state of songs by id, and favSeq numbers
	// its reads and changes (see library.go).
	favs   map[string]favorite
	favSeq uint64
	// editor is the ADD TO PLAYLIST picker or the NEW PLAYLIST name input
	// over the list panel; nameInput holds the name. editSeq numbers the
	// library writes; libraryWriting means one is in flight (see
	// library.go). created are the playlists created here that the API
	// does not list yet; createCheck, when set, is a create of unknown
	// outcome to look for in the next playlists read.
	editor         libraryEditor
	nameInput      textinput.Model
	editSeq        uint64
	libraryWriting bool
	created        []playback.Playlist
	createCheck    *createCheck

	// volume is the output level shown, and the latest one asked for;
	// volumeKnown is false until the player reports it, and again after
	// it refused a change. volumeBusy means a Volume or SetVolume call is
	// in flight (see volume.go); it starts true for the read Init sends.
	// volumePending holds steps pressed while a read was in flight.
	volume        float64
	volumeKnown   bool
	volumeBusy    bool
	volumePending float64
	// volumeMode is the volume the player last said it drives (app or
	// system; empty until it says): the readout's label. volumeEpoch
	// counts its changes; each volume call carries the epoch it was made
	// in, so an answer for an older mode is recognized.
	volumeMode  playback.VolumeMode
	volumeEpoch int

	// loopWant is the repeat mode LOOP asked for, shown while loopPending
	// (until a state reports it, the player refuses it or loopUntil);
	// loopSeq numbers the changes (see loop.go).
	loopWant    playback.RepeatMode
	loopPending bool
	loopUntil   time.Time
	loopSeq     uint64

	frame  uint64
	bars   eq
	glitch int
	// tickGen identifies the live tick chain; ticks from older chains are
	// dropped so rescheduling never doubles the frame rate.
	tickGen  uint64
	tickFast bool
}

// New returns a radio Model driving p.
func New(p playback.Player, opts Options) Model {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.CallTimeout <= 0 {
		opts.CallTimeout = defaultCallTimeout
	}
	if opts.CloseTimeout <= 0 {
		opts.CloseTimeout = defaultCloseTimeout
	}
	if opts.Recents == nil {
		opts.Recents = history.NewMemory()
	}
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "ARTISTS, SONGS"
	in.CharLimit = 120
	in.SetStyles(inputStyles())
	name := textinput.New()
	name.Prompt = ""
	name.Placeholder = "PLAYLIST NAME"
	name.CharLimit = 100
	name.SetStyles(inputStyles())
	return Model{
		player:       p,
		now:          opts.Now,
		seed:         opts.Seed,
		timeout:      opts.CallTimeout,
		closeTimeout: opts.CloseTimeout,
		stack:        []frame{{kind: viewStations}},
		volumeBusy:   true, // Init reads the volume

		input:         in,
		nameInput:     name,
		recentsStore:  opts.Recents,
		recentsWrites: newRecentsWriter(opts.Recents),
	}
}

// Init authorizes, loads recent searches, reads the volume, starts
// listening to the player, and starts animating.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.authorizeCmd(), m.loadRecentsCmd(), m.readVolumeCmd(0), m.waitStates(), m.waitErrors(), tickAfter(idleTick, m.tickGen))
}

// Messages produced by the model's commands.
type (
	authMsg struct {
		status playback.AuthStatus
		err    error
	}
	playlistsMsg struct {
		playlists []playback.Playlist
		err       error
	}
	// actionMsg reports the outcome of a fire-and-forget Player call.
	actionMsg struct {
		op  string
		err error
	}
	// playMsg reports play request number seq; station is the tuned
	// playlist, or empty when songs were played.
	playMsg struct {
		seq     uint64
		op      string
		station string
		err     error
	}
	// seekMsg reports the outcome of seek number seq.
	seekMsg struct {
		seq    uint64
		target time.Duration
		err    error
	}
	stateMsg        struct{ state playback.State }
	statesClosedMsg struct{}
	playerErrMsg    struct{ err error }
	errorsClosedMsg struct{}
	tickMsg         struct{ gen uint64 }
)

func (m Model) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), m.timeout)
}

func (m Model) authorizeCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		s, err := m.player.Authorize(ctx)
		return authMsg{status: s, err: err}
	}
}

// loadPlaylistsCmd lists the library playlists. The helper pages through
// the Apple Music API for them, so the call gets the detail timeout.
func (m Model) loadPlaylistsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), max(m.timeout, detailCallTimeout))
		defer cancel()
		pls, err := m.player.Playlists(ctx)
		return playlistsMsg{playlists: pls, err: err}
	}
}

// action runs one Player call with a timeout and reports its outcome.
func (m Model) action(op string, fn func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		return actionMsg{op: op, err: fn(ctx)}
	}
}

// quitCmd closes the player and quits, but never waits longer than
// closeTimeout: a stuck player must not keep the UI (and the terminal in
// raw mode) from exiting. The caller may still wait for Close afterwards,
// once the terminal is restored.
func (m Model) quitCmd() tea.Cmd {
	return func() tea.Msg {
		closed := make(chan struct{})
		go func() {
			_ = m.player.Close()
			close(closed)
		}()
		timer := time.NewTimer(m.closeTimeout)
		defer timer.Stop()
		select {
		case <-closed:
		case <-timer.C:
		}
		return tea.QuitMsg{}
	}
}

// waitStates delivers the next player state; it is re-armed after each one.
func (m Model) waitStates() tea.Cmd {
	ch := m.player.States()
	return func() tea.Msg {
		s, ok := <-ch
		if !ok {
			return statesClosedMsg{}
		}
		return stateMsg{state: s}
	}
}

// waitErrors delivers the next asynchronous player error.
func (m Model) waitErrors() tea.Cmd {
	ch := m.player.Errors()
	return func() tea.Msg {
		err, ok := <-ch
		if !ok {
			return errorsClosedMsg{}
		}
		return playerErrMsg{err: err}
	}
}

// scheduleTick starts a new tick chain for the next animation frame: fast
// while something moves, slow (for the clock) when idle, to keep the
// process cheap.
func (m *Model) scheduleTick() tea.Cmd {
	m.tickGen++
	m.tickFast = m.animating()
	d := idleTick
	if m.tickFast {
		d = fastTick
	}
	return tickAfter(d, m.tickGen)
}

func tickAfter(d time.Duration, gen uint64) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{gen: gen} })
}

func (m Model) animating() bool {
	return m.isPlaying() || !m.bars.flat() || m.glitch > 0
}

func (m Model) isPlaying() bool {
	return m.hasState && !m.signalLost() &&
		(m.state.Status == playback.StatusPlaying || m.state.Status == playback.StatusSeeking)
}

// position estimates the playback position now, advancing the last reported
// position by the time elapsed since it arrived while playing.
func (m Model) position() time.Duration {
	pos := m.state.Position
	if m.state.Status == playback.StatusPlaying && !m.signalLost() {
		pos += m.now().Sub(m.stateAt)
	}
	if m.state.Duration > 0 {
		pos = clampDuration(pos, 0, m.state.Duration)
	}
	return max(pos, 0)
}

// signalLost reports that the player shut down underneath the UI.
func (m Model) signalLost() bool { return m.lostState || m.lostErrs }

// top is the navigation entry the list panel shows.
func (m Model) top() frame { return m.stack[len(m.stack)-1] }

func (m Model) cursor() int { return m.top().cursor }

// onPlaylistsBranch reports whether the stack is the PLAYLISTS root and
// the pages opened from it (a library playlist), not the search branch.
func (m Model) onPlaylistsBranch() bool {
	return !slices.ContainsFunc(m.stack, func(f frame) bool { return f.kind == viewSearch })
}

// litTab is the zone ID of the nav tab lit for the branch shown.
func (m Model) litTab() string {
	if m.onPlaylistsBranch() {
		return zoneTabStations
	}
	return zoneTabSearch
}

// atListTop reports whether the cursor of the view on top is on its first
// row (on SEARCH, the input), where ↑ climbs to the nav tabs and the wheel
// stops.
func (m Model) atListTop() bool {
	switch m.top().kind {
	case viewStations:
		return m.stationCursor() <= m.firstStationRow()
	case viewSearch:
		return m.cursor() < 0
	}
	return m.cursor() <= 0
}

// stationCursor is the selected station: the cursor of the stations root,
// whatever view is on top.
func (m Model) stationCursor() int { return m.stack[0].cursor }

func (m *Model) setStationCursor(c int) { m.setCursorAt(0, c) }

// firstStationRow is the stations cursor of the top row: -1, the + NEW
// PLAYLIST row, once the library is reachable, else the first playlist.
func (m Model) firstStationRow() int {
	if m.auth == authOK {
		return -1
	}
	return 0
}

// The stack helpers copy the stack before changing it: Models are values,
// and an older copy must never see a newer one's navigation.

func (m *Model) push(f frame) { m.stack = append(slices.Clone(m.stack), f) }

// pop removes the top entry, cancelling its load; the stations root is
// never popped.
func (m *Model) pop() {
	if len(m.stack) > 1 {
		m.top().cancelLoad()
		m.stack = slices.Clone(m.stack[:len(m.stack)-1])
	}
}

// popToRoot removes every entry above the stations root, cancelling their
// loads.
func (m *Model) popToRoot() {
	for _, f := range m.stack[1:] {
		f.cancelLoad()
	}
	m.stack = slices.Clone(m.stack[:1])
}

// parkBranch leaves for the stations root keeping the entries above it,
// with their cursors, pages and loads, for restoreBranch. A branch parked
// before is replaced, so its loads are cancelled.
func (m *Model) parkBranch() {
	for _, f := range m.parked {
		f.cancelLoad()
	}
	m.parked = slices.Clone(m.stack[1:])
	m.stack = slices.Clone(m.stack[:1])
}

// restoreBranch puts the parked branch back over the stations root; ok is
// false when nothing is parked. A page that failed to load while parked
// reports its failure now, on screen.
func (m *Model) restoreBranch() (ok bool) {
	if len(m.parked) == 0 {
		return false
	}
	m.stack = append(slices.Clone(m.stack[:1]), m.parked...)
	m.parked = nil
	if failure := m.top().loadFailure(); failure != "" {
		m.setStatus(failure)
	}
	return true
}

// settleFrame replaces the first entry, on the stack or parked, that match
// accepts with settle's result. A failed load reaches the status line only
// from the stack: a parked page keeps its failure until it is restored.
// It reports whether an entry matched.
func (m *Model) settleFrame(match func(frame) bool, settle func(frame) frame) bool {
	for i, f := range m.stack {
		if match(f) {
			settled := settle(f)
			m.setFrame(i, settled)
			if failure := settled.loadFailure(); failure != "" {
				m.setStatus(failure)
			}
			return true
		}
	}
	for i, f := range m.parked {
		if match(f) {
			m.parked = slices.Clone(m.parked)
			m.parked[i] = settle(f)
			return true
		}
	}
	return false
}

// setTop replaces the top entry.
func (m *Model) setTop(f frame) { m.setFrame(len(m.stack)-1, f) }

// setFrame replaces stack entry i.
func (m *Model) setFrame(i int, f frame) {
	m.stack = slices.Clone(m.stack)
	m.stack[i] = f
}

// setCursor moves the cursor of the top entry.
func (m *Model) setCursor(c int) { m.setCursorAt(len(m.stack)-1, c) }

// setCursorAt moves the cursor of stack entry i.
func (m *Model) setCursorAt(i, c int) {
	m.stack = slices.Clone(m.stack)
	m.stack[i].cursor = c
}

// setStatus shows s on the status line; it may carry network text (an
// error from the helper), so it is cleaned first.
func (m *Model) setStatus(s string) {
	m.status = cleanLine(s)
	m.statusUntil = m.now().Add(statusTTL)
}
