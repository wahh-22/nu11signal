package config

import (
	"encoding/json"
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
		{"unknown fields are ignored", `{"visualizer": "rain", "colors": "neon"}`, Config{Visualizer: "rain"}, false},
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

func TestLoadReadsTheTheme(t *testing.T) {
	got, err := NewFile(write(t, `{"visualizer": "rain", "theme": "BLUE"}`)).Load()
	if err != nil || got != (Config{Visualizer: "rain", Theme: "BLUE"}) {
		t.Fatalf("Load() = %+v, %v; want the visualizer and the theme", got, err)
	}
}

func TestSaveRoundTripsPrivately(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nu11signal")
	path := filepath.Join(dir, "config.json")
	f := NewFile(path)
	want := Config{Visualizer: "rain", Theme: "BLUE"}
	if err := f.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := f.Load()
	if err != nil || got != want {
		t.Fatalf("Load() after Save = %+v, %v; want %+v", got, err, want)
	}
	for p, mode := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != mode {
			t.Errorf("%s mode %o; want %o", p, got, mode)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("Save left %d files in %s: %v; want only config.json", len(entries), dir, entries)
	}
}

func TestSaveKeepsUnknownFields(t *testing.T) {
	path := write(t, `{"visualizer": "rain", "future": {"a": 1}, "theme": "NIGHT CITY"}`)
	if err := NewFile(path).Save(Config{Visualizer: "rain", Theme: "BLUE"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved file is not JSON: %v\n%s", err, data)
	}
	if raw["theme"] != "BLUE" || raw["visualizer"] != "rain" || raw["future"] == nil {
		t.Fatalf("saved %s; want the new theme, the visualizer and the unknown field", data)
	}
}

func TestSaveRefusesToOverwriteABrokenFile(t *testing.T) {
	path := write(t, `visualizer = rain`)
	if err := NewFile(path).Save(Config{Theme: "BLUE"}); err == nil {
		t.Fatal("Save over an unparsable file succeeded")
	}
	if data, _ := os.ReadFile(path); string(data) != `visualizer = rain` {
		t.Fatalf("Save changed the unparsable file to %q", data)
	}
}

func TestSaveWritesThroughASymlink(t *testing.T) {
	// A dotfiles setup links config.json elsewhere: Save updates the
	// linked file and keeps the link.
	target := write(t, `{"theme": "NIGHT CITY"}`)
	link := filepath.Join(t.TempDir(), "config.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := NewFile(link).Save(Config{Theme: "BLUE"}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Save replaced the symlink: %v, %v", info, err)
	}
	if got, err := NewFile(target).Load(); err != nil || got.Theme != "BLUE" {
		t.Fatalf("target after Save = %+v, %v; want theme BLUE", got, err)
	}
}

func TestUpdateCheck(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{`{}`, true},
		{`{"update_check": true}`, true},
		{`{"update_check": false}`, false},
	}
	for _, tt := range tests {
		c, err := NewFile(write(t, tt.content)).Load()
		if err != nil {
			t.Fatal(err)
		}
		if got := c.UpdateCheckOn(); got != tt.want {
			t.Errorf("%s: UpdateCheckOn() = %v; want %v", tt.content, got, tt.want)
		}
	}
}

func TestSaveKeepsAndRemovesUpdateCheck(t *testing.T) {
	path := write(t, `{"update_check": false, "colors": "neon"}`)
	f := NewFile(path)
	c, err := f.Load()
	if err != nil {
		t.Fatal(err)
	}
	c.Theme = "BLUE"
	if err := f.Save(c); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["update_check"] != false || got["theme"] != "BLUE" || got["colors"] != "neon" {
		t.Fatalf("saved %s; want update_check false kept beside theme and colors", data)
	}
	// A save that does not set it (SETTINGS saving a theme picked before
	// the file loaded) keeps the user's choice: only the user edits it.
	if err := f.Save(Config{Theme: "MATRIX"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), `"update_check": false`) {
		t.Fatalf("saved %s; want update_check false kept by a save that leaves it unset", data)
	}
}
