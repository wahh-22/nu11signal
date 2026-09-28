// Package radio is the soul-king terminal UI: a Cyberpunk 2077 style car
// radio driving a playback.Player. It depends only on the playback port.
package radio

import (
	"context"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"soulking/internal/playback"
)

// Options configures a radio Model. Zero values select sensible defaults.
type Options struct {
	// Now is the clock; defaults to time.Now. Tests inject a fixed clock.
	Now func() time.Time
	// Seed drives the decorative EQ and glitch animations.
	Seed uint64
	// CallTimeout bounds every Player call (default 8s).
	CallTimeout time.Duration
}

const (
	searchLimit     = 25
	seekStep        = 10 * time.Second
	statusTTL       = 4 * time.Second
	glitchFrames    = 6
	fastTick        = 100 * time.Millisecond
	idleTick        = time.Second
	defaultCallTime = 8 * time.Second
)

type authPhase int

const (
	authPending authPhase = iota
	authOK
	authFailed
)

type listView int

const (
	viewStations listView = iota
	viewResults
)

// Model is the Bubble Tea model of the radio.
type Model struct {
	player  playback.Player
	now     func() time.Time
	seed    uint64
	timeout time.Duration

	width, height int

	auth       authPhase
	authDetail string

	stations       []playback.Playlist
	results        []playback.Song
	resultsTerm    string
	list           listView
	stationCursor  int
	resultCursor   int
	playingStation string

	searching bool
	input     textinput.Model

	state     playback.State
	hasState  bool
	stateAt   time.Time
	lostState bool
	lostErrs  bool

	status      string
	statusUntil time.Time

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
		opts.CallTimeout = defaultCallTime
	}
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "ARTIST, TRACK, ALBUM"
	in.CharLimit = 120
	in.SetStyles(inputStyles())
	return Model{
		player:  p,
		now:     opts.Now,
		seed:    opts.Seed,
		timeout: opts.CallTimeout,
		input:   in,
	}
}

// Init authorizes, starts listening to the player, and starts animating.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.authorizeCmd(), m.waitStates(), m.waitErrors(), tickAfter(idleTick, m.tickGen))
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
	searchMsg struct {
		term  string
		songs []playback.Song
		err   error
	}
	// actionMsg reports the outcome of a fire-and-forget Player call.
	actionMsg struct {
		op  string
		err error
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

func (m Model) loadPlaylistsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		pls, err := m.player.Playlists(ctx)
		return playlistsMsg{playlists: pls, err: err}
	}
}

func (m Model) searchCmd(term string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		songs, err := m.player.Search(ctx, term, searchLimit)
		return searchMsg{term: term, songs: songs, err: err}
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

func (m Model) quitCmd() tea.Cmd {
	return func() tea.Msg {
		_ = m.player.Close()
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

func (m Model) showingResults() bool { return m.list == viewResults && len(m.results) > 0 }

func (m Model) cursor() int {
	if m.showingResults() {
		return m.resultCursor
	}
	return m.stationCursor
}

func (m *Model) setStatus(s string) {
	m.status = s
	m.statusUntil = m.now().Add(statusTTL)
}
