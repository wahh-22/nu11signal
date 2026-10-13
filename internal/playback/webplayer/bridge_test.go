package webplayer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

func bridgePair(t *testing.T, callback func(BridgeMessage)) (*Bridge, net.Conn) {
	t.Helper()
	goSide, app := net.Pipe()
	b := NewBridge(goSide, callback)
	t.Cleanup(func() { b.Close(); app.Close() })
	app.SetDeadline(time.Now().Add(30 * time.Second))
	return b, app
}

func TestBridgeNowPlayingEncoding(t *testing.T) {
	for _, playing := range []bool{true, false} {
		b, app := bridgePair(t, nil)
		var m BridgeMessage
		want := fmt.Sprintf(`{"type":"nowplaying","title":"Song","artist":"Artist","album":"Album","playing":%t,"position_ms":0,"duration_ms":60000}`, playing)
		if err := json.Unmarshal([]byte(want), &m); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- b.Send(context.Background(), m) }()
		raw, err := bufio.NewReader(app).ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if raw != want+"\n" {
			t.Fatalf("got %s; want %s", raw, want)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestBridgeLevelsDecoding(t *testing.T) {
	var got BridgeMessage
	b := &Bridge{onMessage: func(m BridgeMessage) { got = m }}
	bands, wave := make([]int, 70), make([]int, 140)
	for i := range bands {
		bands[i] = 150
	}
	for i := range wave {
		wave[i] = -150
	}
	raw, _ := json.Marshal(map[string]any{"type": "levels", "bands": bands, "wave": wave})
	b.dispatch(raw)
	raw, _ = json.Marshal(got)
	var decoded struct{ Bands, Wave []int }
	json.Unmarshal(raw, &decoded)
	if len(decoded.Bands) != 64 || len(decoded.Wave) != 128 {
		t.Fatalf("sizes = %d/%d; want 64/128", len(decoded.Bands), len(decoded.Wave))
	}
	for _, v := range decoded.Bands {
		if v != 100 {
			t.Fatalf("band = %d", v)
		}
	}
	for _, v := range decoded.Wave {
		if v != -100 {
			t.Fatalf("wave = %d", v)
		}
	}
	b.dispatch([]byte(`{"type":"levels","bands":[-1,0,50,100],"wave":[-100,0,50,150]}`))
	if fmt.Sprint(got.Bands) != "[0 0 50 100]" || fmt.Sprint(got.Wave) != "[-100 0 50 100]" {
		t.Fatalf("alternate clamping = %+v", got)
	}
}

func TestBridgeLevelsEncoding(t *testing.T) {
	for _, on := range []bool{true, false} {
		b, app := bridgePair(t, nil)
		var m BridgeMessage
		json.Unmarshal([]byte(fmt.Sprintf(`{"type":"levels","on":%t}`, on)), &m)
		done := make(chan error, 1)
		go func() { done <- b.Send(context.Background(), m) }()
		raw, err := bufio.NewReader(app).ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("{\"type\":\"levels\",\"on\":%t}\n", on); raw != want {
			t.Fatalf("message = %s; want %s", raw, want)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestBridgeKeyboardEncoding(t *testing.T) {
	for _, show := range []bool{true, false} {
		b, app := bridgePair(t, nil)
		result := make(chan error, 1)
		go func() {
			result <- b.Send(context.Background(), BridgeMessage{Type: "keyboard", Show: show, ID: 99, Cols: 80})
		}()
		raw, err := bufio.NewReader(app).ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("{\"type\":\"keyboard\",\"show\":%t}\n", show); raw != want {
			t.Fatalf("message = %s; want %s", raw, want)
		}
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
}

func TestBridgeReadinessBound(t *testing.T) {
	if BridgeLoadTimeout != 2*time.Minute || BridgeLoadTimeout <= DefaultLoadTimeout {
		t.Fatalf("bridge timeout = %s; want 2m, longer than desktop", BridgeLoadTimeout)
	}
	if got := bridgeLauncher().readyTimeout; got != BridgeLoadTimeout {
		t.Fatalf("bridge readiness bound = %s; want 2m", got)
	}
	if got := defaultLauncher().readyTimeout; got != DefaultLoadTimeout {
		t.Fatalf("desktop readiness bound = %s; want %s", got, DefaultLoadTimeout)
	}
}

func TestBridgeValues(t *testing.T) {
	for _, tt := range []struct{ name, response, want, description string }{
		{"value", `"ok":true,"value":{"n":42}`, `{"n":42}`, ""},
		{"null", `"ok":true,"value":null`, `null`, ""},
		{"undefined", `"ok":true`, `null`, ""},
		{"script error", `"ok":false,"error":"bad\u0001é\nprivate stack"`, "", "bad"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, app := bridgePair(t, nil)
			go func() {
				var m BridgeMessage
				json.NewDecoder(app).Decode(&m)
				fmt.Fprintf(app, "{\"type\":\"result\",\"id\":%d,%s}\n", m.ID, tt.response)
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := b.Evaluate(ctx, "Promise.resolve(42)")
			if tt.description != "" {
				var se *ScriptError
				if !errors.As(err, &se) || se.Description != tt.description {
					t.Fatalf("error = %v", err)
				}
			} else if err != nil || string(got) != tt.want {
				t.Fatalf("value = %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestBridgeConcurrentOutOfOrder(t *testing.T) {
	b, app := bridgePair(t, nil)
	const n = 12
	go func() {
		dec := json.NewDecoder(app)
		messages := make([]BridgeMessage, n)
		for i := range messages {
			if dec.Decode(&messages[i]) != nil {
				return
			}
		}
		for i := n - 1; i >= 0; i-- {
			json.NewEncoder(app).Encode(map[string]any{"type": "result", "id": messages[i].ID, "ok": true, "value": messages[i].Expr})
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			expr := fmt.Sprint(i)
			raw, err := b.Evaluate(ctx, expr)
			if err != nil || string(raw) != fmt.Sprintf("%q", expr) {
				t.Errorf("evaluate %s = %s, %v", expr, raw, err)
			}
		})
	}
	wg.Wait()
}

func TestBridgeCancelAndClose(t *testing.T) {
	b, app := bridgePair(t, nil)
	received := make(chan struct{})
	go func() { var m BridgeMessage; json.NewDecoder(app).Decode(&m); close(received) }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := b.Evaluate(ctx, "wait"); done <- err }()
	<-received
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	b.mu.Lock()
	pending := len(b.pending)
	b.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending = %d", pending)
	}
	go func() { var m BridgeMessage; json.NewDecoder(app).Decode(&m); app.Close() }()
	if _, err := b.Evaluate(context.Background(), "close"); !errors.Is(err, ErrBrowserGone) {
		t.Fatalf("pending close = %v", err)
	}
	if _, err := b.Evaluate(context.Background(), "later"); !errors.Is(err, ErrBrowserGone) {
		t.Fatalf("future close = %v", err)
	}
}

func TestBridgeBlockedWriteCancellation(t *testing.T) {
	b, _ := bridgePair(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := b.Evaluate(ctx, "unread"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked write = %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if err := b.Send(ctx2, BridgeMessage{Type: "login", Show: true}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued send = %v", err)
	}
	b.Close()
	<-b.Done()
}

func TestBridgeInvalidLinesAndControls(t *testing.T) {
	controls := make(chan BridgeMessage, 2)
	b, app := bridgePair(t, func(m BridgeMessage) { controls <- m })
	go func() {
		reader := bufio.NewReader(app)
		line, _ := reader.ReadBytes('\n')
		var m BridgeMessage
		json.Unmarshal(line, &m)
		fmt.Fprintln(app, strings.Repeat("x", defaultMaxMessage+1))
		fmt.Fprintln(app, `not json`)
		fmt.Fprintf(app, "{\"type\":\"result\",\"id\":%d}\n", m.ID)
		fmt.Fprintln(app, `{"type":"resize","cols":120,"rows":40}`)
		fmt.Fprintln(app, `{"type":"login","closed":true}`)
		fmt.Fprintf(app, "{\"type\":\"result\",\"id\":%d,\"ok\":true,\"value\":42}\n", m.ID)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := b.Evaluate(ctx, "42")
	if err != nil || string(raw) != "42" {
		t.Fatalf("evaluate = %s, %v", raw, err)
	}
	if m := <-controls; m.Type != "resize" || m.Cols != 120 || m.Rows != 40 {
		t.Fatalf("resize = %+v", m)
	}
	if m := <-controls; m.Type != "login" || !m.Closed {
		t.Fatalf("login = %+v", m)
	}
}

func TestPlayerOverBridge(t *testing.T) {
	b, app := bridgePair(t, nil)
	page := newPage()
	page.setHandle(func(c call) (any, error) {
		if c.fn == "status" {
			return map[string]any{"ready": true, "authorized": true}, nil
		}
		return nil, nil
	})
	go func() {
		dec := json.NewDecoder(app)
		enc := json.NewEncoder(app)
		for {
			var m BridgeMessage
			if dec.Decode(&m) != nil {
				return
			}
			raw, err := page.Evaluate(context.Background(), m.Expr)
			if err != nil {
				return
			}
			enc.Encode(map[string]any{"type": "result", "id": m.ID, "ok": true, "value": raw})
		}
	}()
	p := New(b)
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := WaitReady(ctx, p); err != nil {
		t.Fatal(err)
	}
	if auth, err := p.Authorize(ctx); err != nil || auth != playback.AuthAuthorized {
		t.Fatalf("authorize = %s, %v", auth, err)
	}
}

func TestBridgeLoginEnvelope(t *testing.T) {
	b, app := bridgePair(t, nil)
	messages := make(chan string, 2)
	go func() {
		reader := bufio.NewReader(app)
		for range 2 {
			line, _ := reader.ReadString('\n')
			messages <- strings.TrimSpace(line)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, show := range []bool{true, false} {
		if err := b.Send(ctx, BridgeMessage{Type: "login", Show: show}); err != nil {
			t.Fatal(err)
		}
		if got, want := <-messages, fmt.Sprintf(`{"type":"login","show":%t}`, show); got != want {
			t.Fatalf("message = %s; want %s", got, want)
		}
	}
}

func TestBridgeWaitReadyFailures(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprintf("closed=%t", closed), func(t *testing.T) {
			b, app := bridgePair(t, nil)
			if closed {
				app.Close()
			} else {
				go func() {
					decoder := json.NewDecoder(app)
					for {
						var m BridgeMessage
						if decoder.Decode(&m) != nil {
							return
						}
						value := `{"installed":true}`
						if m.Expr != bootstrap {
							value = `{"value":{"ready":false,"authorized":true}}`
						}
						fmt.Fprintf(app, "{\"type\":\"result\",\"id\":%d,\"ok\":true,\"value\":%s}\n", m.ID, value)
					}
				}()
			}
			p := New(b)
			defer p.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			err := WaitReady(ctx, p)
			want := context.DeadlineExceeded
			if closed {
				want = ErrBrowserGone
			}
			if !errors.Is(err, ErrNotLoaded) || !errors.Is(err, want) {
				t.Fatalf("readiness = %v; want %v and ErrNotLoaded", err, want)
			}
		})
	}
}
