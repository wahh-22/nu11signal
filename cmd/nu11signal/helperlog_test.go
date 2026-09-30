package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelperLogPathIsInTheUserLogs(t *testing.T) {
	got := helperLogPath("/Users/v")
	if want := "/Users/v/Library/Logs/nu11signal/helper.log"; got != want {
		t.Fatalf("helperLogPath = %q; want %q", got, want)
	}
}

func TestOpenHelperLogCreatesPrivateFileAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Logs", "nu11signal", "helper.log")
	for _, line := range []string{"first\n", "second\n"} {
		f, err := openHelperLog(path, 1<<20)
		if err != nil {
			t.Fatalf("openHelperLog: %v", err)
		}
		if _, err := f.WriteString(line); err != nil {
			t.Fatalf("write: %v", err)
		}
		f.Close()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "first\nsecond\n" {
		t.Fatalf("log = %q; want both sessions appended", data)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("log mode = %v, %v; want 0600", info.Mode().Perm(), err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("log dir mode = %v, %v; want 0700", info.Mode().Perm(), err)
	}
}

func TestOpenHelperLogRotatesAnOversizedLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helper.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 11)), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := openHelperLog(path, 10)
	if err != nil {
		t.Fatalf("openHelperLog: %v", err)
	}
	f.WriteString("fresh\n")
	f.Close()
	if data, _ := os.ReadFile(path); string(data) != "fresh\n" {
		t.Fatalf("log = %q; want a fresh file", data)
	}
	if old, err := os.ReadFile(path + ".1"); err != nil || string(old) != strings.Repeat("x", 11) {
		t.Fatalf("rotated log = %q, %v; want the old contents", old, err)
	}
	// A log within the limit is kept and appended to.
	f, err = openHelperLog(path, 10)
	if err != nil {
		t.Fatalf("openHelperLog: %v", err)
	}
	f.WriteString("more\n")
	f.Close()
	if data, _ := os.ReadFile(path); string(data) != "fresh\nmore\n" {
		t.Fatalf("log = %q; want appended", data)
	}
}
