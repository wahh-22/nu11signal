// Package helper adapts the soulking-helper process (a windowless MusicKit
// app speaking JSON Lines on stdin/stdout) to the playback.Player port.
package helper

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"soulking/internal/playback"
)

var (
	// ErrHelperExited is wrapped by every error caused by the helper
	// process ending, including calls that were pending when it died.
	ErrHelperExited = errors.New("helper process exited")
	// ErrClosed is returned by calls made after Close.
	ErrClosed = errors.New("helper client closed")
)

// CommandError is a failure the helper reported for one command.
type CommandError struct {
	Command string
	Message string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("helper %s: %s", e.Command, e.Message)
}

// Options configures Start.
type Options struct {
	// Path is the helper executable (see Locate).
	Path string
	// Env is the helper's environment; nil inherits the current one.
	Env []string
	// Stderr, when set, receives a copy of the helper's diagnostics. The
	// helper never writes to the parent's terminal; the last few KiB are
	// always kept to explain an unexpected exit.
	Stderr io.Writer
	// ReadyTimeout bounds the wait for the "ready" event (default 5s).
	ReadyTimeout time.Duration
	// CloseTimeout bounds the wait for exit after stdin closes (default 2s).
	CloseTimeout time.Duration
}

const (
	stateBuffer  = 8
	errorBuffer  = 16
	stderrTail   = 4 << 10
	maxLineBytes = 4 << 20
)

type reply struct {
	msg inbound
	err error
}

// Client is a running helper process. It implements playback.Player.
type Client struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stderr       *tailBuffer
	closeTimeout time.Duration

	// writeSlot (capacity 1) serializes writes to stdin. It is a channel,
	// not a mutex, so a caller waiting behind a stuck write gives up with
	// its ctx instead of leaving a goroutine parked on a lock.
	writeSlot chan struct{}

	mu      sync.Mutex
	pending map[string]chan reply
	nextID  uint64
	closing bool
	err     error // why the client is unusable, once the process is gone

	readyOnce sync.Once
	ready     chan struct{}
	done      chan struct{} // closed after the process is reaped
	waitErr   error         // written before done is closed
	states    chan playback.State
	errs      chan error

	closeOnce sync.Once
	closeErr  error
}

// Start launches the helper and waits until it reports ready. ctx bounds
// only the startup; the process lives until Close or until it exits.
func Start(ctx context.Context, opts Options) (*Client, error) {
	if opts.Path == "" {
		return nil, errors.New("helper: no executable path")
	}
	if opts.ReadyTimeout <= 0 {
		opts.ReadyTimeout = 5 * time.Second
	}
	if opts.CloseTimeout <= 0 {
		opts.CloseTimeout = 2 * time.Second
	}

	cmd := exec.Command(opts.Path)
	cmd.Env = opts.Env
	tail := &tailBuffer{max: stderrTail}
	cmd.Stderr = tail
	if opts.Stderr != nil {
		cmd.Stderr = io.MultiWriter(tail, ignoreErrors{opts.Stderr})
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("helper: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("helper: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("helper: start %s: %w", opts.Path, err)
	}

	c := &Client{
		cmd:          cmd,
		stdin:        stdin,
		stderr:       tail,
		closeTimeout: opts.CloseTimeout,
		writeSlot:    make(chan struct{}, 1),
		pending:      make(map[string]chan reply),
		ready:        make(chan struct{}),
		done:         make(chan struct{}),
		states:       make(chan playback.State, stateBuffer),
		errs:         make(chan error, errorBuffer),
	}
	go c.readLoop(stdout)

	timer := time.NewTimer(opts.ReadyTimeout)
	defer timer.Stop()
	select {
	case <-c.ready:
		return c, nil
	case <-c.done:
		return nil, fmt.Errorf("helper: exited before ready: %w", c.err)
	case <-timer.C:
		c.kill()
		return nil, fmt.Errorf("helper: no ready event within %s%s", opts.ReadyTimeout, tailSuffix(tail.String()))
	case <-ctx.Done():
		c.kill()
		return nil, ctx.Err()
	}
}

// States delivers state snapshots. When the consumer falls behind, the
// oldest snapshots are dropped so the latest one is always available.
func (c *Client) States() <-chan playback.State { return c.states }

// Errors delivers asynchronous helper failures (error events and responses
// to requests the helper could not parse).
func (c *Client) Errors() <-chan error { return c.errs }

// Close closes the helper's stdin, waits up to CloseTimeout for it to exit,
// and kills it otherwise. It is idempotent.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closing = true
		c.mu.Unlock()
		_ = c.stdin.Close()

		timer := time.NewTimer(c.closeTimeout)
		defer timer.Stop()
		select {
		case <-c.done:
			if c.waitErr != nil {
				c.closeErr = fmt.Errorf("helper: %w", c.waitErr)
			}
		case <-timer.C:
			c.kill()
			c.closeErr = fmt.Errorf("helper: did not exit within %s of stdin closing; killed", c.closeTimeout)
		}
	})
	return c.closeErr
}

