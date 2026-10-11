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
	end, err := c.record(onSamples)
	if err != nil {
		c.close()
		return nil, err
	}
	var once sync.Once
	done := make(chan struct{})
	stop := func() {
		once.Do(func() {
			end()
			c.close()
			close(done)
		})
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
