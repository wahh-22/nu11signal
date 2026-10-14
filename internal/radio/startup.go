package radio

import tea "charm.land/bubbletea/v2"

// Startup is the boot splash a frontend shows while it opens its player,
// before any Model can run: the same header, emblem, BOOTING line and
// glitch as the Model's own boot (see boot.go), but held for the whole
// wait instead of bootDur. When the frontend sends the Model built on its
// player, the program runs that Model from then on: at the size already
// known, started, its own boot carrying the same splash on before the
// normal UI scrambles in. Closing the channel instead (the player failed
// to open) quits the program.
//
// The splash is drawn by a Model that never runs Init and has no player:
// Startup drives only its size and its frames, never the player-bound
// work of an ordinary tick, and drops keys and clicks, as there is no
// player yet to act on or to close.
type Startup struct {
	m    Model
	next <-chan Model
}

// startedMsg carries the frontend's Model; ok is false once the channel
// closed without one.
type startedMsg struct {
	m  Model
	ok bool
}

// NewStartup returns the startup splash, from opts (the boot is always
// shown; OnTypingChanged is not reported, nothing can be typed), handing
// over to the Model next delivers.
func NewStartup(opts Options, next <-chan Model) Startup {
	opts.SkipBoot = false
	opts.OnTypingChanged = nil
	return Startup{m: New(nil, opts), next: next}
}

// Init waits for the frontend's Model.
func (s Startup) Init() tea.Cmd {
	next := s.next
	return func() tea.Msg {
		m, ok := <-next
		return startedMsg{m: m, ok: ok}
	}
}

// Update sizes and animates the splash until the handover.
func (s Startup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case startedMsg:
		if !msg.ok {
			return s, tea.Quit
		}
		return s.handOver(msg.m)
	case tea.WindowSizeMsg:
		next, cmd := s.m.Update(msg)
		if m, ok := next.(Model); ok {
			s.m = m
		}
		return s, cmd
	case tickMsg:
		if msg.gen != s.m.tickGen {
			return s, nil // superseded chain
		}
		s.m.frame++
		// Hold the boot: its end stays ahead, so the splash never gives
		// way to the normal UI before there is a player behind it.
		s.m.bootEnd = s.m.now().Add(bootDur)
		s.m.fx = s.m.fx.advance(s.m.now(), s.m.seed, s.m.fxActive())
		return s, s.m.scheduleTick()
	}
	return s, nil
}

// handOver starts m at the splash's size: its boot begins there, so the
// splash runs on unbroken into the normal UI.
func (s Startup) handOver(m Model) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.Init()}
	if s.m.width > 0 && s.m.height > 0 {
		next, cmd := m.Update(tea.WindowSizeMsg{Width: s.m.width, Height: s.m.height})
		if nm, ok := next.(Model); ok {
			m = nm
		}
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// View draws the splash.
func (s Startup) View() tea.View { return s.m.View() }
