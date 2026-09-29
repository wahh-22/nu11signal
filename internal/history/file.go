package history

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// File stores recent terms as JSON at a path. A missing or unreadable-as-JSON
// file counts as empty; writes replace the file atomically.
type File struct {
	path string
	mu   sync.Mutex
}

// fileContent is the on-disk format.
type fileContent struct {
	Terms []string `json:"terms"`
}

// NewFile returns a File storing its terms at path. Nothing is read or
// created until Load or Add.
func NewFile(path string) *File { return &File{path: path} }

// DefaultPath is recent.json in the nu11signal directory of the user's
// config directory (os.UserConfigDir).
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "nu11signal", "recent.json"), nil
}

// Load returns the stored terms, most recent first.
func (f *File) Load() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.load()
}

// Add stores term as the most recent one.
func (f *File) Add(term string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	terms, err := f.load()
	if err != nil {
		return err
	}
	return f.write(Push(terms, term))
}

func (f *File) load() ([]string, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c fileContent
	if err := json.Unmarshal(data, &c); err != nil {
		// A corrupt file is not worth failing over: start a new list,
		// which the next Add writes over it.
		return nil, nil
	}
	return normalize(c.Terms), nil
}

// write replaces the file through a temporary file and a rename, so a crash
// never leaves a half-written list behind.
func (f *File) write(terms []string) error {
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(fileContent{Terms: terms})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".recent-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path)
}
