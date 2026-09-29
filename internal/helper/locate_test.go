package helper

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// realRoot returns a temporary directory with symlinks resolved (on macOS
// the temp dir lives behind /var -> /private/var), so expected paths match
// what Locate derives from the resolved executable.
func realRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLocateResolutionOrder(t *testing.T) {
	root := realRoot(t)
	exe := filepath.Join(root, "bin", "nu11signal")
	envPath := filepath.Join(root, "custom", "nu11signal-helper")
	besideExe := filepath.Join(root, "bin", bundleRelPath)
	libexec := filepath.Join(root, "libexec", bundleRelPath)
	devBuild := filepath.Join(root, "build", bundleRelPath)

	tests := []struct {
		name    string
		env     string
		create  []string
		want    string
		wantErr string
	}{
		{"env var wins", envPath, []string{envPath, besideExe, libexec, devBuild}, envPath, ""},
		{"bundle beside executable", "", []string{besideExe, libexec, devBuild}, besideExe, ""},
		{"homebrew libexec layout", "", []string{libexec, devBuild}, libexec, ""},
		{"dev build layout", "", []string{devBuild}, devBuild, ""},
		{"env var pointing nowhere fails", envPath, []string{besideExe}, "", "not an executable file"},
		{"relative env var is rejected", "build/" + bundleRelPath, []string{devBuild}, "", "absolute path"},
		{"env var pointing at a directory fails", filepath.Join(root, "bin"), []string{besideExe}, "", "not an executable file"},
		{"nothing found", "", nil, "", "tried"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, dir := range []string{"bin", "custom", "libexec", "build"} {
				_ = os.RemoveAll(filepath.Join(root, dir))
			}
			writeExecutable(t, exe)
			for _, p := range tt.create {
				writeExecutable(t, p)
			}
			l := locator{
				getenv:     func(string) string { return tt.env },
				executable: func() (string, error) { return exe, nil },
			}
			got, err := l.locate()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("locate = %q; want error", got)
				}
				if !errors.Is(err, ErrHelperNotFound) {
					t.Fatalf("error %v is not ErrHelperNotFound", err)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("locate = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// A helper planted in the working directory must never be picked up: only
// the explicit override and executable-relative locations are trusted.
func TestLocateIgnoresWorkingDirectory(t *testing.T) {
	root := realRoot(t)
	exe := writeExecutable(t, filepath.Join(root, "install", "bin", "nu11signal"))
	attacker := filepath.Join(root, "attacker")
	writeExecutable(t, filepath.Join(attacker, "build", bundleRelPath))
	writeExecutable(t, filepath.Join(attacker, bundleRelPath))
	t.Chdir(attacker)

	l := locator{
		getenv:     func(string) string { return "" },
		executable: func() (string, error) { return exe, nil },
	}
	got, err := l.locate()
	if err == nil {
		t.Fatalf("locate = %q; want not found (working directory is untrusted)", got)
	}
	if strings.Contains(err.Error(), attacker) {
		t.Fatalf("error %q mentions the working directory", err)
	}
}

func TestLocateResolvesExecutableSymlinks(t *testing.T) {
	root := realRoot(t)
	realExe := writeExecutable(t, filepath.Join(root, "repo", "bin", "nu11signal"))
	want := writeExecutable(t, filepath.Join(root, "repo", "build", bundleRelPath))
	link := filepath.Join(root, "usr", "local", "bin", "nu11signal")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realExe, link); err != nil {
		t.Fatal(err)
	}

	l := locator{
		getenv:     func(string) string { return "" },
		executable: func() (string, error) { return link, nil },
	}
	got, err := l.locate()
	if err != nil || got != want {
		t.Fatalf("locate = %q, %v; want %q", got, err, want)
	}
}

func TestLocateErrorListsPathsTried(t *testing.T) {
	root := realRoot(t)
	exe := writeExecutable(t, filepath.Join(root, "bin", "nu11signal"))
	l := locator{
		getenv:     func(string) string { return "" },
		executable: func() (string, error) { return exe, nil },
	}
	_, err := l.locate()
	if err == nil {
		t.Fatal("want error")
	}
	for _, p := range []string{
		filepath.Join(root, "bin", bundleRelPath),
		filepath.Join(root, "libexec", bundleRelPath),
		filepath.Join(root, "build", bundleRelPath),
		HelperEnv,
	} {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q does not mention %q", err, p)
		}
	}
}

func TestLocateFailsWhenExecutableIsUnknown(t *testing.T) {
	l := locator{
		getenv:     func(string) string { return "" },
		executable: func() (string, error) { return "", errors.New("no /proc") },
	}
	if _, err := l.locate(); !errors.Is(err, ErrHelperNotFound) {
		t.Fatalf("locate error = %v; want ErrHelperNotFound", err)
	}
}

// A Homebrew cask symlinks bin/nu11signal from the unpacked release archive
// into the prefix; the helper must still be found in the archive's
// ../libexec, relative to the real file rather than the symlink.
func TestLocateResolvesHomebrewCaskSymlink(t *testing.T) {
	root := realRoot(t)
	release := filepath.Join(root, "Caskroom", "nu11signal", "0.1.0", "nu11signal-0.1.0")
	realExe := writeExecutable(t, filepath.Join(release, "bin", "nu11signal"))
	want := writeExecutable(t, filepath.Join(release, "libexec", bundleRelPath))
	link := filepath.Join(root, "homebrew", "bin", "nu11signal")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realExe, link); err != nil {
		t.Fatal(err)
	}

	l := locator{
		getenv:     func(string) string { return "" },
		executable: func() (string, error) { return link, nil },
	}
	got, err := l.locate()
	if err != nil || got != want {
		t.Fatalf("locate = %q, %v; want %q", got, err, want)
	}
}
