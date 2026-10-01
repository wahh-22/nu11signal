// Package radio is the Nu11Signal terminal UI: a Cyberpunk 2077 style car
// radio driving a playback.Player. It depends only on the playback port.
package radio

import (
	"context"
	"slices"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/config"
	"github.com/wahh-22/nu11signal/internal/history"
	"github.com/wahh-22/nu11signal/internal/playback"
	updatecheck "github.com/wahh-22/nu11signal/internal/update"
)

// Options configures a radio Model. Zero values select sensible defaults.
type Options struct {
	// Now is the clock; defaults to time.Now. Tests inject a fixed clock.
	Now func() time.Time
	// Seed drives the decorative EQ and glitch animations.
	Seed uint64
	// Effects starts the signal effects on: glitch bursts and content
	// intros (see glitch.go and intro.go). Off by default, and in tests, so frames stay fixed;
	// keyEffects toggles them.
	Effects bool
	// SkipBoot starts past the boot splash (see boot.go), on the normal UI
	// from the first frame: tests that are not about the boot use it.
	SkipBoot bool
	// CallTimeout bounds every Player call (default 8s).
	CallTimeout time.Duration
	// CloseTimeout bounds how long quitting waits for Player.Close before
	// the UI exits anyway (default 6s: at least the helper client's own
	// bound, see helper.DefaultCloseTimeout, so the helper's fade out is
	// never cut short).
	CloseTimeout time.Duration
	// Recents keeps recent search terms; nil keeps them in memory only.
	Recents history.Recents
	// Config is the settings file, read once at startup (see onConfig);
	// nil keeps the defaults.
	Config config.Source
	// Updates answers the latest release, asked once from Init (see
	// release.go); nil never checks.
	Updates updatecheck.Checker
	// Version is the running release version, compared with the latest;
	// one that is not a semantic version ("dev") never checks.
	Version string
	// Upgrade is the command that upgrades this install, shown with a
	// newer release; empty shows the release page instead.
	Upgrade string
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
	defaultCloseTimeout = 6 * time.Second
)

// RenderFPS is the frame rate the program's renderer should run at
// (tea.WithFPS). Bubble Tea's default, 60, wakes the process 60 times a
// second even when nothing changed, most of its idle cost; 20 frames a
// second is a frame period (50 ms) no longer than any animation step
// (fastTick, introTick, burstTick), so no animation frame is
// skipped, and it keeps a key press on screen within 50 ms.
const RenderFPS = 20

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
	// onAirQueue is the songs the latest confirmed play queued, in order
	// (see onPlay); the status line names the one after the song playing
	// (see upNext).
	onAirQueue []playback.Song
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
	// favorite of the song playing, which ↓ goes back to. inputHadFocus
	// keeps, while the list does not have the focus, whether the search
	// input had the keys before. On the nav tabs, tab is the selected one
	// and tabsFrom the area ↓ returns to.
	focus         focusArea
	control       playerControl
	favFrom       playerControl
	onBar         bool
	expanded      bool
	inputHadFocus bool
	tab           int
	tabsFrom      focusArea
	// help shows the KEYS overlay (see help.go), which takes every key
	// but keyHelp and esc (closing it) and ctrl+c (asking to quit).
	help bool
	// settings shows the SETTINGS overlay (see settings.go), its cursor
	// on the theme row settingsCursor. theme is the theme applied (see
	// setTheme); themePicked means SETTINGS chose one, which the settings
	// file read at startup no longer overrides. settingsFile is the
	// settings as last read or saved, what a save starts from.
	settings       bool
	settingsCursor int
	theme          string
	themePicked    bool
	settingsFile   config.Config
	// quitAsk shows the quit modal (see quit.go) over whatever is on
	// screen; it takes every key and every click until it is answered.
	quitAsk bool

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

	// frame counts the ticks, every redraw of the effects included (a
	// burst looks different on each); animFrame counts the animation
	// steps, taken at the animation's own pace whatever the redraws (see
	// animate), and animAt is when the latest one was due.
	frame     uint64
	animFrame uint64
	animAt    time.Time
	bars      eq
	// levels delivers the player's spectrum readings (nil when it cannot
	// measure); spectrum is the latest, taken at spectrumAt. playSince is
	// when playback started (zero while it does not play). barsDecorative
	// says the last frame's bars were decorative; barsHandover counts the
	// frames left to glide onto the readings (see stepBars).
	levels         <-chan playback.Spectrum
	spectrum       playback.Spectrum
	spectrumAt     time.Time
	playSince      time.Time
	barsDecorative bool
	barsHandover   int
	glitch         int
	// rain is the visualizer the spectrum area draws (see viz.go);
	// configSource the settings file (see vizstate.go).
	rain         rainViz
	configSource config.Source
	configSaves  *configSaves
	// updates, version and upgrade check for a newer release (see
	// release.go); newer is the one found, zero while none is known.
	updates updatecheck.Checker
	version string
	upgrade string
	newer   updatecheck.Release
	// tickGen identifies the live tick chain; ticks from older chains are
	// dropped so rescheduling never doubles the frame rate.
	tickGen  uint64
	tickFast bool
	// fx schedules the signal effects (see glitch.go); intro is the
	// latest content intro (see intro.go).
	fx    effects
	intro intro
	// swap dissolves the spectrum area between the idle emblem and the
	// rain when playback starts or stops (see vizswap.go).
	swap vizSwap
	// boot shows the boot splash (see boot.go) until bootEnd, set by the
	// first size; zero until then.
	boot    bool
	bootEnd time.Time
	// shutdown shows the shutdown splash (see shutdown.go) from the
	// confirmed quit until tea.Quit: at least until shutdownEnd, and
	// until closed, the player's close answered or given up on.
	shutdown    bool
	shutdownEnd time.Time
	closed      bool
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
	var levels <-chan playback.Spectrum
	if src, ok := p.(playback.LevelSource); ok {
		levels = src.Levels()
	}
	m := Model{
		player:       p,
		levels:       levels,
		now:          opts.Now,
		seed:         opts.Seed,
		timeout:      opts.CallTimeout,
		closeTimeout: opts.CloseTimeout,
		stack:        []frame{{kind: viewStations}},
		volumeBusy:   true, // Init reads the volume
		fx:           effects{on: opts.Effects},
		boot:         !opts.SkipBoot,
		configSource: opts.Config,
		updates:      opts.Updates,
		version:      opts.Version,
		upgrade:      opts.Upgrade,
		configSaves:  &configSaves{},

		input:         in,
		nameInput:     name,
		recentsStore:  opts.Recents,
		recentsWrites: newRecentsWriter(opts.Recents),
	}
	// A new Model starts on the default theme until the settings file
	// names another (see onConfig).
	return m.setTheme(themes[0])
}

