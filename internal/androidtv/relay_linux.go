//go:build linux || android

package androidtv

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// relayOutput uses cancellable writes for the process pipe. A ready pipe
// has room for PIPE_BUF bytes (4096 on Linux/Android); writing at most
// that much cannot block while this relay is its only writer. Keeping
// each write small avoids a stuck stdout delaying shutdown indefinitely.
func relayOutput(ctx context.Context, master *os.File, output io.Writer) error {
	if file, ok := output.(*os.File); ok {
		output = &pipeWriter{ctx: ctx, fd: int(file.Fd())}
	}
	_, err := io.Copy(output, master)
	return err
}

type pipeWriter struct {
	ctx context.Context
	fd  int
}

func (w *pipeWriter) Write(data []byte) (int, error) {
	sent := 0
	descriptors := []unix.PollFd{{Fd: int32(w.fd), Events: unix.POLLOUT}}
	for len(data) > 0 {
		if err := w.ctx.Err(); err != nil {
			return sent, err
		}
		n, err := unix.Poll(descriptors, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return sent, err
		}
		if n == 0 {
			continue
		}
		if descriptors[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return sent, io.ErrClosedPipe
		}
		n, err = unix.Write(w.fd, data[:min(len(data), 4096)])
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			return sent, err
		}
		sent += n
		if n == 0 {
			return sent, io.ErrShortWrite
		}
		data = data[n:]
	}
	return sent, nil
}
