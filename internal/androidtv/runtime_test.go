package androidtv

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

func privateSocket(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "app.sock")
}

func TestSocketPermissionsAndStale(t *testing.T) {
	path := privateSocket(t)
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	stale.Close()
	listener, err := listenSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	if _, err := listenSocket(path); err == nil {
		t.Fatal("replaced a live listener")
	}
}

func TestSocketRefusesUnsafePaths(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mode    os.FileMode
		regular bool
	}{
		{"group accessible", 0750, false}, {"world accessible", 0701, false}, {"regular file", 0700, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := privateSocket(t)
			os.Chmod(filepath.Dir(path), tt.mode)
			if tt.regular {
				if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if l, err := listenSocket(path); err == nil {
				l.Close()
				t.Fatal("unsafe socket path accepted")
			}
			if tt.regular {
				raw, _ := os.ReadFile(path)
				if string(raw) != "keep" {
					t.Fatal("regular file altered")
				}
			}
		})
	}
}

func TestAcceptOneAndTimeout(t *testing.T) {
	t.Run("one app", func(t *testing.T) {
		path := privateSocket(t)
		l, err := listenSocket(path)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		app, err := net.Dial("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		defer app.Close()
		conn, err := acceptApp(context.Background(), l, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if second, err := net.Dial("unix", path); err == nil {
			second.Close()
			t.Fatal("accepted a second app")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		l, err := listenSocket(privateSocket(t))
		if err != nil {
			t.Fatal(err)
		}
		if conn, err := acceptApp(context.Background(), l, 10*time.Millisecond); err == nil {
			conn.Close()
			t.Fatal("accept did not time out")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		l, err := listenSocket(privateSocket(t))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := acceptApp(ctx, l, time.Second); !errors.Is(err, context.Canceled) {
			t.Fatalf("accept = %v", err)
		}
	})
}

func TestSignIn(t *testing.T) {
	for _, tt := range []struct {
		name          string
		authorizeAt   int
		close, cancel bool
		wantShows     []bool
	}{
		{"already authorized", 1, false, false, nil},
		{"after polls", 4, false, false, []bool{true, false}},
		{"user closed", 0, true, false, []bool{true}},
		{"context canceled", 0, false, true, []bool{true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			closed := make(chan struct{})
			calls := 0
			var shows []bool
			poll := func(context.Context) (playback.AuthStatus, error) {
				calls++
				if calls == tt.authorizeAt {
					return playback.AuthAuthorized, nil
				}
				return playback.AuthNotDetermined, nil
			}
			send := func(_ context.Context, m webplayer.BridgeMessage) error {
				shows = append(shows, m.Show)
				if m.Show {
					if tt.close {
						close(closed)
					}
					if tt.cancel {
						cancel()
					}
				}
				return nil
			}
			err := coordinateLogin(ctx, poll, send, closed, time.Millisecond)
			if tt.cancel {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(shows) != len(tt.wantShows) {
				t.Fatalf("shows = %v; want %v", shows, tt.wantShows)
			}
			for i := range shows {
				if shows[i] != tt.wantShows[i] {
					t.Fatalf("shows = %v", shows)
				}
			}
			if tt.authorizeAt > 0 && calls != tt.authorizeAt {
				t.Fatalf("polls = %d", calls)
			}
		})
	}
}

func TestSignInPollTimeoutStillShowsLogin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	var shows []bool
	poll := func(ctx context.Context) (playback.AuthStatus, error) {
		if _, bounded := ctx.Deadline(); !bounded {
			t.Error("authorization poll has no deadline")
		}
		calls++
		if calls <= 2 {
			return "", context.DeadlineExceeded
		}
		return playback.AuthAuthorized, nil
	}
	err := coordinateLogin(ctx, poll, func(_ context.Context, m webplayer.BridgeMessage) error {
		shows = append(shows, m.Show)
		return nil
	}, make(chan struct{}), time.Millisecond)
	if err != nil || len(shows) != 2 || !shows[0] || shows[1] {
		t.Fatalf("shows = %v, err = %v", shows, err)
	}
}

func TestSignInBridgeControls(t *testing.T) {
	for _, userClosed := range []bool{false, true} {
		t.Run(map[bool]string{false: "authorized", true: "login closed"}[userClosed], func(t *testing.T) {
			goConn, app := net.Pipe()
			defer app.Close()
			r := &Runtime{loginClosed: make(chan struct{})}
			r.Bridge = webplayer.NewBridge(goConn, r.onMessage)
			defer r.Bridge.Close()
			p := &Player{Player: webplayer.New(r.Bridge)}
			defer p.Close()
			controls := make(chan []bool, 1)
			go func() {
				decoder := json.NewDecoder(app)
				encoder := json.NewEncoder(app)
				var shows []bool
				polls := 0
				for {
					var m webplayer.BridgeMessage
					if decoder.Decode(&m) != nil {
						return
					}
					if m.Type == "login" {
						shows = append(shows, m.Show)
						if userClosed {
							encoder.Encode(map[string]any{"type": "login", "closed": true})
							controls <- shows
						} else if !m.Show {
							controls <- shows
						}
						continue
					}
					var value any = map[string]any{"installed": true}
					if strings.Contains(m.Expr, "status") && !strings.Contains(m.Expr, "installed") {
						polls++
						value = map[string]any{"value": map[string]any{"ready": true, "authorized": !userClosed && polls >= 3}}
					}
					if encoder.Encode(map[string]any{"type": "result", "id": m.ID, "ok": true, "value": value}) != nil {
						return
					}
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := coordinateLogin(ctx, p.Authorize, r.Bridge.Send, r.loginClosed, time.Millisecond); err != nil {
				t.Fatal(err)
			}
			select {
			case shows := <-controls:
				want := 2
				if userClosed {
					want = 1
				}
				if len(shows) != want || !shows[0] || (want == 2 && shows[1]) {
					t.Fatalf("controls = %v", shows)
				}
			case <-ctx.Done():
				t.Fatal("missing bridge controls")
			}
		})
	}
}

func TestSignInClosedDuringPoll(t *testing.T) {
	closed := make(chan struct{})
	calls := 0
	poll := func(ctx context.Context) (playback.AuthStatus, error) {
		calls++
		if calls == 1 {
			return playback.AuthNotDetermined, nil
		}
		close(closed)
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := coordinateLogin(ctx, poll, func(context.Context, webplayer.BridgeMessage) error { return nil }, closed, time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestAndroidAuthorizationHint(t *testing.T) {
	p := &Player{Player: webplayer.New(nil)}
	defer p.Close()
	hint := p.AuthorizationHint()
	if !strings.Contains(hint, "TV") || strings.Contains(hint, "--apple-music-login") {
		t.Fatalf("hint = %q", hint)
	}
}

// A disconnected app must retain the bridge's sentinel, not a raw Unix
// transport error; this is also the shutdown signal used by the TUI.
func TestSocketDisconnect(t *testing.T) {
	path := privateSocket(t)
	l, err := listenSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	app, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := acceptApp(context.Background(), l, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	b := webplayer.NewBridge(conn, nil)
	defer b.Close()
	app.Close()
	select {
	case <-b.Done():
	case <-time.After(time.Second):
		t.Fatal("disconnect not observed")
	}
	if _, err := b.Evaluate(context.Background(), "1"); !errors.Is(err, webplayer.ErrBrowserGone) {
		t.Fatal(err)
	}
}
