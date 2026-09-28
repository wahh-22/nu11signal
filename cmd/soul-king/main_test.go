package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"testing"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want options
	}{
		{"no flags plays for real", nil, options{}},
		{"demo mode", []string{"--demo"}, options{demo: true}},
		{"version flag", []string{"--version"}, options{version: true}},
		{"single-dash version", []string{"-version"}, options{version: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFlags(tt.args, io.Discard)
			if err != nil || got != tt.want {
				t.Fatalf("parseFlags(%q) = %+v, %v; want %+v", tt.args, got, err, tt.want)
			}
		})
	}
}

func TestParseFlagsRejectsUnknownFlags(t *testing.T) {
	var usage bytes.Buffer
	if _, err := parseFlags([]string{"--nope"}, &usage); err == nil {
		t.Fatal("parseFlags(--nope) succeeded; want error")
	}
	if usage.Len() == 0 {
		t.Fatal("no usage printed for an unknown flag")
	}
}

func TestParseFlagsHelp(t *testing.T) {
	if _, err := parseFlags([]string{"-h"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseFlags(-h) error = %v; want flag.ErrHelp", err)
	}
}

// version is overridden at link time (-ldflags "-X main.version=x.y.z");
// unstamped builds must still say something sensible.
func TestVersionDefaultsToDev(t *testing.T) {
	if version != "dev" {
		t.Fatalf("version = %q; want %q for an unstamped test build", version, "dev")
	}
}

func TestPrintVersion(t *testing.T) {
	var out bytes.Buffer
	printVersion(&out)
	if got, want := out.String(), version+"\n"; got != want {
		t.Fatalf("printVersion wrote %q; want %q", got, want)
	}
}
