// Package demo is an in-process playback.Player with a fictional library.
// It simulates playback progress so the UI can be exercised without Apple
// Music or the helper; it makes no sound.
package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// ErrClosed is returned by calls made after Close.
var ErrClosed = errors.New("demo player closed")

// ErrEmptyTerm is returned by SearchCatalog for a blank term, as the
// helper rejects one.
var ErrEmptyTerm = errors.New("search term is empty")

// Options configures New.
type Options struct {
	// Tick is how often playback advances and a state is emitted
	// (default 500ms). Playback advances in real time.
	Tick time.Duration
}

const stateBuffer = 16

// maxSearchLimit is the helper's per-type cap on catalog search results.
const maxSearchLimit = 25

// Player is the demo playback.Player. Methods are safe for concurrent use.
type Player struct {
	tick time.Duration
	done chan struct{}
	wg   sync.WaitGroup

	mu     sync.Mutex
	closed bool
	states chan playback.State
	errs   chan error
	queue  []playback.Song
	index  int
	status playback.Status
	pos    time.Duration
}

// New starts a demo player. Close stops it.
func New(opts Options) *Player {
	if opts.Tick <= 0 {
		opts.Tick = 500 * time.Millisecond
	}
	p := &Player{
		tick:   opts.Tick,
		done:   make(chan struct{}),
		states: make(chan playback.State, stateBuffer),
		errs:   make(chan error, 1),
		status: playback.StatusStopped,
	}
	p.wg.Add(1)
	go p.run()
	return p
}

func (p *Player) run() {
	defer p.wg.Done()
	t := time.NewTicker(p.tick)
	defer t.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-t.C:
			p.advance()
		}
	}
}

// advance moves the playhead by one tick, rolling over to the next track.
func (p *Player) advance() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.status != playback.StatusPlaying || len(p.queue) == 0 {
		return
	}
	p.pos += p.tick
	if p.pos >= p.queue[p.index].Duration {
		p.index = (p.index + 1) % len(p.queue)
		p.pos = 0
	}
	p.emitLocked()
}

// emitLocked publishes the current state, dropping the oldest unread state
// when the buffer is full so the newest one always gets through. All sends
// happen under mu, so the drain-then-send cannot block.
func (p *Player) emitLocked() {
	s := playback.State{Status: p.status, Position: p.pos}
	if len(p.queue) > 0 {
		song := p.queue[p.index]
		s.Title, s.Artist, s.Album, s.SongID, s.Duration = song.Title, song.Artist, song.Album, song.ID, song.Duration
	}
	select {
	case p.states <- s:
		return
	default:
	}
	select {
	case <-p.states:
	default:
	}
	p.states <- s
}

// do runs fn under the lock after checking ctx and Close, then emits state
// when fn succeeds and emit is set.
func (p *Player) do(ctx context.Context, emit bool, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	if err := fn(); err != nil {
		return err
	}
	if emit {
		p.emitLocked()
	}
	return nil
}

// Authorize always succeeds.
func (p *Player) Authorize(ctx context.Context) (playback.AuthStatus, error) {
	if err := p.do(ctx, false, func() error { return nil }); err != nil {
		return "", err
	}
	return playback.AuthAuthorized, nil
}

// SearchCatalog matches term case-insensitively: artists by name, songs by
// title, artist or album, and suggestions from the matching artist names and song
// titles. Like the helper, it rejects a blank term (ErrEmptyTerm) and
// clamps limit, which caps each list separately, to 1...25.
func (p *Player) SearchCatalog(ctx context.Context, term string, limit int) (playback.SearchResults, error) {
	var res playback.SearchResults
	term = strings.TrimSpace(term)
	if term == "" {
		return res, ErrEmptyTerm
	}
	limit = min(max(limit, 1), maxSearchLimit)
	err := p.do(ctx, false, func() error {
		needle := strings.ToLower(term)
		seen := map[string]bool{}
		suggest := func(s string) {
			s = strings.ToLower(s)
			if len(res.Suggestions) < limit && strings.Contains(s, needle) && !seen[s] {
				seen[s] = true
				res.Suggestions = append(res.Suggestions, s)
			}
		}
		for _, a := range artists {
			if len(res.Artists) < limit && strings.Contains(strings.ToLower(a.Name), needle) {
				res.Artists = append(res.Artists, a)
				suggest(a.Name)
			}
		}
		for _, s := range catalog {
			if len(res.Songs) >= limit {
				break
			}
			hay := strings.ToLower(s.Title + " " + s.Artist + " " + s.Album)
			if strings.Contains(hay, needle) {
				res.Songs = append(res.Songs, s)
				suggest(s.Title)
			}
		}
		return nil
	})
	return res, err
}

