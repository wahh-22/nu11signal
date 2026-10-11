package webplayer

import (
	"context"
	"sync"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/spectrum"
)

var _ playback.LevelSource = (*Player)(nil)

// Levels delivers the latest reading of the Linux default sink's system mix.
// Capture unavailable (including non-Linux) means no readings, not a UI error.
func (p *Player) Levels() <-chan playback.Spectrum { return p.readings }

// updateCaptureLocked ties a capture session to each playing interval. A
// cancellation happens immediately, while opening/closing devices never holds
// the player lock. The gate orders a new recorder after the old one's cleanup.
func (p *Player) updateCaptureLocked(status playback.Status) {
	if status != playback.StatusPlaying {
		if p.captureCancel != nil {
			p.captureCancel()
			p.captureCancel = nil
		}
		select {
		case <-p.readings:
		default:
		}
		return
	}
	if p.captureCancel != nil || p.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	p.captureCancel = cancel
	p.captureWG.Add(1)
	go p.captureLevels(ctx)
}

func (p *Player) captureLevels(ctx context.Context) {
	defer p.captureWG.Done()
	p.captureGate.Lock()
	defer p.captureGate.Unlock()
	if ctx.Err() != nil {
		return
	}
	var mu sync.Mutex
	var meter *spectrum.Meter
	rate := 0
	fresh := false
	stop, err := p.capture(ctx, func(samples []float32, r int) {
		if r <= 0 || ctx.Err() != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if meter == nil || rate != r {
			meter = spectrum.NewMeter(r)
			rate = r
		}
		meter.WriteMono(samples)
		if len(samples) > 0 {
			fresh = true
		}
	})
	// The TUI owns stderr. Capture is optional, so failure stays silent and is
	// not retried on every poll; the next playing interval can try again.
	if err != nil {
		return
	}
	defer stop()
	ticker := time.NewTicker(spectrum.ReadingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mu.Lock()
			var reading playback.Spectrum
			ok := false
			if meter != nil && fresh {
				reading, ok = meter.Reading()
				fresh = false
			}
			mu.Unlock()
			if !ok {
				continue
			}
			p.mu.Lock()
			if !p.closed && ctx.Err() == nil {
				select {
				case <-p.readings:
				default:
				}
				select {
				case p.readings <- reading:
				default:
				}
			}
			p.mu.Unlock()
		}
	}
}
