package update

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CacheTTL is how long a cached answer stands before Cached asks again.
const CacheTTL = 24 * time.Hour

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

// Cached is a Checker answering from a cache file while it is fresh
// (checked less than CacheTTL ago, and not in the future) and asking
// Inner otherwise, caching a successful answer. A missing or corrupt
// file is a stale one; a failed write is ignored (the next launch asks
// again).
type Cached struct {
	Inner Checker
	Path  string
	// Now is the clock; time.Now by default.
	Now func() time.Time
}

// NewCached returns a Cached over inner with its file at path.
func NewCached(inner Checker, path string) *Cached {
	return &Cached{Inner: inner, Path: path, Now: time.Now}
}

// Latest answers from the cache while fresh, else asks Inner; when Inner
// fails, a stale cached answer is still returned, and without one the
// error.
func (c *Cached) Latest(ctx context.Context) (Release, error) {
	now := c.Now()
	cache, ok := c.read()
	if ok {
		if age := now.Sub(cache.CheckedAt); age >= 0 && age < CacheTTL {
			return cache.Latest, nil
		}
	}
	rel, err := c.Inner.Latest(ctx)
	if err != nil {
		if ok {
			return cache.Latest, nil
		}
		return Release{}, err
	}
	c.write(cacheFile{CheckedAt: now, Latest: rel})
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
