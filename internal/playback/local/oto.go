package local

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// This file is the only one that touches an audio device: everything else
// in the package renders into a sink.

// otoBuffer is how much audio the oto player pulls ahead of the device:
// it bounds how late a pause, a volume change or a switch is heard (oto's
// default is half a second), against the risk of an underrun.
const otoBuffer = 60 * time.Millisecond

// openTimeout bounds how long a play waits for the audio output to open
// (oto opens it on its own goroutine, trying PulseAudio, then ALSA): a
// device that never answers fails the play rather than freezing it.
const openTimeout = 5 * time.Second

// oto supports one context per process, at one sample rate, so it is
// shared by every otoSink. A context still opening when a play gave up on
// it is kept, with its ready channel, for the next play to wait on: oto
// would refuse a second one. Once one fails, every play fails as it did.
var (
	otoMu    sync.Mutex
	otoCtx   *oto.Context
	otoReady chan struct{}
	otoRate  int
	otoErr   error
	otoOpen  bool // otoCtx is open: ready and without error
)

func otoContext(rate int) (*oto.Context, error) {
	otoMu.Lock()
	defer otoMu.Unlock()
	if otoErr != nil {
		return nil, otoErr
	}
	if otoCtx != nil && rate != otoRate {
		return nil, fmt.Errorf("local: audio output already open at %d Hz", otoRate)
	}
	if otoOpen {
		return otoCtx, nil
	}
	if otoCtx == nil {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate: rate, ChannelCount: 2, Format: oto.FormatFloat32LE, ApplicationName: "nu11signal",
		})
		if err != nil {
			return nil, fmt.Errorf("local: audio output: %w", err)
		}
		otoCtx, otoReady, otoRate = ctx, ready, rate
	}
	timer := time.NewTimer(openTimeout)
	defer timer.Stop()
	if err := awaitOutput(otoReady, timer.C); err != nil {
		return nil, err
	}
	if err := otoCtx.Err(); err != nil {
		otoErr = fmt.Errorf("local: audio output: %w", err)
		return nil, otoErr
	}
	otoOpen = true
	return otoCtx, nil
}

// awaitOutput waits for an audio output opening to be ready, or reports
// it as missing (playback.ErrNoOutput) once timeout fires.
func awaitOutput(ready <-chan struct{}, timeout <-chan time.Time) error {
	select {
	case <-ready:
		return nil
	case <-timeout:
		return fmt.Errorf("local: audio output did not open within %v: %w", openTimeout, noOutput())
	}
}

// otoSink plays the rendered audio on the default output device: macOS
// Core Audio, or PulseAudio (falling back to ALSA) on Linux, all loaded at
// run time by oto, so no cgo is needed.
type otoSink struct {
	player *oto.Player
}

func (s *otoSink) start(rate int, src func([]float32)) error {
	if s.player != nil {
		return errors.New("local: audio output already started")
	}
	ctx, err := otoContext(rate)
	if err != nil {
		return err
	}
	s.player = ctx.NewPlayer(&pcmReader{src: src})
	s.player.SetBufferSize(int(int64(rate) * 8 * int64(otoBuffer) / int64(time.Second)))
	s.player.Play()
	return nil
}

func (s *otoSink) close() error {
	if s.player == nil {
		return nil
	}
	s.player.Pause()
	return s.player.Close()
}

// pcmReader serves rendered stereo frames as oto's little-endian float32
// bytes; it never ends, rendering silence when nothing plays.
type pcmReader struct {
	src func([]float32)
	buf []float32
}

func (r *pcmReader) Read(p []byte) (int, error) {
	frames := len(p) / 8
	if cap(r.buf) < 2*frames {
		r.buf = make([]float32, 2*frames)
	}
	buf := r.buf[:2*frames]
	r.src(buf)
	for i, v := range buf {
		binary.LittleEndian.PutUint32(p[4*i:], math.Float32bits(v))
	}
	return 8 * frames, nil
}
