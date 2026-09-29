package radio

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// The LOOP control cycles the player's repeat mode: OFF, ALL (the queue
// starts over), ONE (the song starts over). The change is optimistic: the
// button shows the mode asked for at once, and keeps showing it until a
// state reports it (states sent before the change still carry the old
// one) or the player refuses it; then the player's reports rule again.

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
// confirms or refuses it, else the player's.
func (m Model) loopMode() playback.RepeatMode {
	if m.loopPending {
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
	m.loopWant, m.loopPending = mode, true
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

// confirmLoop ends the optimistic mode once the player reports it.
func (m *Model) confirmLoop() {
	if m.loopPending && m.state.Repeat == m.loopWant {
		m.loopPending = false
	}
}

// loopLabels are the LOOP button's labels for mode, widest first.
func loopLabels(mode playback.RepeatMode) []string {
	name := map[playback.RepeatMode]string{playback.RepeatAll: "ALL", playback.RepeatOne: "ONE"}[mode]
	if name == "" {
		name = "OFF"
	}
	return []string{"↻ LOOP " + name, "↻ " + name, "↻"}
}

// loopButton is the widest LOOP button that fits in w cells: lit while a
// mode other than OFF is on; ok is false when none fits.
func (m Model) loopButton(w int) (b button, ok bool) {
	mode := m.loopMode()
	for _, label := range loopLabels(mode) {
		b = button{id: zoneLoop, label: label, tone: stCyan, active: mode != playback.RepeatOff, focused: m.focused(ctlLoop)}
		if b.width() <= w {
			return b, true
		}
	}
	return button{}, false
}

// loopBar lays out the LOOP button in at most w cells, with its zone;
// empty when it does not fit.
func (m Model) loopBar(w int) (string, zones) {
	b, ok := m.loopButton(w)
	if !ok {
		return "", nil
	}
	var zs zones
	zs.add(b.id, 0, 0, b.width())
	return b.render(), zs
}

// loopTail ends line, w cells, with the LOOP button, as the compact layout
// draws it at the end of the artist line, while there is room for it and
// heartTitleMinRoom cells of the line; the zones are in the line's
// coordinates.
func (m Model) loopTail(line string, w int) (string, zones) {
	b, ok := m.loopButton(w - heartTitleMinRoom - 1)
	if !ok {
		return line, nil
	}
	room := w - b.width() - 1
	line = ansi.Truncate(line, room, "…")
	var zs zones
	zs.add(b.id, room+1, 0, b.width())
	return line + strings.Repeat(" ", room-ansi.StringWidth(line)+1) + b.render(), zs
}
