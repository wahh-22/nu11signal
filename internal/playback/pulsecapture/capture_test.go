package pulsecapture

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClient struct {
	fail           bool
	starts, closes atomic.Int32
	cb             func([]float32, int)
}

func (f *fakeClient) record(cb func([]float32, int)) (func(), error) {
	if f.fail {
		return nil, errors.New("no sink")
	}
	f.starts.Add(1)
	f.cb = cb
	cb([]float32{1}, 48000)
	return func() { f.closes.Add(1) }, nil
}

func (f *fakeClient) close() { f.closes.Add(1) }

func TestCaptureLifecycle(t *testing.T) {
	f := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	got := false
	var samples atomic.Int32
	stop, err := start(ctx, func(s []float32, r int) {
		samples.Add(1)
		got = len(s) == 1 && r == 48000
	}, func() (client, error) { return f, nil })
	if err != nil || !got || f.starts.Load() != 1 {
		t.Fatalf("start: %v samples %v", err, got)
	}
	cancel()
	// Cancellation alone must release resources; do not hide it with stop.
	deadline := time.Now().Add(time.Second)
	for f.closes.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.closes.Load() != 2 {
		t.Fatal("cleanup must be exactly once")
	}
	f.cb([]float32{1}, 48000)
	if samples.Load() != 1 {
		t.Fatal("samples continued after cancellation")
	}
	stop()
	stop()
}

// A recorder may still be waiting for the server when cancellation arrives.
type startingClient struct {
	entered, closed chan struct{}
	once            sync.Once
	ended           atomic.Int32
	cb              func([]float32, int)
}

func (f *startingClient) record(cb func([]float32, int)) (func(), error) {
	f.cb = cb
	close(f.entered)
	<-f.closed
	cb([]float32{1}, 48000)
	return func() { f.ended.Add(1) }, nil
}
func (f *startingClient) close() { f.once.Do(func() { close(f.closed) }) }

func TestCaptureCancellationDuringStartup(t *testing.T) {
	f := &startingClient{entered: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var samples atomic.Int32
	returned := make(chan error, 1)
	go func() {
		stop, err := start(ctx, func([]float32, int) { samples.Add(1) }, func() (client, error) { return f, nil })
		if stop != nil {
			stop()
		}
		returned <- err
	}()
	<-f.entered
	cancel()
	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation leaked the starting recorder")
	}
	f.cb([]float32{1}, 48000)
	if samples.Load() != 0 || f.ended.Load() != 1 {
		t.Fatalf("samples=%d stream stops=%d", samples.Load(), f.ended.Load())
	}
}

type callbackStoppingClient struct {
	fakeClient
	returned chan struct{}
}

func (f *callbackStoppingClient) record(cb func([]float32, int)) (func(), error) {
	end, err := f.fakeClient.record(cb)
	return func() {
		<-f.returned // Model a recorder whose shutdown awaits its callback.
		end()
	}, err
}

func TestCaptureCallbackCanStop(t *testing.T) {
	f := &callbackStoppingClient{returned: make(chan struct{})}
	var stop func()
	stop, err := start(context.Background(), func([]float32, int) {
		if stop != nil {
			stop()
		}
	}, func() (client, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		f.cb([]float32{1}, 48000)
		close(f.returned)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback calling stop deadlocked")
	}
	stop()
	if f.closes.Load() != 2 {
		t.Fatal("cleanup must be exactly once")
	}
}

func TestCaptureErrors(t *testing.T) {
	for _, f := range []*fakeClient{nil, {fail: true}} {
		_, err := start(context.Background(), func([]float32, int) {}, func() (client, error) {
			if f == nil {
				return nil, errors.New("no server")
			}
			return f, nil
		})
		if err == nil {
			t.Fatal("missing error")
		}
		if f != nil && f.closes.Load() != 1 {
			t.Fatal("leaked client")
		}
	}
}
