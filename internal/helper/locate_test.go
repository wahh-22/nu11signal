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

func TestLocateResolutionOrder(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "bin")
	workDir := filepath.Join(root, "work")
	envPath := filepath.Join(root, "custom", "soulking-helper")
	besideExe := filepath.Join(exeDir, bundleRelPath)
	inWorkDir := filepath.Join(workDir, "build", bundleRelPath)

	tests := []struct {
		name    string
		env     string
		create  []string
		want    string
		wantErr bool
	}{
		{"env var wins", envPath, []string{envPath, besideExe, inWorkDir}, envPath, false},
		{"bundle beside executable", "", []string{besideExe, inWorkDir}, besideExe, false},
		{"build dir in working directory", "", []string{inWorkDir}, inWorkDir, false},
		{"env var pointing nowhere fails", envPath, []string{besideExe}, "", true},
		{"nothing found", "", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.RemoveAll(root)
			for _, p := range tt.create {
				writeExecutable(t, p)
			}
			l := locator{
				getenv:     func(string) string { return tt.env },
				executable: func() (string, error) { return filepath.Join(exeDir, "soul-king"), nil },
				workDir:    func() (string, error) { return workDir, nil },
			}
			got, err := l.locate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("locate = %q; want error", got)
				}
				if !errors.Is(err, ErrHelperNotFound) {
					t.Fatalf("error %v is not ErrHelperNotFound", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("locate = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestLocateErrorListsPathsTried(t *testing.T) {
	root := t.TempDir()
	l := locator{
		getenv:     func(string) string { return "" },
		executable: func() (string, error) { return filepath.Join(root, "bin", "soul-king"), nil },
		workDir:    func() (string, error) { return root, nil },
	}
	_, err := l.locate()
	if err == nil {
		t.Fatal("want error")
	}
	for _, p := range []string{
		filepath.Join(root, "bin", bundleRelPath),
		filepath.Join(root, "build", bundleRelPath),
		HelperEnv,
	} {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q does not mention %q", err, p)
		}
	}
}
