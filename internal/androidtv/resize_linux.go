//go:build linux || android

package androidtv

import (
	"os"

	"golang.org/x/sys/unix"
)

// notifyResize wakes Bubble Tea's SIGWINCH listener. The PTY is opened
// with O_NOCTTY, so TIOCSWINSZ alone cannot signal this process; Bubble
// Tea then reads the new size from the PTY slave supplied as its output.
func notifyResize() { _ = unix.Kill(os.Getpid(), unix.SIGWINCH) }
