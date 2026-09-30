package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Config
		wantErr bool
	}{
		{"a visualizer", `{"visualizer": "synthwave"}`, Config{Visualizer: "synthwave"}, false},
		{"the name as written", `{"visualizer": "SynthWave"}`, Config{Visualizer: "SynthWave"}, false},
		{"unknown fields are ignored", `{"visualizer": "rain", "theme": "neon"}`, Config{Visualizer: "rain"}, false},
		{"no visualizer", `{}`, Config{}, false},
		{"an empty file", ``, Config{}, true},
		{"not JSON", `visualizer = rain`, Config{}, true},
		{"the wrong type", `{"visualizer": 3}`, Config{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewFile(write(t, tt.content)).Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v; want error %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("Load() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadMissingFileIsTheDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nu11signal", "config.json")
	got, err := NewFile(path).Load()
	if err != nil || got != (Config{}) {
		t.Fatalf("Load() = %+v, %v; want the zero Config and no error", got, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Load created %s", path)
	}
}

func TestLoadUnreadableFileFails(t *testing.T) {
	// A directory where the file should be cannot be read as one.
	if _, err := NewFile(t.TempDir()).Load(); err == nil {
		t.Fatal("Load() of a directory succeeded")
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := os.UserConfigDir()
	if want := filepath.Join(dir, "nu11signal", "config.json"); path != want {
		t.Fatalf("DefaultPath() = %q; want %q", path, want)
	}
	if !strings.HasSuffix(path, filepath.Join("nu11signal", "config.json")) {
		t.Fatalf("DefaultPath() = %q", path)
	}
}
