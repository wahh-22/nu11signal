// Package androidtv connects the Go TUI to the Android TV app's music
// WebView and terminal pipes. The socket carries only JSON controls and
// evaluations; stdin/stdout carry terminal bytes through a Go-owned PTY.
package androidtv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

// ErrUnsupported reports that the Go-owned PTY requires Linux or Android.
var ErrUnsupported = errors.New("androidtv: PTY requires Linux or Android")

const acceptTimeout = 30 * time.Second

// Runtime owns one app connection and a PTY. Slave is the TUI's real
// terminal; Relay moves its bytes to and from the process pipes. Close
// releases both ends and the bridge, and is safe after any startup error.
// The player must be closed before the runtime, so its stop can reach the
// page. A bridge disconnect should cancel the TUI (see Bridge.Done).
type Runtime struct {
	Bridge      *webplayer.Bridge
	Slave       *os.File
	master      *os.File
	loginClosed chan struct{}
	loginOnce   sync.Once
	closeOnce   sync.Once
	mu          sync.Mutex // serializes resize with master Close
}

// Open accepts exactly one app within 30 seconds (or ctx), after opening
// the PTY. The socket's parent must be private; a stale socket is removed
// but a live listener, regular file or symlink is never replaced. The
// listening socket is 0600 and is closed and unlinked after acceptance.
func Open(ctx context.Context, path string) (*Runtime, error) {
	master, slave, err := openPTY()
	if err != nil {
		return nil, err
	}
	r := &Runtime{master: master, Slave: slave, loginClosed: make(chan struct{})}
	listener, err := listenSocket(path)
	if err != nil {
		r.Close()
		return nil, err
	}
	conn, err := acceptApp(ctx, listener, acceptTimeout)
	if err != nil {
		r.Close()
		return nil, err
	}
	r.Bridge = webplayer.NewBridge(conn, r.onMessage)
	return r, nil
}

func listenSocket(path string) (*net.UnixListener, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("androidtv: socket path must be absolute")
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("androidtv: socket directory: %w", err)
	}
	if !parent.IsDir() || parent.Mode().Perm()&0077 != 0 {
		return nil, errors.New("androidtv: socket directory must not be group/world accessible")
	}
	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("androidtv: refusing to replace a non-socket file")
		}
		conn, dialErr := net.DialTimeout("unix", path, 100*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return nil, errors.New("androidtv: socket is already in use")
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("androidtv: checking stale socket: %w", dialErr)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("androidtv: remove stale socket: %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("androidtv: socket path: %w", err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("androidtv: listen: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("androidtv: socket permissions: %w", err)
	}
	return listener, nil
}

func acceptApp(ctx context.Context, l *net.UnixListener, timeout time.Duration) (net.Conn, error) {
	defer l.Close()
	stop := context.AfterFunc(ctx, func() { l.Close() })
	defer stop()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := l.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	conn, err := l.AcceptUnix()
	if ctx.Err() != nil {
		if conn != nil {
			conn.Close()
		}
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("androidtv: waiting for app: %w", err)
	}
	return conn, nil
}

func (r *Runtime) onMessage(m webplayer.BridgeMessage) {
	switch m.Type {
	case "resize":
		r.mu.Lock()
		defer r.mu.Unlock()
		// A bogus size must not wrap to uint16 or collapse the terminal.
		if m.Cols > 0 && m.Cols <= 65535 && m.Rows > 0 && m.Rows <= 65535 {
			if resizePTY(r.master, m.Cols, m.Rows) == nil {
				// This PTY is not our controlling terminal, so the kernel
				// has no foreground process group to notify for us.
				notifyResize()
			}
		}
	case "login":
		if m.Closed {
			r.loginOnce.Do(func() { close(r.loginClosed) })
		}
	}
}

// Close is idempotent. It unblocks bridge calls and PTY relays; it does
// not close process stdin/stdout, which belong to the caller.
func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {
		if r.Bridge != nil {
			r.Bridge.Close()
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.Slave.Close()
		r.master.Close()
	})
	return nil
}

// Relay runs until ctx ends or either process pipe fails. The input
// relay polls with cancellation, rather than leaving a goroutine blocked
// on process stdin after the UI exits. The caller must Close the runtime
// to stop a blocked PTY read/write, and wait for the returned channel.
// Process-pipe output is polled with cancellation too. An arbitrary
// output writer must itself unblock when closed by its owner.
func (r *Runtime) Relay(ctx context.Context, input *os.File, output io.Writer) <-chan error {
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		inputDone := make(chan error, 1)
		go func() { inputDone <- relayInput(ctx, input, r.master) }()
		outputDone := make(chan error, 1)
		go func() { outputDone <- relayOutput(ctx, r.master, output) }()
		var err error
		select {
		case err = <-inputDone:
			cancel()
			r.Close()
			<-outputDone
		case err = <-outputDone:
			cancel()
			r.Close()
			<-inputDone
		case <-ctx.Done():
			err = ctx.Err()
			r.Close()
			<-inputDone
			<-outputDone
		}
		done <- err
	}()
	return done
}

// Player keeps all webplayer optional capabilities while replacing its
// desktop sign-in instruction.
type Player struct{ *webplayer.Player }

// AuthorizationHint names the TV app, not a desktop browser command.
func (p *Player) AuthorizationHint() string { return "relaunch nu11signal on the TV to sign in" }

// NewPlayer waits for MusicKit before returning the reusable web player.
// Readiness is bounded by webplayer.BridgeLoadTimeout as well as ctx.
func (r *Runtime) NewPlayer(ctx context.Context) (*Player, error) {
	p := webplayer.New(r.Bridge)
	if err := webplayer.WaitReady(ctx, p); err != nil {
		p.Close()
		return nil, err
	}
	return &Player{Player: p}, nil
}

// SignIn shows the music WebView only if authorization is still needed,
// polls every second, and hides it after authorization. Back in the app
// ends this attempt without closing the player; later authorization can
// still be observed by the TUI. Each poll is bounded: a stalled WebView
// must not prevent the initial login control or end the sign-in attempt.
// ctx also cancels an in-flight poll.
func (r *Runtime) SignIn(ctx context.Context, p *Player) error {
	return coordinateLogin(ctx, p.Authorize, r.Bridge.Send, r.loginClosed, time.Second)
}

func coordinateLogin(ctx context.Context, poll func(context.Context) (playback.AuthStatus, error), send func(context.Context, webplayer.BridgeMessage) error, closed <-chan struct{}, every time.Duration) error {
	wait, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-closed:
			cancel()
		case <-wait.Done():
		}
	}()
	stopped := func(err error) error {
		select {
		case <-closed:
			return nil
		default:
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	pollBounded := func() (playback.AuthStatus, error) {
		ctx, cancel := context.WithTimeout(wait, 5*time.Second)
		defer cancel()
		return poll(ctx)
	}
	auth, err := pollBounded()
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return stopped(err)
	}
	if auth == playback.AuthAuthorized {
		return nil
	}
	if wait.Err() != nil {
		return stopped(wait.Err())
	}
	if err := send(wait, webplayer.BridgeMessage{Type: "login", Show: true}); err != nil {
		return stopped(err)
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-wait.Done():
			return stopped(wait.Err())
		case <-ticker.C:
		}
		auth, err := pollBounded()
		if errors.Is(err, context.DeadlineExceeded) && wait.Err() == nil {
			continue
		}
		if err != nil {
			return stopped(err)
		}
		if auth == playback.AuthAuthorized {
			return stopped(send(wait, webplayer.BridgeMessage{Type: "login", Show: false}))
		}
	}
}
