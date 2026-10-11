package pulsecapture

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClient struct {
	fail           bool
	starts, closes atomic.Int32
}

func (f *fakeClient) record(cb func([]float32, int)) (func(), error) {
	if f.fail {
		return nil, errors.New("no sink")
	}
	f.starts.Add(1)
	cb([]float32{1}, 48000)
	return func() { f.closes.Add(1) }, nil
}

func (f *fakeClient) close() { f.closes.Add(1) }

func TestCaptureLifecycle(t *testing.T) {
	f := &fakeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	got := false
	stop, err := start(ctx, func(s []float32, r int) { got = len(s) == 1 && r == 48000 }, func() (client, error) { return f, nil })
	if err != nil || !got || f.starts.Load() != 1 {
		t.Fatalf("start: %v samples %v", err, got)
	}
	cancel()
	stop()
	stop()
	deadline := time.Now().Add(time.Second)
	for f.closes.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
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
