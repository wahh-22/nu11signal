//go:build !linux || android

package pulsecapture

import (
	"context"
	"errors"
	"testing"
)

func TestNonLinuxUnsupported(t *testing.T) {
	_, err := Start(context.Background(), func([]float32, int) {})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
}
