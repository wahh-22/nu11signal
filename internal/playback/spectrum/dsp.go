package spectrum

import (
	"math"
	"math/cmplx"
)

// binRange is the FFT bins [lo, hi) a band sums.
type binRange struct{ lo, hi int }

// bandRanges ports the helper's SpectrumBands.binRanges: each band sums the
// bins whose center falls inside it; a band narrower than a bin reads the
// bin nearest its geometric center. DC and Nyquist are never used.
func bandRanges(rate float64, size, bands int, lo, hi float64) []binRange {
	binHz := rate / float64(size)
	last := size/2 - 1
	edge := func(i int) float64 { return lo * math.Pow(hi/lo, float64(i)/float64(bands)) }
	out := make([]binRange, bands)
	for b := range out {
		l := max(int(math.Ceil(edge(b)/binHz)), 1)
		u := min(int(math.Ceil(edge(b+1)/binHz)), last+1)
		if l < u {
			out[b] = binRange{l, u}
			continue
		}
		c := min(max(int(math.Round(math.Sqrt(edge(b)*edge(b+1))/binHz)), 1), last)
		out[b] = binRange{c, c + 1}
	}
	return out
}

// bandOf is the band a frequency falls in; -1 outside lo..<hi.
func bandOf(hz float64, bands int, lo, hi float64) int {
	if hz < lo || hz >= hi {
		return -1
	}
	return min(int(math.Log(hz/lo)/math.Log(hi/lo)*float64(bands)), bands-1)
}

// normalizedLevel maps a band's power (a full-scale sine is 1) to 0...1,
// linear in decibels between FloorDB and CeilingDB.
func normalizedLevel(power float64) float64 {
	if !(power > 0) || math.IsInf(power, 1) {
		return 0
	}
	db := 10 * math.Log10(power)
	return min(max((db-FloorDB)/(CeilingDB-FloorDB), 0), 1)
}

// fft is an in-place iterative radix-2 FFT; len(x) must be a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			t := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], x[start+k+size/2]*t
				x[start+k], x[start+k+size/2] = a+b, a-b
				t *= w
			}
		}
	}
}

// analyzer is the helper's SpectrumAnalyzer: a Hann-windowed FFT of
// FFTSize mono samples summed into bands. It keeps its buffers; use it
// from one goroutine at a time.
type analyzer struct {
	ranges []binRange
	window []float64
	buf    []complex128
}

func newAnalyzer(rate int) *analyzer {
	a := &analyzer{ranges: bandRanges(float64(rate), FFTSize, BandCount, MinHz, MaxHz),
		window: make([]float64, FFTSize), buf: make([]complex128, FFTSize)}
	for i := range a.window {
		a.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/FFTSize) // periodic Hann
	}
	return a
}

// levels are the band levels of FFTSize samples.
func (a *analyzer) levels(samples []float32) []float64 {
	for i := range a.buf {
		a.buf[i] = complex(float64(samples[i])*a.window[i], 0)
	}
	fft(a.buf)
	// The Hann window halves a tone, so a sine of amplitude A peaks at
	// A * FFTSize / 4: dividing the squared magnitudes by (FFTSize/4)^2
	// makes a full-scale sine 1, as in the helper.
	scale := 1 / (FFTSize / 4.0 * FFTSize / 4.0)
	out := make([]float64, len(a.ranges))
	for b, r := range a.ranges {
		var sum float64
		for _, c := range a.buf[r.lo:r.hi] {
			sum += real(c)*real(c) + imag(c)*imag(c)
		}
		out[b] = normalizedLevel(sum * scale)
	}
	return out
}

// smoother is the helper's LevelSmoother: fast attack, slow release (a
// bar falls back over about a second at 15 readings a second).
type smoother struct{ levels []float64 }

const (
	attack  = 0.7
	release = 0.2
)

func newSmoother(n int) *smoother { return &smoother{levels: make([]float64, n)} }

// update moves every level toward its reading and returns a copy; levels
// under half a percent snap to 0.
func (s *smoother) update(readings []float64) []float64 {
	for i := range s.levels {
		var v float64
		if i < len(readings) {
			v = readings[i]
		}
		rate := release
		if v > s.levels[i] {
			rate = attack
		}
		s.levels[i] += (v - s.levels[i]) * rate
		if s.levels[i] < 0.005 {
			s.levels[i] = 0
		}
	}
	return append([]float64(nil), s.levels...)
}

func (s *smoother) reset() { clear(s.levels) }

// decimate is the helper's Waveform.decimated: points consecutive buckets,
// each keeping its sample farthest from zero (clipped to -1...1, NaN as 0)
// so a transient survives. Fewer samples than points is all zeros.
func decimate(samples []float32, points int) []float64 {
	out := make([]float64, points)
	if len(samples) < points {
		return out
	}
	for p := range out {
		var pk float64
		for _, v := range samples[p*len(samples)/points : (p+1)*len(samples)/points] {
			s := float64(v)
			if math.IsNaN(s) {
				s = 0
			}
			s = min(max(s, -1), 1)
			if math.Abs(s) > math.Abs(pk) {
				pk = s
			}
		}
		out[p] = pk
	}
	return out
}

// quantize rounds to hundredths, the resolution the helper's levels
// event carries, so backends hand the UI the same values.
func quantize(v []float64) []float64 {
	for i := range v {
		v[i] = math.Round(v[i]*100) / 100
	}
	return v
}

// ring keeps the newest mono samples (the stereo mean) for the analyzer.
// It is not synchronized: the meter guards it with its lock.
type ring struct {
	buf     []float32
	written int
}

func newRing(capacity int) *ring { return &ring{buf: make([]float32, capacity)} }

func (r *ring) writeStereo(s []float32) {
	for i := 0; i+1 < len(s); i += 2 {
		r.buf[r.written%len(r.buf)] = (s[i] + s[i+1]) / 2
		r.written++
	}
}

func (r *ring) writeMono(s []float32) {
	for _, v := range s {
		r.buf[r.written%len(r.buf)] = v
		r.written++
	}
}

// latest copies the newest n samples, oldest first; false when fewer were
// written since the last reset.
func (r *ring) latest(n int) ([]float32, bool) {
	if n > len(r.buf) || r.written < n {
		return nil, false
	}
	out := make([]float32, n)
	for i := range out {
		out[i] = r.buf[(r.written-n+i)%len(r.buf)]
	}
	return out, true
}

func (r *ring) reset() { r.written = 0 }
