// Package androidtv connects the Go TUI to the Android TV app's music
// WebView and terminal pipes. The socket carries only JSON controls and
// evaluations; stdin/stdout carry terminal bytes through a Go-owned PTY.
package androidtv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	readings    chan webplayer.BridgeMessage
	mu          sync.Mutex // serializes resize with master Close
	sized       chan struct{}
	sizedOnce   sync.Once
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
	r := &Runtime{master: master, Slave: slave, loginClosed: make(chan struct{}), readings: make(chan webplayer.BridgeMessage, 1), sized: make(chan struct{})}
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
	case "levels":
		if len(m.Bands) != 0 && r.readings != nil {
			offerLatest(r.readings, m)
		}
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
			if r.sized != nil {
				r.sizedOnce.Do(func() { close(r.sized) })
			}
		}
	case "login":
		if m.Closed {
			r.loginOnce.Do(func() { close(r.loginClosed) })
		}
	}
}

// Sized is closed once the app has sent its terminal size. The PTY opens
// at 80x24, and a program started before the real size arrives can miss
// the SIGWINCH that announces it (its listener is not installed yet) and
// stay at 80x24; one started after reads the real size from the PTY.
func (r *Runtime) Sized() <-chan struct{} { return r.sized }

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
type Player struct {
	*webplayer.Player
	page      webplayer.Evaluator
	hideLogin func(context.Context, webplayer.BridgeMessage) error
	// SignIn and the composite recheck share this handoff. Neither may
	// publish authorization before the account's new document is ready.
	authMu                         sync.Mutex
	pending, refreshing, refreshed bool
	spectrumStates                 chan playback.State
	levels                         chan playback.Spectrum
	captureStop                    chan struct{}
	captureDone                    chan struct{}
	closeOnce                      sync.Once
	closeErr                       error
}

// Authorize refreshes the anonymous storefront once after late sign-in.
// The old-document marker prevents a poll racing navigation from publishing
// its stale authorized status. A new document reinstalls the webplayer script;
// its ready:false status keeps the composite pending for as long as needed.
func (p *Player) Authorize(ctx context.Context) (playback.AuthStatus, error) {
	p.authMu.Lock()
	defer p.authMu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if p.refreshing {
		raw, err := p.page.Evaluate(ctx, "Boolean(window.__nu11StorefrontReload)")
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, webplayer.ErrBrowserGone) {
				return "", err
			}
			return playback.AuthNotDetermined, nil
		}
		var old bool
		if json.Unmarshal(raw, &old) != nil || old {
			return playback.AuthNotDetermined, nil
		}
	}
	s, err := p.Player.Authorize(ctx)
	if err != nil {
		return s, err
	}
	if s != playback.AuthAuthorized {
		p.pending = true
		return s, nil
	}
	if p.pending && !p.refreshed && p.page != nil {
		// Return to the TUI immediately; Apple chooses the account storefront
		// on the root navigation, rather than retaining the anonymous /us page.
		if err := p.hideLogin(ctx, webplayer.BridgeMessage{Type: "login", Show: false}); err != nil {
			return "", err
		}
		_, err := p.page.Evaluate(ctx, `(() => { window.__nu11StorefrontReload = true; window.location.replace("https://music.apple.com/"); return true; })()`)
		if err != nil && !strings.Contains(err.Error(), "page navigated") {
			if ctx.Err() != nil || errors.Is(err, webplayer.ErrBrowserGone) {
				return "", err
			}
			// The command may not have reached the page. Retry on the next
			// poll, rather than accepting the still-old authorized document.
			return playback.AuthNotDetermined, nil
		}
		p.refreshing, p.refreshed = true, true
		return playback.AuthNotDetermined, nil
	}
	p.refreshing = false
	return s, nil
}

// AuthorizationHint names the TV app, not a desktop browser command.
func (p *Player) AuthorizationHint() string { return "relaunch nu11signal on the TV to sign in" }

// NewPlayer waits for MusicKit before returning the reusable web player.
// Readiness is bounded by webplayer.BridgeLoadTimeout as well as ctx.
func (r *Runtime) NewPlayer(ctx context.Context) (*Player, error) {
	p := webplayer.New(r.Bridge, webplayer.WithCatalogTimeout(CatalogTimeout))
	if err := webplayer.WaitReady(ctx, p); err != nil {
		p.Close()
		return nil, err
	}
	tv := &Player{Player: p, page: r.Bridge, hideLogin: r.Bridge.Send}
	tv.startSpectrum(p.States(), r.readings, r.Bridge.Send)
	return tv, nil
}

// SignIn shows the music WebView only if authorization is still needed,
// polls every second, and hides it after authorization. Back in the app
// ends this attempt without closing the player; later authorization can
// still be observed by the TUI. Each poll is bounded: a stalled WebView
// must not prevent the initial login control or end the sign-in attempt.
// ctx also cancels an in-flight poll.
func (r *Runtime) SignIn(ctx context.Context, p *Player) error {
	return coordinateLogin(ctx, p.loginStatus, p.sendLogin, r.loginClosed, time.Second)
}

// The sign-in view ends as soon as the account is signed in, even though
// the composite must wait longer for the refreshed document's MusicKit.
func (p *Player) loginStatus(ctx context.Context) (playback.AuthStatus, error) {
	s, err := p.Authorize(ctx)
	p.authMu.Lock()
	defer p.authMu.Unlock()
	if err == nil && p.refreshed {
		return playback.AuthAuthorized, nil
	}
	return s, err
}

func (p *Player) sendLogin(ctx context.Context, m webplayer.BridgeMessage) error {
	p.authMu.Lock()
	defer p.authMu.Unlock()
	// An initial SignIn poll can race the composite's completed sign-in.
	// Do not reopen the view after the handoff has already hidden it.
	if m.Show && p.refreshed {
		return nil
	}
	return p.hideLogin(ctx, m)
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
