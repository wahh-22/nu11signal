package radio

import (
	"sync"

	"github.com/wahh-22/nu11signal/internal/history"
)

// recentsWriter applies changes to the recent-searches store in the order
// Update made them. Commands run concurrently, so each change is queued
// when it is made (in Update, one message at a time) and its command
// applies every change still queued, oldest first, one command at a time:
// a delete issued after an add can never reach the store before it, and a
// command that never runs only leaves its change to the next one.
type recentsWriter struct {
	store history.Recents
	// mu guards queue; applying holds its own lock so that changes leave
	// the queue and reach the store in one piece.
	mu       sync.Mutex
	queue    []*recentsChange
	applying sync.Mutex
}

// recentsChange is one queued change; err is its outcome once applied.
type recentsChange struct {
	write func(history.Recents) error
	err   error
}

func newRecentsWriter(store history.Recents) *recentsWriter {
	return &recentsWriter{store: store}
}

// write queues a change and returns the function its command runs, which
// reports that change's outcome.
func (w *recentsWriter) write(change func(history.Recents) error) func() error {
	c := &recentsChange{write: change}
	w.mu.Lock()
	w.queue = append(w.queue, c)
	w.mu.Unlock()
	return func() error {
		w.flush()
		// Once flush returns, c has been applied, by this command or by
		// one that took the lock before it.
		return c.err
	}
}

// flush applies the queued changes in order.
func (w *recentsWriter) flush() {
	w.applying.Lock()
	defer w.applying.Unlock()
	w.mu.Lock()
	queue := w.queue
	w.queue = nil
	w.mu.Unlock()
	for _, c := range queue {
		c.err = c.write(w.store)
	}
}
