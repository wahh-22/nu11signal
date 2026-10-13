// Package pulsecapture records the default output's system mix as mono PCM.
package pulsecapture

import (
	"context"
	"errors"
	"sync"
)

// ErrUnsupported indicates that monitor recording is unavailable on this OS.
var ErrUnsupported = errors.New("pulsecapture: supported only on Linux")

// client isolates the PulseAudio connection and monitor stream for tests.
type client interface {
	record(func([]float32, int)) (func(), error)
	close()
}

// Start records float32 mono PCM. The callback must not retain the slice.
// stop is idempotent; cancellation also releases the stream and connection.
// During a callback, stop disables delivery immediately and cleans up
// asynchronously, so the recorder can finish that callback first.
func Start(ctx context.Context, onSamples func([]float32, int)) (func(), error) {
	return start(ctx, onSamples, connect)
}

func start(ctx context.Context, onSamples func([]float32, int), open func() (client, error)) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := open()
	if err != nil {
		return nil, err
	}
	var closeOnce sync.Once
	closeClient := func() { closeOnce.Do(c.close) }
	var callbackMu sync.Mutex
	stopped := false
	active := 0
	disable := func() {
		callbackMu.Lock()
		stopped = true
		callbackMu.Unlock()
	}
	// Close the connection even while record is waiting for a server reply.
	done := make(chan struct{})
	started := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			select {
			case <-started:
				// Once started, stop owns stream-before-connection cleanup.
			default:
				disable()
				closeClient()
			}
		case <-done:
		}
	}()
	end, err := c.record(func(samples []float32, rate int) {
		callbackMu.Lock()
		enabled := !stopped && ctx.Err() == nil
		if enabled {
			active++
		}
		callbackMu.Unlock()
		// User callbacks may call stop; never invoke them under its lock.
		if enabled {
			defer func() {
				callbackMu.Lock()
				active--
				callbackMu.Unlock()
			}()
			onSamples(samples, rate)
		}
	})
	close(started)
	if err != nil {
		disable()
		closeClient()
		close(done)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			disable()
			// Pulse stream shutdown may await the recorder's callback.
			go func() {
				end()
				closeClient()
				close(done)
			}()
		})
		callbackMu.Lock()
		inCallback := active > 0
		callbackMu.Unlock()
		if !inCallback {
			<-done
		}
	}
	if err := ctx.Err(); err != nil {
		stop()
		return nil, err
	}
	go func() {
		select {
		case <-ctx.Done():
			stop()
		case <-done:
		}
	}()
	return stop, nil
}
