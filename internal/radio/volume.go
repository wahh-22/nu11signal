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
	}
	// setVolumeMsg reports a SetVolume call for level.
	setVolumeMsg struct {
		level float64
		err   error
	}
)

// readVolumeCmd reads the volume; step is applied once it answers.
func (m Model) readVolumeCmd(step float64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		level, err := m.player.Volume(ctx)
		return volumeMsg{level: level, step: step, err: err}
	}
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
	level := m.volume
	return m, func() tea.Msg {
		ctx, cancel := m.ctx()
		defer cancel()
		return setVolumeMsg{level: level, err: m.player.SetVolume(ctx, level)}
	}
}

// onVolume settles a read. The startup read fails quietly (the readout
// says VOL --); a read a press asked for reports its failure.
func (m Model) onVolume(msg volumeMsg) (Model, tea.Cmd) {
	m.volumeBusy = false
	if msg.err != nil {
		if msg.step != 0 {
			m.setStatus("VOLUME FAILED // " + msg.err.Error())
		}
		return m, nil
	}
	// The player may report a level out of range; the UI shows it clamped.
	m.volume, m.volumeKnown = playback.ClampVolume(msg.level), true
	if msg.step != 0 {
		return m.stepVolume(msg.step)
	}
	return m, nil
}

// onSetVolume settles a change, sending the latest level if presses moved
// it meanwhile.
func (m Model) onSetVolume(msg setVolumeMsg) (Model, tea.Cmd) {
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

// volumeMeterMax is the widest meter of the volume readout.
const volumeMeterMax = 10

// volumeTextWidth is the room the readout takes besides its meter.
var volumeTextWidth = ansi.StringWidth("VOL  100%")

// volumeBar lays out VOL- and VOL+ around the volume readout in at most w
// cells, with its zones. The readout keeps one width whatever the level,
// so the buttons never move; its meter shrinks to fit, and below the room
// for the buttons only the readout is left:
//
//	╱ - ╱ VOL ▮▮▮▮▮▮▯▯▯▯  60% ╱ + ╱
func (m Model) volumeBar(w int) (string, zones) {
	down := button{id: zoneVolDown, label: "-", tone: stCyan, focused: m.focused(ctlVolDown)}
	up := button{id: zoneVolUp, label: "+", tone: stCyan, focused: m.focused(ctlVolUp)}
	room := min(w-down.width()-up.width()-2, volumeTextWidth+volumeMeterMax)
	if room < volumeTextWidth {
		return m.volumeReadout(w), nil
	}
	var zs zones
	zs.add(down.id, 0, 0, down.width())
	zs.add(up.id, down.width()+1+room+1, 0, up.width())
	return down.render() + " " + m.volumeReadout(room) + " " + up.render(), zs
}

// volumeReadout renders "VOL ▮▮▮▯▯  60%" in exactly w cells, the meter as
// wide as fits (up to volumeMeterMax), or "VOL --" while the level is
// unknown.
func (m Model) volumeReadout(w int) string {
	label := stMuted.Render("VOL ")
	if !m.volumeKnown {
		return fit(label+stDim.Render("--"), w)
	}
	pct := fmt.Sprintf("%3d%%", int(math.Round(m.volume*100)))
	meter := min(w-volumeTextWidth, volumeMeterMax)
	if meter < 3 {
		return fit(label+stRed.Render(strings.TrimSpace(pct)), w)
	}
	filled := int(math.Round(m.volume * float64(meter)))
	return fit(label+stCyan.Render(strings.Repeat("▮", filled))+stDim.Render(strings.Repeat("▯", meter-filled))+
		" "+stRed.Render(pct), w)
}
