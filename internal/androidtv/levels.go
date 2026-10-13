package androidtv

import (
	"context"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

// offerLatest never holds up the bridge reader or playback state polling.
func offerLatest[T any](ch chan T, v T) {
	select {
	case ch <- v:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- v:
	default:
	}
}

// startSpectrum consumes the underlying states once, forwarding them to the UI
// and ordering capture controls with playback. The worker owns channel closure.
func (p *Player) startSpectrum(states <-chan playback.State, readings <-chan webplayer.BridgeMessage, send func(context.Context, webplayer.BridgeMessage) error) {
	p.spectrumStates = make(chan playback.State, 16)
	p.levels = make(chan playback.Spectrum, 1)
	p.captureStop = make(chan struct{})
	p.captureDone = make(chan struct{})
	go func() {
		defer close(p.captureDone)
		defer close(p.spectrumStates)
		defer close(p.levels)
		on := false
		var nowPlaying nowPlayingTracker
		var lastSent time.Time
		var lastMessage webplayer.BridgeMessage
		var dirty bool
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		defer timer.Stop()
		var due <-chan time.Time
		publish := func() {
			dirty = false
			due = nil
			if sameNowPlaying(nowPlaying.message, lastMessage) {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = send(ctx, nowPlaying.message)
			lastSent = time.Now()
			lastMessage = nowPlaying.message
		}
		control := func(next bool) {
			if next == on {
				return
			}
			on = next
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = send(ctx, webplayer.BridgeMessage{Type: "levels", On: on})
			if !on {
				select {
				case <-p.levels:
				default:
				}
			}
		}
		defer func() { control(false) }()
		for {
			select {
			case <-p.captureStop:
				return
			case <-due:
				if dirty {
					publish()
				}
			case s, ok := <-states:
				if !ok {
					return
				}
				control(s.Status == playback.StatusPlaying)
				if nowPlaying.update(s, time.Now()) {
					dirty = true
					if time.Since(lastSent) >= time.Second {
						publish()
					} else if due == nil {
						timer.Reset(time.Second - time.Since(lastSent))
						due = timer.C
					}
				}
				offerLatest(p.spectrumStates, s)
			case m := <-readings:
				if !on || len(m.Bands) == 0 {
					continue
				}
				s := playback.Spectrum{Bands: make([]float64, len(m.Bands)), Wave: make([]float64, len(m.Wave))}
				for i, v := range m.Bands {
					s.Bands[i] = float64(v) / 100
				}
				for i, v := range m.Wave {
					s.Wave[i] = float64(v) / 100
				}
				offerLatest(p.levels, s)
			}
		}
	}()
}

var noLevels = func() <-chan playback.Spectrum {
	ch := make(chan playback.Spectrum)
	close(ch)
	return ch
}()

func (p *Player) Levels() <-chan playback.Spectrum {
	// A wrapper built without a Runtime has no capture worker. Never expose
	// a nil channel: composite must be able to finish its forwarding worker.
	if p.levels == nil {
		return noLevels
	}
	return p.levels
}

func (p *Player) States() <-chan playback.State {
	if p.spectrumStates == nil {
		return p.Player.States()
	}
	return p.spectrumStates
}

func (p *Player) Close() error {
	p.closeOnce.Do(func() {
		if p.captureStop != nil {
			close(p.captureStop)
			<-p.captureDone
		}
		if p.Player != nil {
			p.closeErr = p.Player.Close()
		}
	})
	return p.closeErr
}

var _ playback.LevelSource = (*Player)(nil)
