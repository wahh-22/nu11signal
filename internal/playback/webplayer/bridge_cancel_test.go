package webplayer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestBridgeCancellationNotifiesApp(t *testing.T) {
	b, app := bridgePair(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := b.Evaluate(ctx, "slow()")
		done <- err
	}()
	dec := json.NewDecoder(app)
	var evaluation BridgeMessage
	if err := dec.Decode(&evaluation); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	app.SetReadDeadline(time.Now().Add(time.Second))
	var message map[string]any
	if err := dec.Decode(&message); err != nil {
		t.Fatalf("missing cancellation: %v", err)
	}
	if message["type"] != "cancel" || message["id"] != float64(evaluation.ID) || len(message) != 2 {
		t.Fatalf("cancel envelope = %v", message)
	}
	b.dispatch([]byte(`{"type":"result","id":1,"ok":true,"value":42}`))
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.pending) != 0 {
		t.Fatalf("pending = %d", len(b.pending))
	}
}
