//go:build linux || android

package androidtv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// openPTY uses the kernel's Unix98 allocator, rather than a platform
// helper or a third-party PTY package. Nonblocking descriptors let Go's
// poller interrupt PTY reads and writes when Runtime.Close closes them.
func openPTY() (_ *os.File, _ *os.File, err error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("androidtv: open PTY: %w", err)
	}
	master := os.NewFile(uintptr(fd), "/dev/ptmx")
	var slave *os.File
	defer func() {
		if err != nil {
			master.Close()
			if slave != nil {
				slave.Close()
			}
		}
	}()
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return nil, nil, fmt.Errorf("androidtv: unlock PTY: %w", err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		return nil, nil, fmt.Errorf("androidtv: PTY number: %w", err)
	}
	path := fmt.Sprintf("/dev/pts/%d", number)
	slaveFD, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("androidtv: open PTY slave: %w", err)
	}
	slave = os.NewFile(uintptr(slaveFD), path)
	if err = resizePTY(master, 80, 24); err != nil {
		return nil, nil, err
	}
	return master, slave, nil
}

// withPTYFD retains Go's nonblocking descriptor and coordinates with
// Close; File.Fd would instead put the descriptor into blocking mode.
func withPTYFD(master *os.File, fn func(int) error) error {
	raw, err := master.SyscallConn()
	if err != nil {
		return err
	}
	var ioctlErr error
	if err := raw.Control(func(fd uintptr) { ioctlErr = fn(int(fd)) }); err != nil {
		return err
	}
	return ioctlErr
}

func resizePTY(master *os.File, cols, rows int) error {
	return withPTYFD(master, func(fd int) error {
		return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(cols), Row: uint16(rows)})
	})
}

func ptySize(master *os.File) (int, int, error) {
	var size *unix.Winsize
	err := withPTYFD(master, func(fd int) error { var err error; size, err = unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ); return err })
	if err != nil {
		return 0, 0, err
	}
	return int(size.Col), int(size.Row), nil
}

// relayInput leaves the process descriptor open. Poll has a short bound
// so cancellation is observed even when the app sends no more input.
// The app is the sole writer and this is the sole reader of its pipe.
func relayInput(ctx context.Context, input, master *os.File) error {
	fd := int(input.Fd())
	descriptors := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := unix.Poll(descriptors, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		if descriptors[0].Revents&unix.POLLNVAL != 0 {
			return os.ErrClosed
		}
		count, err := unix.Read(fd, buffer)
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			return err
		}
		if count == 0 {
			return io.EOF
		}
		if _, err := master.Write(buffer[:count]); err != nil {
			return err
		}
	}
}
