package update

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CachePath is update.json in the directory of the settings file at
// configPath (config.DefaultPath).
func CachePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "update.json")
}

// cacheFile is the content of update.json.
type cacheFile struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    Release   `json:"latest"`
}

// Cached is a Checker asking Inner on every call and keeping its last
// successful answer in a cache file, the answer when Inner fails (offline,
// a rate limit) whatever its age. A missing or corrupt file is no answer;
// a failed write is ignored (the next launch asks again).
type Cached struct {
	Inner Checker
	Path  string
}

// NewCached returns a Cached over inner with its file at path.
func NewCached(inner Checker, path string) *Cached {
	return &Cached{Inner: inner, Path: path}
}

// Latest asks Inner and caches a successful answer; when Inner fails, the
// cached answer is returned, and without one the error.
func (c *Cached) Latest(ctx context.Context) (Release, error) {
	rel, err := c.Inner.Latest(ctx)
	if err != nil {
		if cache, ok := c.read(); ok {
			return cache.Latest, nil
		}
		return Release{}, err
	}
	c.write(cacheFile{CheckedAt: time.Now(), Latest: rel})
	return rel, nil
}

// read loads the cache; ok is false for a missing, unreadable or corrupt
// file, or one without a valid version.
func (c *Cached) read() (cacheFile, bool) {
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return cacheFile{}, false
	}
	var f cacheFile
	if json.Unmarshal(data, &f) != nil || !Valid(f.Latest.Version) {
		return cacheFile{}, false
	}
	return f, true
}

// write stores f atomically, private (0600, the directory 0700): a
// temporary file in the directory renamed over the cache. Errors are
// dropped: the cache is an optimization.
func (c *Cached) write(f cacheFile) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(c.Path)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".update-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	defer os.Remove(name) // a no-op once renamed
	_, werr := tmp.Write(append(data, '\n'))
	if cerr := tmp.Close(); werr != nil || cerr != nil {
		return
	}
	if os.Chmod(name, 0o600) != nil {
		return
	}
	_ = os.Rename(name, c.Path)
}
