package radio

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// The LOOP control cycles the player's repeat mode: OFF, ALL (the queue
// starts over), ONE (the song starts over); its button, on the transport
// row, names the mode shown: [↻ OFF], [↻ ALL], [↻ ONE]. The change is
// optimistic: the button shows the mode asked for at once, and keeps
// showing it until a state reports it (states sent before the change
// still carry the old one), the player refuses it or loopHold passes;
// then the player's reports rule again.

// loopHold is how long the mode asked for is shown without a state
// reporting it: then the player's reports rule again, so a change the
// player accepted but did not take cannot stick on the button.
const loopHold = 3 * time.Second

// loopMsg reports SetRepeat number seq, for mode.
type loopMsg struct {
	seq  uint64
	mode playback.RepeatMode
	err  error
}

// nextLoop is the mode after mode in the LOOP cycle.
func nextLoop(mode playback.RepeatMode) playback.RepeatMode {
	switch mode {
	case playback.RepeatAll:
		return playback.RepeatOne
	case playback.RepeatOne:
		return playback.RepeatOff
	}
	return playback.RepeatAll
}

// loopMode is the repeat mode shown: the one asked for until the player
// confirms or refuses it or loopHold passes, else the player's.
func (m Model) loopMode() playback.RepeatMode {
	if m.loopPending && m.now().Before(m.loopUntil) {
		return m.loopWant
	}
	if m.state.Repeat == "" {
		return playback.RepeatOff
	}
	return m.state.Repeat
}

// cycleLoop asks the player for the next mode of the cycle, showing it at
// once. Presses meanwhile each send theirs: the helper runs them in order.
func (m Model) cycleLoop() (Model, tea.Cmd) {
	mode := nextLoop(m.loopMode())
	m.loopSeq++
	m.loopWant, m.loopPending, m.loopUntil = mode, true, m.now().Add(loopHold)
	seq, player := m.loopSeq, m.player
	return m, func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		return loopMsg{seq: seq, mode: mode, err: player.SetRepeat(ctx, mode)}
	}
}

// onLoop settles a change: a refusal is reported and, for the latest
// change, gives the button back to the player's mode. A change accepted
// stays shown until a state reports it (see onState).
func (m Model) onLoop(msg loopMsg) Model {
	if msg.err == nil {
		return m
	}
	m.setStatus("LOOP FAILED // " + msg.err.Error())
	if msg.seq == m.loopSeq {
		m.loopPending = false
	}
	return m
}

// confirmLoop ends the optimistic mode once the player reports it, or
// once it expired.
func (m *Model) confirmLoop() {
	if m.loopPending && (m.state.Repeat == m.loopWant || !m.now().Before(m.loopUntil)) {
		m.loopPending = false
	}
}

// loopName is the LOOP button's name for mode: OFF, ALL or ONE.
func loopName(mode playback.RepeatMode) string {
	switch mode {
	case playback.RepeatAll:
		return "ALL"
	case playback.RepeatOne:
		return "ONE"
	}
	return "OFF"
}
