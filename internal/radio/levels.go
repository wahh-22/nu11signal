package radio

import (
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// levelsFresh is how long a spectrum reading drives the bars: the helper
// sends about 15 a second while it measures, so an older one means it
// stopped (paused, or the app volume fell back to the system volume).
const levelsFresh = 500 * time.Millisecond

// In app volume mode the helper measures what plays, but only once its
// tap has attached, a moment after the song starts. Until then the bars
// wait, held where they are, for up to eqWaitLevels: animating them
// decoratively meanwhile would show bars, then none as the first quiet
// readings arrive, then the real ones. Without a reading by then they
// turn decorative. Moving from decorative bars to readings takes
// eqHandover frames, so the bars glide instead of jumping.
const (
	eqWaitLevels = 1500 * time.Millisecond
	eqHandover   = 4
)

// trackPlay notes when playback starts (playSince, zero while it does
// not play). A reading from before it (the last one sent before a pause,
// still in the channel or already taken) is dropped: it is not what
// plays now.
func (m Model) trackPlay() Model {
	if !m.isPlaying() {
		m.playSince = time.Time{}
		return m
	}
	if !m.playSince.IsZero() {
		return m
	}
	m.playSince, m.spectrum = m.now(), nil
	// The channel keeps the latest reading only: one read drains it.
	select {
	case _, ok := <-m.levels:
		if !ok {
			m.levels = nil
		}
	default:
	}
	return m
}

// pollLevels takes the player's latest spectrum reading, if a new one
// arrived. It runs on the animation tick and never blocks, so readings
// add no frames or messages of their own: the bars show the newest one
// each frame. A closed channel (the player shut down) is dropped, and an
// empty reading is no reading.
func (m Model) pollLevels() Model {
	if m.levels == nil {
		return m
	}
	select {
	case levels, ok := <-m.levels:
		if !ok {
			m.levels = nil
			return m
		}
		if len(levels) > 0 {
			m.spectrum, m.spectrumAt = levels, m.now()
		}
	default:
	}
	return m
}

// liveSpectrum is the reading the bars draw this frame: the latest one
// while playing, fresh and taken since playback started. Otherwise
// (paused, no reading, a player that cannot measure) the bars wait for
// one (see waitingForLevels), animate decoratively, or fall when paused.
func (m Model) liveSpectrum() ([]float64, bool) {
	if !m.isPlaying() || len(m.spectrum) == 0 || m.now().Sub(m.spectrumAt) >= levelsFresh ||
		m.spectrumAt.Before(m.playSince) {
		return nil, false
	}
	return m.spectrum, true
}

// waitingForLevels reports whether the bars hold still for a first
// reading: playing in app volume mode with a player that measures, no
// reading yet since playback started (trackPlay clears the last one),
// for eqWaitLevels.
func (m Model) waitingForLevels() bool {
	return m.levels != nil && m.volumeMode == playback.VolumeApp && m.isPlaying() &&
		len(m.spectrum) == 0 && m.now().Sub(m.playSince) < eqWaitLevels
}

// stepBars moves the bars one frame: onto the live reading (gliding over
// eqHandover frames when they were decorative), held while waiting for a
// first reading, else decorative while playing or falling when not.
func (m Model) stepBars() Model {
	if levels, ok := m.liveSpectrum(); ok {
		target := m.bars.follow(levels, m.eqBarCount())
		if m.barsDecorative {
			m.barsDecorative, m.barsHandover = false, eqHandover
		}
		if m.barsHandover > 0 {
			// 1/4, 1/3, 1/2, then all of what is left: on the reading.
			m.bars = m.bars.ease(target, 1/float64(m.barsHandover))
			m.barsHandover--
			return m
		}
		m.bars = target
		return m
	}
	m.barsHandover = 0
	if m.waitingForLevels() {
		return m
	}
	playing := m.isPlaying()
	m.bars = m.bars.step(playing, m.seed, m.frame)
	m.barsDecorative = playing
	return m
}

// resampleLevels maps the bands of a reading onto n bars: each bar is the
// mean of the bands it spans, weighted by how much of each it covers, so
// fewer bars average neighbouring bands and more bars repeat them.
func resampleLevels(bands []float64, n int) []float64 {
	out := make([]float64, max(n, 0))
	if len(bands) == 0 || n <= 0 {
		return out
	}
	// In units of 1/n band, bar j spans [j*len, (j+1)*len) and band i
	// spans [i*n, (i+1)*n): integer overlaps keep equal bands exact.
	b := len(bands)
	for j := range out {
		lo, hi := j*b, (j+1)*b
		var sum float64
		for i := lo / n; i < b && i*n < hi; i++ {
			sum += bands[i] * float64(min(hi, (i+1)*n)-max(lo, i*n))
		}
		out[j] = sum / float64(b)
	}
	return out
}
