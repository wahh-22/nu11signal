package local

import (
	"errors"
	"io"
	"math"
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
