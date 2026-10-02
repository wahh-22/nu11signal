// Package config reads the user's settings file, config.json in the
// nu11signal directory of the user's config directory:
//
//	{"visualizer": "rain", "theme": "BLUE", "update_check": false,
//	 "music_dirs": ["~/Music", "/srv/music"]}
//
// The file is optional: a missing one is the defaults. nu11signal writes
// it only when a setting is changed in the app (Save), keeping the fields
// it does not know. Unknown fields are ignored; what a value means is the
// UI's business (the UI reads the theme, and reads the visualizer and
// ignores it: rain is the only one).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Config is the content of the settings file. A zero field is a setting
// the file leaves out.
type Config struct {
	// Visualizer names the NOW PLAYING visualizer, as written. It is
	// accepted for older files and ignored: the UI only has the rain.
	Visualizer string `json:"visualizer,omitempty"`
	// Theme names the UI's color theme, as SETTINGS lists it; the UI
	// falls back to its default for a name it does not know.
	Theme string `json:"theme,omitempty"`
	// UpdateCheck turns the launch check for a newer release off when
	// false; nil (left out) is on. See UpdateCheckOn.
	UpdateCheck *bool `json:"update_check,omitempty"`
	// MusicDirs are the folders scanned for local music files, as
	// written (a leading ~ is the home directory, resolved by the
	// command); nil (left out) is the command's default, ~/Music.
	MusicDirs []string `json:"music_dirs,omitempty"`
}

// UpdateCheckOn reports whether the settings allow the update check: on
// unless "update_check" is false.
func (c Config) UpdateCheckOn() bool { return c.UpdateCheck == nil || *c.UpdateCheck }

// Source loads and saves the settings.
type Source interface {
	// Load returns the settings. A missing file is no settings (the zero
	// Config) and no error; a file that cannot be read or parsed is an
	// error, with the zero Config.
	Load() (Config, error)
	// Save stores c in place of the settings, keeping what the store
	// holds that Config does not know.
	Save(c Config) error
}

// File is a Source reading and writing a JSON file at a path.
type File struct{ path string }

// NewFile returns a File at path. Nothing is read until Load, nor
// written until Save.
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

// Save writes c to the file (see Source.Save) atomically: into a temporary
// file in the same directory, then renamed over the file, so a crash
// leaves the old file or the new one, never half of one. The directory is
// created private (0700) if missing and the file is private (0600). The
// fields of the file Config does not know are kept, and a field c leaves
// empty is removed. A file that exists but is not a JSON object is left
// alone and is an error: overwriting it would lose what the user wrote.
func (f *File) Save(c Config) error {
	fields := map[string]json.RawMessage{}
	data, err := os.ReadFile(f.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &fields); err != nil {
			return fmt.Errorf("%s is not a settings file, left as it is: %w", filepath.Base(f.path), err)
		}
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
	}
	known, err := json.Marshal(c)
	if err != nil {
		return err
	}
	var set map[string]json.RawMessage
	if err := json.Unmarshal(known, &set); err != nil {
		return err
	}
	// Known fields are replaced, or removed when c leaves them empty;
	// update_check and music_dirs are only ever written by the user, so a
	// save that leaves them unset keeps what the file says.
	for _, name := range []string{"visualizer", "theme"} {
		delete(fields, name)
	}
	for k, v := range set {
		fields[k] = v
	}
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(f.path, append(out, '\n'))
}

// writeAtomic writes data to path (the file a symlink names, if it is
// one) through a private temporary file in its directory, renamed over
// it once complete.
func writeAtomic(path string, data []byte) error {
	// A linked file (a dotfiles setup) is updated where it lives, keeping
	// the link: the rename would replace the link with a plain file.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // a no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file 0600 already; say so in case of a umask.
	if err := os.Chmod(name, 0o600); err != nil {
		return err
	}
	return os.Rename(name, path)
}
