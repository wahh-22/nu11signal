package webplayer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// BridgeLoadTimeout bounds MusicKit readiness on slow TV devices: a Google
// TV Streamer was observed taking about 41 seconds to make MusicKit available.
const BridgeLoadTimeout = 2 * time.Minute

// BridgeMessage is a control message on the app bridge. Resize reports
// terminal cells; login reports that the user left sign-in. Show is sent
// explicitly, including false, when Go opens or hides sign-in. Result
// fields are decoded separately so a missing ok cannot complete a call.
type BridgeMessage struct {
	Type   string `json:"type"`
	ID     int64  `json:"id,omitempty"`
	Expr   string `json:"expr,omitempty"`
	Show   bool   `json:"show"`
	Closed bool   `json:"closed,omitempty"`
	Cols   int    `json:"cols,omitempty"`
	Rows   int    `json:"rows,omitempty"`
}

// Bridge evaluates expressions through an app-owned WebView over a
// JSON-lines io.ReadWriteCloser. Go sends {"type":"evaluate","id":int64,
// "expr":string} or {"type":"login","show":bool}. The app awaits promises
// and returns values by value: {"type":"result","id":int64,"ok":true,
// "value":JSON} (null for undefined), or ok:false with an error string.
// Exceptions become sanitized ScriptErrors, just as with CDP. The app
// also sends {"type":"resize","cols":int,"rows":int} and
// {"type":"login","closed":true}. These go to onMessage, on the reader
// goroutine: it must return quickly and must not call Evaluate or Send.
// Calls may overlap and replies may arrive out of order. Writes are
// serialized; cancellation forgets a call, not a partially written line.
// Close unblocks writes and fails pending and future calls with
// ErrBrowserGone. Malformed and oversized inbound lines are discarded.
type Bridge struct {
	conn      io.ReadWriteCloser
	onMessage func(BridgeMessage)
	writeSlot chan struct{}
	mu        sync.Mutex
	nextID    int64
	pending   map[int64]chan reply
	err       error
	done      chan struct{}
	closeOnce sync.Once
}

// NewBridge starts the sole reader; onMessage may be nil. Close must be
// called by the owner, including after the app disconnects.
func NewBridge(conn io.ReadWriteCloser, onMessage func(BridgeMessage)) *Bridge {
	b := &Bridge{
		conn:      conn,
		onMessage: onMessage,
		writeSlot: make(chan struct{}, 1),
		pending:   make(map[int64]chan reply),
		done:      make(chan struct{}),
	}
	go b.readLoop()
	return b
}

// Done closes when the connection is unusable.
func (b *Bridge) Done() <-chan struct{} { return b.done }

// Close is idempotent and releases the connection, including stuck writes.
func (b *Bridge) Close() error {
	b.closeOnce.Do(func() { b.fail(ErrBrowserGone); b.conn.Close() })
	return nil
}

// Evaluate sends an expression and waits for its settled JSON value.
func (b *Bridge) Evaluate(ctx context.Context, expr string) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ch := make(chan reply, 1)
	b.mu.Lock()
	if b.err != nil {
		err := b.err
		b.mu.Unlock()
		return nil, err
	}
	b.nextID++
	id := b.nextID
	b.pending[id] = ch
	b.mu.Unlock()
	defer b.forget(id)
	if err := b.Send(ctx, BridgeMessage{Type: "evaluate", ID: id, Expr: expr}); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		return r.result, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Send writes a control message, bounded by ctx. A write abandoned by a
// caller retains its slot until completion or Close, so no next message
// can interleave with it. A write failure closes the whole connection.
func (b *Bridge) Send(ctx context.Context, m BridgeMessage) error {
	// Keep evaluate and login envelopes exact, without unrelated fields.
	var payload any = m
	switch m.Type {
	case "evaluate":
		payload = struct {
			Type string `json:"type"`
			ID   int64  `json:"id"`
			Expr string `json:"expr"`
		}{m.Type, m.ID, m.Expr}
	case "login":
		payload = struct {
			Type string `json:"type"`
			Show bool   `json:"show"`
		}{m.Type, m.Show}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	select {
	case <-b.done:
		return b.gone()
	default:
	}
	select {
	case b.writeSlot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return b.gone()
	}
	if err := ctx.Err(); err != nil {
		<-b.writeSlot
		return err
	}
	written := make(chan error, 1)
	go func() {
		defer func() { <-b.writeSlot }()
		_, err := io.Copy(b.conn, bytes.NewReader(append(raw, '\n')))
		if err != nil {
			b.fail(fmt.Errorf("%w: write: %v", ErrBrowserGone, err))
			b.conn.Close()
			err = b.gone()
		}
		written <- err
	}()
	select {
	case err := <-written:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return b.gone()
	}
}

func (b *Bridge) gone() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

func (b *Bridge) forget(id int64) {
	b.mu.Lock()
	delete(b.pending, id)
	b.mu.Unlock()
}

func (b *Bridge) fail(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return
	}
	b.err = err
	for id, ch := range b.pending {
		ch <- reply{err: err}
		delete(b.pending, id)
	}
	close(b.done)
}

func (b *Bridge) readLoop() {
	defer b.conn.Close()
	reader := bufio.NewReaderSize(b.conn, readBuffer)
	for {
		raw, err := readBridgeLine(reader)
		if err != nil {
			b.fail(fmt.Errorf("%w: read: %v", ErrBrowserGone, err))
			return
		}
		if raw != nil {
			b.dispatch(raw)
		}
	}
}

// readBridgeLine drains an oversized line without retaining its tail;
// the next newline always starts a fresh frame, even after malformed JSON.
func readBridgeLine(reader *bufio.Reader) ([]byte, error) {
	var raw []byte
	oversized := false
	for {
		chunk, err := reader.ReadSlice('\n')
		if !oversized {
			if len(raw)+len(chunk) > defaultMaxMessage+1 {
				oversized = true
				raw = nil
			} else {
				raw = append(raw, chunk...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if oversized {
			return nil, nil
		}
		return raw[:len(raw)-1], nil
	}
}

func (b *Bridge) dispatch(raw []byte) {
	var m struct {
		BridgeMessage
		OK    *bool           `json:"ok"`
		Value json.RawMessage `json:"value"`
		Error string          `json:"error"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	if m.Type != "result" {
		if m.Type != "" && b.onMessage != nil {
			b.onMessage(m.BridgeMessage)
		}
		return
	}
	if m.ID <= 0 || m.OK == nil {
		return
	}
	r := reply{result: m.Value}
	if !*m.OK {
		r.err = &ScriptError{Description: sanitizeDescription(m.Error)}
	} else if len(r.result) == 0 {
		r.result = json.RawMessage("null")
	}
	b.mu.Lock()
	ch := b.pending[m.ID]
	delete(b.pending, m.ID)
	b.mu.Unlock()
	if ch != nil {
		ch <- r
	}
}

// WaitReady waits for the same MusicKit readiness used by Open, with
// BridgeLoadTimeout as an upper bound in addition to ctx. The bridge
// keeps the page visible: hiding the WebView is the app's responsibility.
func WaitReady(ctx context.Context, p *Player) error {
	_, err := bridgeLauncher().waitReady(ctx, p)
	return err
}

func bridgeLauncher() launcher {
	l := defaultLauncher()
	l.readyTimeout = BridgeLoadTimeout
	return l
}
