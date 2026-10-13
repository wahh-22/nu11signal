// Package spectrum turns PCM into the spectrum and waveform described in
// docs/audio.md, section Spectrum. It does not own a timer or audio device.
package spectrum

import (
	"sync"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

const (
	// BandCount log-spaced bands cover MinHz through MaxHz.
	BandCount = 24
	MinHz     = 40.0
	MaxHz     = 16000.0
	// FFTSize is the newest mono sample window, with a periodic Hann window.
	FFTSize = 2048
	// FloorDB and CeilingDB map band power to empty and full bars in dBFS.
	FloorDB   = -50.0
	CeilingDB = -10.0
	// WavePoints consecutive buckets each retain their largest signed swing.
	WavePoints = 64
	// ReadingRate and ReadingInterval are the recommended cadence. Smoothing
	// uses a 0.7 attack and 0.2 release per reading, not elapsed time.
	ReadingRate     = 15
	ReadingInterval = time.Second / ReadingRate
)

// Meter retains PCM and smooths readings. Use NewMeter; methods are safe for
// concurrent use. Call Reading at ReadingInterval, only while playing, and
// Reset when playback starts or resumes. PCM should be tapped before volume.
// Bands (0...1) and Wave (-1...1) are rounded to hundredths.
type Meter struct {
	mu    sync.Mutex
	ring  *ring
	fresh bool

	measureMu sync.Mutex
	an        *analyzer
	sm        *smoother
}

// NewMeter makes a meter for PCM at sampleRate Hz, which must be positive.
func NewMeter(sampleRate int) *Meter {
	if sampleRate <= 0 {
		panic("spectrum: sample rate must be positive")
	}
	return &Meter{ring: newRing(2 * FFTSize), an: newAnalyzer(sampleRate), sm: newSmoother(BandCount)}
}

// WriteStereo retains interleaved left/right samples mixed to their mean.
// An incomplete trailing frame is ignored. The input is not retained.
func (m *Meter) WriteStereo(samples []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ring.writeStereo(samples)
}

// WriteMono retains mono samples. The input is not retained.
func (m *Meter) WriteMono(samples []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ring.writeMono(samples)
}

// Reset discards PCM and resets smoothing on the next complete snapshot.
func (m *Meter) Reset() {
	m.measureMu.Lock()
	defer m.measureMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ring.reset()
	m.fresh = true
}

// Snapshot is an immutable window for Analyze. Its zero value is invalid.
type Snapshot struct {
	samples []float32
	fresh   bool
}

// Snapshot copies the newest FFTSize samples; false until a full window has
// arrived since Reset. It allows a backend to capture under its playback lock
// and Analyze outside that lock. Analyze successful snapshots once, in order.
func (m *Meter) Snapshot() (Snapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	samples, ok := m.ring.latest(FFTSize)
	if !ok {
		return Snapshot{}, false
	}
	s := Snapshot{samples: samples, fresh: m.fresh}
	m.fresh = false
	return s, true
}

// Analyze computes and smooths a successful Snapshot from this meter.
func (m *Meter) Analyze(s Snapshot) playback.Spectrum {
	m.measureMu.Lock()
	defer m.measureMu.Unlock()
	return m.analyzeLocked(s)
}

func (m *Meter) analyzeLocked(s Snapshot) playback.Spectrum {
	if s.fresh {
		m.sm.reset()
	}
	bands := m.sm.update(m.an.levels(s.samples))
	return playback.Spectrum{Bands: quantize(bands), Wave: quantize(decimate(s.samples, WavePoints))}
}

// Reading snapshots and analyzes the newest window. Repeated calls without
// new PCM still advance smoothing, as in the local player. Backends that
// need playback-lock coordination can use Snapshot and Analyze separately.
func (m *Meter) Reading() (playback.Spectrum, bool) {
	m.measureMu.Lock()
	defer m.measureMu.Unlock()
	s, ok := m.Snapshot()
	if !ok {
		return playback.Spectrum{}, false
	}
	return m.analyzeLocked(s), true
}
