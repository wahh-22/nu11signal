package radio

import (
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// The output volume is read once at startup and shown in NOW PLAYING.
// Changes are optimistic: the readout moves at once and the player is
// asked for the new level. Only one call is in flight at a time; presses
// meanwhile only move the level, and the latest one is sent when the call
// answers, so holding a key never floods the helper. A refused change (an
// output device without a settable volume) is reported on the status line
// and leaves the level unknown (VOL --) until the next press reads it
// again.
//
// The player says, in its states, which volume it drives: its own (app)
// or, as a fallback, the system's; the readout's label says which (VOL or
// SYS). When the mode changes (the app volume granted mid-session, or
// failed), the level is another one: it is unknown until read again, and
// the presses that moved the old one are dropped. A call in flight across
// the change answers for the old mode (each call carries the mode epoch it
// was made in), so its answer is discarded and the level read again.
// Losing the app volume is reported on the status line.

// volumeStep is how much one press moves the volume.
const volumeStep = 0.05

// Answers to the volume calls.
type (
	// volumeMsg reports a Volume read; step is the change to apply once
	// the level is known (a press that found it unknown), 0 at startup.
	volumeMsg struct {
		level float64
		step  float64
		err   error
		epoch int
	}
	// setVolumeMsg reports a SetVolume call for level.
	setVolumeMsg struct {
		level float64
		err   error
		epoch int
	}
)

// readVolumeCmd reads the volume; step is applied once it answers.
func (m Model) readVolumeCmd(step float64) tea.Cmd {
	epoch := m.volumeEpoch
	return func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		level, err := m.player.Volume(ctx)
		return volumeMsg{level: level, step: step, err: err, epoch: epoch}
	}
}

// followVolumeMode records the volume mode a state reports. When it
// changed from a mode already known, the level shown is the old mode's:
// it becomes unknown (dropping presses not yet sent) and is read again,
// now or, with a call in flight, once that call's stale answer arrives.
func (m Model) followVolumeMode(mode playback.VolumeMode) (Model, tea.Cmd) {
	if mode == "" || mode == m.volumeMode {
		return m, nil
	}
	previous := m.volumeMode
	m.volumeMode = mode
	if previous == "" {
		return m, nil // the first mode names the level already read
	}
	if previous == playback.VolumeApp && mode == playback.VolumeSystem {
		m.setStatus("APP VOLUME OFF // NOW THE SYSTEM VOLUME")
	}
	m.volumeEpoch++
	m.volumeKnown, m.volumePending = false, 0
	if m.volumeBusy {
		return m, nil
	}
	m.volumeBusy = true
	return m, m.readVolumeCmd(0)
}

// staleVolume settles a call made before the volume mode changed: its
// answer is the old mode's, so the new level is read, keeping only the
// steps pressed since the change.
func (m Model) staleVolume() (Model, tea.Cmd) {
	m.volumeKnown = false
	m.volumeBusy = true
	return m, m.readVolumeCmd(0)
}

// roundVolume clamps level and rounds it to whole percents, so that steps
// do not accumulate float error.
func roundVolume(level float64) float64 {
	return playback.ClampVolume(math.Round(level*100) / 100)
}

// stepVolume moves the volume by delta: at once while a call is in flight
// (it is sent when that call answers), else asking the player now. An
// unknown level is read first, then stepped.
func (m Model) stepVolume(delta float64) (Model, tea.Cmd) {
	if !m.volumeKnown {
		if m.volumeBusy {
			// A read is in flight (the startup one included): keep the
			// step for when it answers.
			m.volumePending += delta
			return m, nil
		}
		m.volumeBusy = true
		return m, m.readVolumeCmd(delta)
	}
	m.volume = roundVolume(m.volume + delta)
	if m.volumeBusy {
		return m, nil
	}
	return m.sendVolume()
}

// sendVolume asks the player for the level shown.
func (m Model) sendVolume() (Model, tea.Cmd) {
	m.volumeBusy = true
	level, epoch := m.volume, m.volumeEpoch
	return m, func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		return setVolumeMsg{level: level, err: m.player.SetVolume(ctx, level), epoch: epoch}
	}
}

