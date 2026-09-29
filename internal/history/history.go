// Package history remembers recent catalog searches: the Recents port, an
// in-memory adapter and a JSON file adapter.
package history

import (
	"strings"
	"sync"
)

// Max is how many recent terms are kept.
const Max = 10

// Recents is the port through which the UI keeps recent search terms.
// Load returns them most recent first; Add records one as the most recent;
// Remove forgets one (matched as Push dedupes: trimmed, ignoring case) and
// Clear forgets them all. Implementations are safe for concurrent use.
type Recents interface {
	Load() ([]string, error)
	Add(term string) error
	Remove(term string) error
	Clear() error
}

// Push returns list with term moved to the front: trimmed, deduplicated
// case-insensitively and capped at Max. A blank term leaves list as is.
// list itself is never modified.
func Push(list []string, term string) []string {
	term = strings.TrimSpace(term)
	if term == "" {
		return list
	}
	out := make([]string, 0, min(len(list)+1, Max))
	out = append(out, term)
	for _, t := range list {
		if len(out) == Max {
			break
		}
		if !strings.EqualFold(t, term) {
			out = append(out, t)
		}
	}
	return out
}

// Without returns list without term, matched as Push dedupes: trimmed and
// ignoring case. list itself is never modified.
func Without(list []string, term string) []string {
	term = strings.TrimSpace(term)
	out := make([]string, 0, len(list))
	for _, t := range list {
		if !strings.EqualFold(t, term) {
			out = append(out, t)
		}
	}
	return out
}

// normalize applies Push's rules to a stored list, keeping its order.
func normalize(list []string) []string {
	var out []string
	for i := len(list) - 1; i >= 0; i-- {
		out = Push(out, list[i])
	}
	return out
}

// Memory keeps recent terms for the life of the process only.
type Memory struct {
	mu    sync.Mutex
	terms []string
}

// NewMemory returns an empty Memory.
func NewMemory() *Memory { return &Memory{} }

// Load returns a copy of the kept terms.
func (m *Memory) Load() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.terms...), nil
}

// Add keeps term as the most recent one.
func (m *Memory) Add(term string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.terms = Push(m.terms, term)
	return nil
}

// Remove forgets term.
func (m *Memory) Remove(term string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.terms = Without(m.terms, term)
	return nil
}

// Clear forgets every term.
func (m *Memory) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.terms = nil
	return nil
}