// Init authorizes, loads recent searches and the settings, reads the volume, starts
// listening to the player, starts animating, and checks for a newer release.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.authorizeCmd(), m.loadRecentsCmd(), m.loadConfigCmd(), m.readVolumeCmd(0), m.waitStates(), m.waitErrors(), tickAfter(idleTick, tickMsg{gen: m.tickGen}), m.checkReleaseCmd())
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
	// playlist, or empty when songs were played. report lists the songs
	// the player left out of the queue.
	playMsg struct {
		seq     uint64
		op      string
		station string
		queue   []playback.Song
		report  playback.QueueReport
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
	// tickMsg is a frame of tick chain gen; redraw says the effects
	// brought it early, so it steps the animation only when due (see
	// animate).
	tickMsg struct {
		gen    uint64
		redraw bool
	}
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

// closeCmd closes the player once the quit is confirmed (see
// shutdown.go) and reports it with a closedMsg, but never waits longer
// than closeTimeout: a stuck player must not keep the UI (and the
// terminal in raw mode) from exiting, so a timeout reports closedMsg
// too. The caller may still wait for Close afterwards, once the terminal
// is restored.
func (m Model) closeCmd() tea.Cmd {
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
		return closedMsg{}
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
	return tickAfter(m.tickInterval(), m.nextTick())
}

// nextTick is the tick the chain scheduled now delivers: a redraw when
// the effects brought it before the animation's own interval (see
// animate).
func (m Model) nextTick() tickMsg {
	return tickMsg{gen: m.tickGen, redraw: m.tickInterval() < m.animInterval()}
}

// animInterval is the animation's own pace: fastTick while something
// moves, else idleTick.
func (m Model) animInterval() time.Duration {
	if m.tickFast {
		return fastTick
	}
	return idleTick
}

// tickInterval is the time to the next frame: fastTick while something
// moves, else idleTick, paced by the signal effects while they run (see
// effects.interval) and, when nothing runs faster, landing on the idle
// emblem's frames (see idleFrameWait), at most burstTick during a swap
// between the emblem and the rain (see swapInterval), at most introTick
// during an intro, and cut to the boot's or the
// shutdown's frames and its end (see splashInterval).
func (m Model) tickInterval() time.Duration {
	d := idleTick
	if m.tickFast {
		d = fastTick
	}
	if m.fxActive() {
		d = m.fx.interval(m.now(), d)
	}
	if m.idleActive() && d >= idleFrameTick {
		// Land on the idle emblem's frames; a faster pace (a burst, an
		// intro) already redraws often enough.
		d = max(idleFrameWait(m.now()), minWake)
	}
	d = m.swapInterval(d)
	if m.introAnimating() {
		end := m.intro.start.Add(introDur)
		d = min(d, max(min(introTick, end.Sub(m.now())), minWake))
	}
	return m.splashInterval(d)
}

func (m Model) animating() bool {
	return m.isPlaying() || !m.bars.flat() || m.glitch > 0 || m.introAnimating()
}

func tickAfter(d time.Duration, msg tickMsg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
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