// kill terminates the process and waits until the reader has shut down.
func (c *Client) kill() {
	_ = c.cmd.Process.Kill()
	<-c.done
}

// call sends one request and waits for its response or ctx.
func (c *Client) call(ctx context.Context, cmd string, args map[string]any, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ch := make(chan reply, 1)
	c.mu.Lock()
	switch {
	case c.closing:
		c.mu.Unlock()
		return ErrClosed
	case c.err != nil:
		c.mu.Unlock()
		return c.err
	}
	c.nextID++
	id := strconv.FormatUint(c.nextID, 10)
	c.pending[id] = ch
	c.mu.Unlock()

	line, err := encodeRequest(id, cmd, args)
	if err != nil {
		c.forget(id)
		return fmt.Errorf("helper %s: encode: %w", cmd, err)
	}
	if err := c.send(ctx, line); err != nil {
		c.forget(id)
		if errors.Is(err, ctx.Err()) || errors.Is(err, ErrClosed) {
			return err
		}
		return fmt.Errorf("helper %s: send: %w", cmd, err)
	}

	select {
	case <-ctx.Done():
		c.forget(id)
		return ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if !r.msg.OK {
			return &CommandError{Command: cmd, Message: r.msg.Error}
		}
		if result != nil && len(r.msg.Result) > 0 {
			if err := json.Unmarshal(r.msg.Result, result); err != nil {
				return fmt.Errorf("helper %s: decode result: %w", cmd, err)
			}
		}
		return nil
	}
}

// send writes one request line without letting a helper that stops
// reading block the caller past ctx.
//
// The caller takes the write slot under ctx, then the write itself runs in
// a goroutine that keeps the slot until the whole line is written or stdin
// fails, so an abandoned write never interleaves with the next request.
// Trade-off: a write abandoned mid-line keeps its goroutine and the slot
// until the helper reads again, exits, or Close closes stdin (closing the
// pipe wakes the blocked write). Later calls fail with their own ctx in the
// meantime. Close never takes the slot, so a stuck write cannot hold it
// past CloseTimeout, after which the helper is killed.
func (c *Client) send(ctx context.Context, line []byte) error {
	select {
	case c.writeSlot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	written := make(chan error, 1)
	go func() {
		defer func() { <-c.writeSlot }()
		_, err := c.stdin.Write(line)
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			return c.writeError(err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// writeError explains a failed write to stdin: the client was closed, or
// the helper is gone (or stopped reading, which is just as fatal).
func (c *Client) writeError(err error) error {
	c.mu.Lock()
	closing, exitErr := c.closing, c.err
	c.mu.Unlock()
	switch {
	case closing:
		return ErrClosed
	case exitErr != nil:
		return exitErr
	}
	return fmt.Errorf("%w: stdin: %v", ErrHelperExited, err)
}

func (c *Client) forget(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// readLoop is the only reader of stdout and the only sender on the states
// and errors channels, which it closes once the process is reaped.
func (c *Client) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	for scanner.Scan() {
		c.dispatch(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		// The stream is no longer decodable; the helper cannot be trusted.
		offer(c.errs, fmt.Errorf("helper: read output: %w", err))
		_ = c.cmd.Process.Kill()
		_, _ = io.Copy(io.Discard, stdout)
	}
	c.waitErr = c.cmd.Wait()

	status := "exited cleanly"
	if c.waitErr != nil {
		status = c.waitErr.Error()
	}
	exitErr := fmt.Errorf("%w (%s)%s", ErrHelperExited, status, tailSuffix(c.stderr.String()))

	c.mu.Lock()
	c.err = exitErr
	pending := c.pending
	c.pending = nil
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- reply{err: exitErr}
	}
	close(c.states)
	close(c.errs)
	close(c.done)
}

func (c *Client) dispatch(line []byte) {
	if len(strings.TrimSpace(string(line))) == 0 {
		return
	}
	var m inbound
	if err := json.Unmarshal(line, &m); err != nil {
		offer(c.errs, fmt.Errorf("helper: malformed output %q: %w", line, err))
		return
	}
	switch {
	case m.Event == "ready":
		c.readyOnce.Do(func() { close(c.ready) })
	case m.Event == "state":
		if m.State != nil {
			offer(c.states, m.State.toDomain())
		}
	case m.Event == "error":
		offer(c.errs, fmt.Errorf("helper: %s", m.Message))
	case m.Event != "":
		// Unknown events are ignored for forward compatibility.
	case m.ID == "":
		offer(c.errs, fmt.Errorf("helper rejected a request: %s", m.Error))
	default:
		c.mu.Lock()
		ch, ok := c.pending[m.ID]
		delete(c.pending, m.ID)
		c.mu.Unlock()
		if ok { // otherwise the caller already gave up (ctx cancelled)
			ch <- reply{msg: m}
		}
	}
}

// offer sends v without blocking, dropping the oldest buffered values to
// make room. It must only be called from the single sending goroutine.
func offer[T any](ch chan T, v T) {
	for {
		select {
		case ch <- v:
			return
		default:
		}
		select {
		case <-ch:
		default:
		}
	}
}
