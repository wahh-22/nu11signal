//go:build linux && !android

package pulsecapture

import (
	"fmt"

	"github.com/jfreymuth/pulse"
)

type pulseClient struct{ c *pulse.Client }

func connect() (client, error) {
	c, err := pulse.NewClient()
	if err != nil {
		return nil, fmt.Errorf("pulsecapture: connect: %w", err)
	}
	return &pulseClient{c}, nil
}

func (c *pulseClient) close() { c.c.Close() }

func (c *pulseClient) record(cb func([]float32, int)) (func(), error) {
	sink, err := c.c.DefaultSink()
	if err != nil {
		return nil, fmt.Errorf("pulsecapture: default sink: %w", err)
	}
	if sink == nil {
		return nil, fmt.Errorf("pulsecapture: no default sink")
	}
	// Pulse resamples the monitor to this rate and mixes it down to mono.
	const rate = 48000
	stream, err := c.c.NewRecord(pulse.Float32Writer(func(samples []float32) (int, error) {
		cb(samples, rate)
		return len(samples), nil
	}), pulse.RecordMonitor(sink), pulse.RecordMono, pulse.RecordSampleRate(rate), pulse.RecordBufferFragmentSize(rate*4/40))
	if err != nil {
		return nil, fmt.Errorf("pulsecapture: record: %w", err)
	}
	stream.Start()
	return func() {
		stream.Stop()
		stream.Close()
	}, nil
}
