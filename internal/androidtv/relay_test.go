package androidtv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestPTYRelayCancellation(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		t.Skip("PTY requires Linux or Android")
	}
	master, slave, err := openPTY()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{master: master, Slave: slave}
	defer r.Close()
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	var output bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := r.Relay(ctx, input, &output)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("relay = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("idle relay did not stop")
	}
}

func TestPTYRelayOutput(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		t.Skip("PTY requires Linux or Android")
	}
	master, slave, err := openPTY()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{master: master, Slave: slave}
	defer r.Close()
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	output, reader, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	defer reader.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := r.Relay(ctx, input, reader)
	if _, err := slave.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	output.SetReadDeadline(time.Now().Add(time.Second))
	buffer := make([]byte, 5)
	if n, err := output.Read(buffer); err != nil || string(buffer[:n]) != "hello" {
		t.Fatalf("output=%q, %v", buffer[:n], err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("relay did not stop")
	}
}
