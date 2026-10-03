package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The stamp is the version between a fixed prefix and terminator, so a
// byte search for it cannot match another version (v0.7.9, 10.7.9, 0.7.99).
func TestStampFor(t *testing.T) {
	if got, want := stampFor("0.7.9"), "nu11signal-version:0.7.9;"; got != want {
		t.Fatalf("stampFor(0.7.9) = %q; want %q", got, want)
	}
}

// Unstamped builds carry no stamp and stay quiet about it.
func TestVersionStampDefaultsToEmpty(t *testing.T) {
	if versionStamp != "" {
		t.Fatalf("versionStamp = %q; want empty for an unstamped test build", versionStamp)
	}
}

// --version prints the same line whatever the stamp; a stamp that names
// another version is reported on stderr only.
func TestRunVersionReportsAMismatchedStamp(t *testing.T) {
	saved := versionStamp
	defer func() { versionStamp = saved }()
	for _, tt := range []struct {
		stamp string
		warn  bool
	}{
		{"", false},
		{stampFor(version), false},
		{stampFor("v" + version), true},
		{stampFor(version + "1"), true},
	} {
		versionStamp = tt.stamp
		var out, errOut bytes.Buffer
		if code := run([]string{"--version"}, deps{stdout: &out, stderr: &errOut}); code != 0 {
			t.Fatalf("stamp %q: run(--version) = %d; want 0", tt.stamp, code)
		}
		if out.String() != version+"\n" {
			t.Errorf("stamp %q: run(--version) wrote %q; want the bare version", tt.stamp, out.String())
		}
		if got := errOut.Len() > 0; got != tt.warn {
			t.Errorf("stamp %q: stderr %q; want a warning: %v", tt.stamp, errOut.String(), tt.warn)
		}
	}
}

// A stripped release build (-s -w, as scripts/release.sh links it) keeps
// the stamp's bytes: release.sh looks for them in Linux binaries it cannot
// run, and the linker drops variables no reachable code reads.
func TestStrippedBuildKeepsTheVersionStamp(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the command")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go tool on PATH")
	}
	out := filepath.Join(t.TempDir(), "nu11signal")
	stamp := stampFor("0.7.9")
	cmd := exec.Command(goTool, "build", "-trimpath",
		"-ldflags", "-s -w -X main.version=0.7.9 -X main.versionStamp="+stamp,
		"-o", out, ".")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	bin, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(bin, []byte(stamp)); n == 0 {
		t.Fatalf("stripped build does not contain %q", stamp)
	}
}
