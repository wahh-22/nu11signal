// Package config reads the user's settings file, config.json in the
// nu11signal directory of the user's config directory:
//
//	{"visualizer": "waterfall"}
//
// The file is optional and only read: nu11signal never creates or writes
// it. Unknown fields are ignored; what a value means is the UI's business.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Config is the content of the settings file. A zero field is a setting
// the file leaves out.
type Config struct {
	// Visualizer names the NOW PLAYING visualizer, as written.
	Visualizer string `json:"visualizer"`
}

// Source loads the settings.
type Source interface {
	// Load returns the settings. A missing file is no settings (the zero
	// Config) and no error; a file that cannot be read or parsed is an
	// error, with the zero Config.
	Load() (Config, error)
}

// File is a Source reading a JSON file at a path.
type File struct{ path string }

// NewFile returns a File reading path. Nothing is read until Load.
func NewFile(path string) *File { return &File{path: path} }

// DefaultPath is config.json in the nu11signal directory of the user's
// config directory (os.UserConfigDir).
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "nu11signal", "config.json"), nil
}

// Load reads the file (see Source.Load).
func (f *File) Load() (Config, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}
