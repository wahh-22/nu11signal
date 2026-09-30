package radio

import "time"

// levelsFresh is how long a spectrum reading drives the bars: the helper
// sends about 15 a second while it measures, so an older one means it
// stopped (paused, or the app volume fell back to the system volume).
const levelsFresh = 500 * time.Millisecond

// pollLevels takes the player's latest spectrum reading, if a new one
// arrived. It runs on the animation tick and never blocks, so readings
// add no frames or messages of their own: the bars show the newest one
// each frame. A closed channel (the player shut down) is dropped.
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
		m.spectrum, m.spectrumAt = levels, m.now()
	default:
	}
	return m
}

// liveSpectrum is the reading the bars draw this frame: the latest one
// while playing and fresh. Otherwise (paused, no reading, a player that
// cannot measure) the bars animate decoratively, or fall when paused.
func (m Model) liveSpectrum() ([]float64, bool) {
	if !m.isPlaying() || m.spectrum == nil || m.now().Sub(m.spectrumAt) >= levelsFresh {
		return nil, false
	}
	return m.spectrum, true
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
