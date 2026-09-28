// Package playbacktest provides an in-memory playback.Player for tests.
package playbacktest

import (
	"context"
	"sync"
	"time"

	"github.com/wahh-22/soul-king/internal/playback"
)

// ChannelBuffer is how many states and errors the Fake buffers before
// PushState and PushError block.
const ChannelBuffer = 64

// Call is one recorded Player method invocation.
type Call struct {
	Method string
	Args   []any
}

// Fake is an in-memory playback.Player. Configure its exported fields before
// handing it to the code under test; methods are safe for concurrent use.
type Fake struct {
	AuthStatus      playback.AuthStatus
	SearchResult    []playback.Song
	PlaylistsResult []playback.Playlist
	// Err, when set, is returned by every method; MethodErr overrides it
	// per method name (for example "Search").
	Err       error
	MethodErr map[string]error

	mu     sync.Mutex
	calls  []Call
	closed bool
	states chan playback.State
	errs   chan error
}

// New returns a Fake that authorizes successfully and returns no results.
func New() *Fake {
	return &Fake{
		AuthStatus: playback.AuthAuthorized,
		states:     make(chan playback.State, ChannelBuffer),
		errs:       make(chan error, ChannelBuffer),
	}
}

// Calls returns a copy of the recorded calls, in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Closed reports whether Close was called.
func (f *Fake) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// PushState delivers s on States. It blocks when ChannelBuffer states are
// unread.
func (f *Fake) PushState(s playback.State) { f.states <- s }

// PushError delivers err on Errors. It blocks when ChannelBuffer errors are
// unread.
func (f *Fake) PushError(err error) { f.errs <- err }

func (f *Fake) record(method string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Method: method, Args: args})
	if err, ok := f.MethodErr[method]; ok {
		return err
	}
	return f.Err
}

func (f *Fake) Authorize(context.Context) (playback.AuthStatus, error) {
	if err := f.record("Authorize"); err != nil {
		return "", err
	}
	return f.AuthStatus, nil
}

func (f *Fake) Search(_ context.Context, term string, limit int) ([]playback.Song, error) {
	if err := f.record("Search", term, limit); err != nil {
		return nil, err
	}
	return f.SearchResult, nil
}

func (f *Fake) Playlists(context.Context) ([]playback.Playlist, error) {
	if err := f.record("Playlists"); err != nil {
		return nil, err
	}
	return f.PlaylistsResult, nil
}

func (f *Fake) PlaySongs(_ context.Context, ids []string, start int) error {
	return f.record("PlaySongs", append([]string(nil), ids...), start)
}

func (f *Fake) PlayPlaylist(_ context.Context, id string) error {
	return f.record("PlayPlaylist", id)
}

func (f *Fake) Pause(context.Context) error    { return f.record("Pause") }
func (f *Fake) Resume(context.Context) error   { return f.record("Resume") }
func (f *Fake) Next(context.Context) error     { return f.record("Next") }
func (f *Fake) Previous(context.Context) error { return f.record("Previous") }
func (f *Fake) Stop(context.Context) error     { return f.record("Stop") }

func (f *Fake) Seek(_ context.Context, position time.Duration) error {
	return f.record("Seek", position)
}

func (f *Fake) States() <-chan playback.State { return f.states }
func (f *Fake) Errors() <-chan error          { return f.errs }

// Close closes the States and Errors channels. It is idempotent; pushing
// after Close panics, as sending on a closed channel does.
func (f *Fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.states)
		close(f.errs)
	}
	return nil
}