// Playlists returns the demo stations.
func (p *Player) Playlists(ctx context.Context) ([]playback.Playlist, error) {
	var out []playback.Playlist
	err := p.do(ctx, false, func() error {
		for _, s := range stations {
			out = append(out, s.Playlist)
		}
		return nil
	})
	return out, err
}

// PlaySongs queues the given catalog songs and plays from start.
func (p *Player) PlaySongs(ctx context.Context, ids []string, start int) error {
	return p.do(ctx, true, func() error {
		queue := make([]playback.Song, 0, len(ids))
		for _, id := range ids {
			s, ok := songByID(id)
			if !ok {
				return fmt.Errorf("demo: unknown song %q", id)
			}
			queue = append(queue, s)
		}
		if start < 0 || start >= len(queue) {
			return fmt.Errorf("demo: start %d out of range", start)
		}
		p.playLocked(queue, start)
		return nil
	})
}

// PlayPlaylist plays a demo station from its first track.
func (p *Player) PlayPlaylist(ctx context.Context, id string) error {
	return p.do(ctx, true, func() error {
		for _, s := range stations {
			if s.ID == id {
				p.playLocked(s.songs(), 0)
				return nil
			}
		}
		return fmt.Errorf("demo: unknown playlist %q", id)
	})
}

func (p *Player) playLocked(queue []playback.Song, start int) {
	p.queue, p.index, p.pos, p.status = queue, start, 0, playback.StatusPlaying
}

// Pause pauses playback.
func (p *Player) Pause(ctx context.Context) error {
	return p.do(ctx, true, func() error {
		if p.status == playback.StatusPlaying {
			p.status = playback.StatusPaused
		}
		return nil
	})
}

// Resume resumes the current queue.
func (p *Player) Resume(ctx context.Context) error {
	return p.do(ctx, true, func() error {
		if len(p.queue) == 0 {
			return errors.New("demo: nothing queued")
		}
		p.status = playback.StatusPlaying
		return nil
	})
}

// Next skips to the next track, wrapping around the queue.
func (p *Player) Next(ctx context.Context) error {
	return p.do(ctx, true, func() error {
		if len(p.queue) == 0 {
			return errors.New("demo: nothing queued")
		}
		p.index, p.pos = (p.index+1)%len(p.queue), 0
		return nil
	})
}

// Previous restarts the track, or goes back one when near its start.
func (p *Player) Previous(ctx context.Context) error {
	return p.do(ctx, true, func() error {
		if len(p.queue) == 0 {
			return errors.New("demo: nothing queued")
		}
		if p.pos < 3*time.Second {
			p.index = (p.index - 1 + len(p.queue)) % len(p.queue)
		}
		p.pos = 0
		return nil
	})
}

// Stop stops playback and rewinds.
func (p *Player) Stop(ctx context.Context) error {
	return p.do(ctx, true, func() error {
		p.status, p.pos = playback.StatusStopped, 0
		return nil
	})
}

// Seek moves the playhead, clamped to the current track.
func (p *Player) Seek(ctx context.Context, position time.Duration) error {
	return p.do(ctx, true, func() error {
		if len(p.queue) == 0 {
			return errors.New("demo: nothing queued")
		}
		p.pos = max(0, min(position, p.queue[p.index].Duration))
		return nil
	})
}

// States delivers state snapshots; it is closed by Close.
func (p *Player) States() <-chan playback.State { return p.states }

// Errors never delivers errors; it is closed by Close.
func (p *Player) Errors() <-chan error { return p.errs }

// Close stops the simulation and closes both channels. It is idempotent.
func (p *Player) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.states)
	close(p.errs)
	p.mu.Unlock()
	close(p.done)
	p.wg.Wait()
	return nil
}
