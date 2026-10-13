package spectrum

import (
	"sync"
	"testing"
	"time"
)

func TestResetWaitsForReading(t *testing.T) {
	m := NewMeter(48000)
	m.WriteMono(make([]float32, FFTSize))
	// Hold analysis to model a reading in flight.
	m.measureMu.Lock()
	read := make(chan struct{})
	go func() { m.Reading(); close(read) }()
	time.Sleep(20 * time.Millisecond)
	reset := make(chan struct{})
	go func() { m.Reset(); close(reset) }()
	select {
	case <-reset:
		m.measureMu.Unlock()
		<-read
		t.Fatal("Reset completed while Reading was in flight")
	case <-time.After(20 * time.Millisecond):
	}
	m.measureMu.Unlock()
	<-read
	<-reset
	if _, ok := m.Reading(); ok {
		t.Fatal("reset retained samples")
	}
}

func TestMeterConcurrentWriterReaderReset(t *testing.T) {
	m := NewMeter(48000)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				switch i {
				case 0:
					m.WriteMono(make([]float32, FFTSize))
				case 1:
					m.Reading()
				case 2:
					m.Reset()
				}
			}
		}(i)
	}
	wg.Wait()
}
