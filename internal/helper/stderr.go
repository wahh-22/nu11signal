package helper

import (
	"io"
	"strings"
	"sync"
)

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.max; over > 0 {
		b.buf = append([]byte(nil), b.buf[over:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// tailSuffix formats captured stderr for inclusion in an error message.
func tailSuffix(tail string) string {
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return ""
	}
	return "; stderr: " + tail
}

// ignoreErrors keeps a failing caller writer from stalling the helper's
// stderr pipe: its errors are swallowed and the copy continues.
type ignoreErrors struct{ w io.Writer }

func (i ignoreErrors) Write(p []byte) (int, error) {
	_, _ = i.w.Write(p)
	return len(p), nil
}