// onVolume settles a read. The startup read fails quietly (the readout
// says VOL --); a read a press asked for reports its failure.
func (m Model) onVolume(msg volumeMsg) (Model, tea.Cmd) {
	if msg.epoch != m.volumeEpoch {
		return m.staleVolume()
	}
	step := msg.step + m.volumePending
	m.volumeBusy, m.volumePending = false, 0
	if msg.err != nil {
		if step != 0 {
			m.setStatus("VOLUME FAILED // " + msg.err.Error())
		}
		return m, nil
	}
	// The player may report a level out of range; the UI shows it clamped.
	m.volume, m.volumeKnown = playback.ClampVolume(msg.level), true
	if step != 0 {
		return m.stepVolume(step)
	}
	return m, nil
}

// onSetVolume settles a change, sending the latest level if presses moved
// it meanwhile.
func (m Model) onSetVolume(msg setVolumeMsg) (Model, tea.Cmd) {
	if msg.epoch != m.volumeEpoch {
		return m.staleVolume()
	}
	m.volumeBusy = false
	if msg.err != nil {
		m.volumeKnown = false
		m.setStatus("VOLUME FAILED // " + msg.err.Error())
		return m, nil
	}
	if m.volumeKnown && m.volume != msg.level {
		return m.sendVolume()
	}
	return m, nil
}

// volumeMeterMax is the widest meter of the volume row, and
// volumeMeterMin the narrowest it keeps.
const (
	volumeMeterMax = 20
	volumeMeterMin = 3
)

// volumePctWidth is the room of the percentage ending the volume row,
// "100%" at the widest.
const volumePctWidth = 4

// volumeBar lays out the volume row in at most w cells, with its zones:
// the label (SYS instead of VOL when the level is the system volume),
// [−], the meter as wide as fits (up to volumeMeterMax), [+] and the
// percentage. The row keeps its layout whatever the level, so the buttons
// never move; while the level is unknown the meter's room reads --.
// Without room for volumeMeterMin cells the meter is left out, and without
// room for the buttons only the readout is left:
//
//	VOL [−] ▮▮▮▮▮▮▮▮▮▮▮▮▯▯▯▯▯▯▯▯ [+] 60%
func (m Model) volumeBar(w int) (string, zones) {
	label := "VOL "
	if m.volumeMode == playback.VolumeSystem {
		label = "SYS "
	}
	pct := fmt.Sprintf("%d%%", int(math.Round(m.volume*100)))
	down := button{id: zoneVolDown, label: "−", tone: stCyan, bracket: true, focused: m.focused(ctlVolDown)}
	up := button{id: zoneVolUp, label: "+", tone: stCyan, bracket: true, focused: m.focused(ctlVolUp)}
	lead := len(label) + down.width() + 1
	bare := lead + up.width() + 1 + volumePctWidth
	if w < bare {
		if !m.volumeKnown {
			return fit(stMuted.Render(label)+stDim.Render("--"), w), nil
		}
		return fit(stMuted.Render(label)+stRed.Render(pct), w), nil
	}
	meter := min(w-bare-1, volumeMeterMax)
	var gauge, tail string
	switch {
	case meter < volumeMeterMin && !m.volumeKnown:
		meter, tail = 0, stDim.Render("--")
	case meter < volumeMeterMin:
		meter, tail = 0, stRed.Render(pct)
	case !m.volumeKnown:
		gauge = fit(stDim.Render("--"), meter) + " "
	default:
		filled := int(math.Round(m.volume * float64(meter)))
		gauge = stCyan.Render(strings.Repeat("▮", filled)) + stDim.Render(strings.Repeat("▯", meter-filled)) + " "
		tail = stRed.Render(pct)
	}
	upX := lead + ansi.StringWidth(gauge)
	var zs zones
	zs.add(down.id, len(label), 0, down.width())
	zs.add(up.id, upX, 0, up.width())
	return stMuted.Render(label) + down.render() + " " + gauge + up.render() + " " + fit(tail, volumePctWidth), zs
}
