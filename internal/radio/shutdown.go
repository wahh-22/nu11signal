package radio

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// The shutdown splash is the boot splash run backwards: once the quit
// modal is confirmed (y, enter, q or ctrl+c again, or its QUIT button,
// see quit.go), the modal closes and the body shows the null emblem
// again (see splash), with the SHUTTING DOWN line (shutdownText) in the
// BOOTING line's place, while the player closes (see closeCmd: the
// helper fades the music out and pauses before it tears down). The
// header stays; the footer goes blank, since no key but the escape hatch
// acts any more. Over the auth error screen the splash takes its place
// too; the tiny layout, with no room for it, shows the line alone.
//
// nu11signal quits (tea.Quit) once both the close has answered, or given
// up after closeTimeout (a closedMsg either way), and shutdownMin has
// passed on the injected clock since the shutdown began, whichever comes
// last: a quick close still shows the splash for a moment, a slow one
// keeps it until it is done.
//
// With the signal effects on the splash glitches like the boot's, a
// burst from its first frame to its last (see splashGlitch), at
// burstTick; the periodic bursts stay off (see fxActive). Calm, it shows
// still, and a single tick lands on shutdownMin. No timer is added:
// tickInterval cuts the tick chain to the shutdown's frames and its end
// (see splashInterval), and stops cutting once that end has passed.
//
// While it shuts down every key and click is ignored but ctrl+c, the
// escape hatch, which quits at once: the caller's own Close still waits
// for the player once the terminal is restored.
const (
	shutdownMin  = time.Second
	shutdownText = "SHUTTING DOWN..."
)

// saltShutdown keeps the shutdown glitch's stream apart from the boot's.
const saltShutdown uint64 = 502

// closedMsg reports that the player closed, or that closeCmd stopped
// waiting for it after closeTimeout.
type closedMsg struct{}

// startShutdown confirms the quit: the modal closes, the shutdown splash
// shows from now for at least shutdownMin, and the player starts
// closing. Update starts the tick chain for its frames (see Update).
func (m Model) startShutdown() (Model, tea.Cmd) {
	m.quitAsk, m.boot = false, false
	m.shutdown, m.closed = true, false
	m.shutdownEnd = m.now().Add(shutdownMin)
	return m, m.closeCmd()
}

// shutdownDone reports whether the shutdown may quit: the player closed
// (or given up on) and the splash shown for shutdownMin.
func (m Model) shutdownDone() bool {
	return m.shutdown && m.closed && !m.now().Before(m.shutdownEnd)
}

// onClosed takes the close's answer: the shutdown quits now if the
// splash has shown long enough, else on the tick that ends it.
func (m Model) onClosed() (Model, tea.Cmd) {
	m.closed = true
	if m.shutdownDone() {
		return m, tea.Quit
	}
	return m, nil
}

// onShutdownTick steps a frame of the shutdown, for its glitch, and quits
// once it is done. Nothing else runs: the player is closing, so the
// favorite reads and the playback bookkeeping of onTick are left out.
func (m Model) onShutdownTick() (Model, tea.Cmd) {
	m.frame++
	if m.shutdownDone() {
		return m, tea.Quit
	}
	return m, m.scheduleTick()
}

// shutdownKey handles a key press during the shutdown: ctrl+c quits at
// once, every other key is ignored.
func (m Model) shutdownKey(k string) (Model, tea.Cmd) {
	if k == keyCtrlC {
		return m, tea.Quit
	}
	return m, nil
}
