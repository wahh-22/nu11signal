//go:build !linux && !android

package androidtv

import (
	"context"
	"io"
	"os"
)

func openPTY() (*os.File, *os.File, error)                   { return nil, nil, ErrUnsupported }
func resizePTY(*os.File, int, int) error                     { return ErrUnsupported }
func ptySize(*os.File) (int, int, error)                     { return 0, 0, ErrUnsupported }
func notifyResize()                                          {}
func relayInput(context.Context, *os.File, *os.File) error   { return ErrUnsupported }
func relayOutput(context.Context, *os.File, io.Writer) error { return ErrUnsupported }
