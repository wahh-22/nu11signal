package local

import (
	"errors"
	"io"
	"math"
	"math/cmplx"
)

// resampler converts a decoder's stereo frames to the output rate by
// linear interpolation between neighbouring input frames. That is cheap
// and plenty for music played back (its error on a 440 Hz tone at 22.05
// kHz is about -60 dB); it does not low-pass before decimating, so
// downsampling may alias content above the new Nyquist frequency, which
// for common rates (44.1/48 kHz in, 44.1/48 kHz out) is inaudible.
type resampler struct {
	src  decoder
	step float64   // input frames per output frame
	buf  []float32 // input frames [0, have)
	have int
	pos  float64 // the next output frame's place in buf
	eof  bool
	err  error
}

func newResampler(src decoder, out int) *resampler {
	return &resampler{src: src, step: float64(src.rate()) / float64(out), buf: make([]float32, 2*1024)}
}

// reset forgets buffered input, as after the decoder seeks.
func (r *resampler) reset() { r.have, r.pos, r.eof, r.err = 0, 0, false, nil }

// read fills dst with output frames; the decoder contract (more than zero
// frames, or zero with an error) holds for it too.
func (r *resampler) read(dst []float32) (int, error) {
	if r.step == 1 {
		return r.src.read(dst)
	}
	n, out := len(dst)/2, 0
	for out < n {
		i := int(r.pos)
		if i+1 >= r.have {
			if !r.eof {
				r.fill(i)
				continue
			}
			if i >= r.have {
				break
			}
			dst[2*out], dst[2*out+1] = r.buf[2*i], r.buf[2*i+1] // the last frame
		} else {
			f := float32(r.pos - float64(i))
			a, b := r.buf[2*i:2*i+2], r.buf[2*i+2:2*i+4]
			dst[2*out], dst[2*out+1] = a[0]+(b[0]-a[0])*f, a[1]+(b[1]-a[1])*f
		}
		out++
		r.pos += r.step
	}
	if out == 0 {
		if r.err != nil {
			return 0, r.err
		}
		return 0, io.EOF
	}
	return out, nil
}

// fill drops the input before frame keep and reads more after it.
func (r *resampler) fill(keep int) {
	keep = min(keep, r.have)
	copy(r.buf, r.buf[2*keep:2*r.have])
	r.have -= keep
	r.pos -= float64(keep)
	for r.have < len(r.buf)/2 {
		n, err := r.src.read(r.buf[2*r.have:])
		r.have += n
		if err != nil {
			r.eof = true
			if !errors.Is(err, io.EOF) {
				r.err = err
			}
			return
		}
		if n > 0 {
			return
		}
	}
}

// Gain ramps: how long a full 0...1 swing takes, as the helper's GainRamp.
const (
	// rampSeconds keeps volume changes, pauses and track switches free of
	// clicks.
	rampSeconds = 0.015
	// closeFadeSeconds is the quiet exit's fade, slow enough to sound like
	// a fade rather than a cut (ShutdownPlan.fadeSeconds in the helper).
	closeFadeSeconds = 0.2
)

// amplitude is the gain a volume level applies: a square law, as the
// helper's, so equal steps sound roughly equal (0.5 is about -12 dB).
// NaN is silence.
func amplitude(level float64) float32 {
	if math.IsNaN(level) {
		return 0
	}
	l := min(max(level, 0), 1)
	return float32(l * l)
}

// rampStep is the gain change per frame for a full swing over seconds at
// rate; a jump without a rate or a duration.
func rampStep(rate int, seconds float64) float32 {
	if rate <= 0 || !(seconds > 0) {
		return 1
	}
	return float32(1 / (float64(rate) * seconds))
}

// ramp is a gain moving linearly toward its target by step per frame.
type ramp struct{ cur, target, step float32 }

// next advances one frame and returns the gain for it.
func (r *ramp) next() float32 {
	// Within half a step of the target counts as there, so float error
	// never leaves a ramp a hair short of it.
	switch {
	case r.cur < r.target-r.step/2:
		r.cur = min(r.cur+r.step, r.target)
	case r.cur > r.target+r.step/2:
		r.cur = max(r.cur-r.step, r.target)
	default:
		r.cur = r.target
	}
	return r.cur
}

func (r *ramp) done() bool { return r.cur == r.target }

// The levels' shape, as the helper's LevelsEvent: 24 log-spaced bands from
// 40 Hz to 16 kHz over a 2048-point FFT, read 15 times a second, plus the
// window decimated to a 64-point waveform.
const (
	numBands   = 24
	minHz      = 40.0
	maxHz      = 16000.0
	fftSize    = 2048
	wavePoints = 64
	// floorDB and ceilingDB are the band powers of an empty and a full
	// bar (the helper's LevelScale).
	floorDB   = -50.0
	ceilingDB = -10.0
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
// linear in decibels between floorDB and ceilingDB.
func normalizedLevel(power float64) float64 {
	if !(power > 0) || math.IsInf(power, 1) {
		return 0
	}
	db := 10 * math.Log10(power)
	return min(max((db-floorDB)/(ceilingDB-floorDB), 0), 1)
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
// fftSize mono samples summed into bands. It keeps its buffers; use it
// from one goroutine at a time.
type analyzer struct {
	ranges []binRange
	window []float64
	buf    []complex128
}

func newAnalyzer(rate int) *analyzer {
	a := &analyzer{ranges: bandRanges(float64(rate), fftSize, numBands, minHz, maxHz),
		window: make([]float64, fftSize), buf: make([]complex128, fftSize)}
	for i := range a.window {
		a.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/fftSize) // periodic Hann
	}
	return a
}

// levels are the band levels of fftSize samples.
func (a *analyzer) levels(samples []float32) []float64 {
	for i := range a.buf {
		a.buf[i] = complex(float64(samples[i])*a.window[i], 0)
	}
	fft(a.buf)
	// The Hann window halves a tone, so a sine of amplitude A peaks at
	// A * fftSize / 4: dividing the squared magnitudes by (fftSize/4)^2
	// makes a full-scale sine 1, as in the helper.
	scale := 1 / (fftSize / 4.0 * fftSize / 4.0)
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
// event carries, so both backends hand the UI the same values.
func quantize(v []float64) []float64 {
	for i := range v {
		v[i] = math.Round(v[i]*100) / 100
	}
	return v
}

// ring keeps the newest mono samples (the stereo mean) for the analyzer.
// It is not synchronized: the player guards it with its lock.
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
