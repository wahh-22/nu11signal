//go:build !linux || android

package pulsecapture

func connect() (client, error) { return nil, ErrUnsupported }
