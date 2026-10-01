package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"0.3.1", "0.3.0", true},
		{"v0.3.1", "0.3.0", true},
		{"0.4.0", "0.3.9", true},
		{"1.0.0", "0.99.99", true},
		{"0.10.0", "0.9.0", true}, // numeric, not lexical
		{"0.3.0", "0.3.0", false},
		{"0.2.9", "0.3.0", false},
		{"0.3.0", "0.3.1", false},
		{"0.3.0", "0.3.0-rc.1", true},     // the release of the prerelease you run
		{"0.3.1-rc.1", "0.3.0", false},    // never offer a prerelease
		{"0.3.1+build.7", "0.3.0", true},  // build metadata is ignored
		{"0.3.0", "0.3.0+build.7", false}, // build metadata is ignored
		{"0.3.1", "dev", false},
		{"0.3.1", "", false},
		{"", "0.3.0", false},
		{"0.3", "0.2.0", false},
		{"0.3.1.4", "0.3.0", false},
		{"0.3.x", "0.3.0", false},
		{"0.3.-1", "0.3.0", false},
		{"0.3.+1", "0.3.0", false},
		{"vv0.3.1", "0.3.0", false},
		{"0.3.1-", "0.3.0", false},
	}
	for _, tt := range tests {
		if got := Newer(tt.latest, tt.current); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v; want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestValid(t *testing.T) {
	for v, want := range map[string]bool{"0.3.0": true, "v1.2.3": true, "1.2.3-rc.1": true, "dev": false, "": false, "1.2": false} {
		if got := Valid(v); got != want {
			t.Errorf("Valid(%q) = %v; want %v", v, got, want)
		}
	}
}

// githubServer serves handler at the latest-release path and fails the
// test on any other path.
func githubServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/wahh-22/nu11signal/releases/latest" {
			t.Errorf("path = %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGitHubParsesTheLatestRelease(t *testing.T) {
	var gotAccept, gotAgent string
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAccept, gotAgent = r.Header.Get("Accept"), r.Header.Get("User-Agent")
		w.Write([]byte(`{"tag_name":"v0.3.1","html_url":"https://github.com/wahh-22/nu11signal/releases/tag/v0.3.1","name":"x"}`))
	})
	g := NewGitHub("0.3.0")
	g.BaseURL = srv.URL
	rel, err := g.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Release{Version: "0.3.1", URL: "https://github.com/wahh-22/nu11signal/releases/tag/v0.3.1"}
	if rel != want {
		t.Fatalf("release = %+v; want %+v", rel, want)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if gotAgent != "nu11signal/0.3.0" {
		t.Errorf("User-Agent = %q", gotAgent)
	}
}

func TestGitHubDefaults(t *testing.T) {
	g := NewGitHub("0.3.0")
	if g.BaseURL != "https://api.github.com" {
		t.Errorf("BaseURL = %q", g.BaseURL)
	}
	if g.Client == nil || g.Client.Timeout != 3*time.Second {
		t.Errorf("client timeout = %v; want 3s", g.Client)
	}
}

func TestGitHubMissingURLFallsBackToTheReleasesPage(t *testing.T) {
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"0.3.1"}`))
	})
	g := NewGitHub("0.3.0")
	g.BaseURL = srv.URL
	rel, err := g.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "0.3.1" || rel.URL != ReleasesURL {
		t.Fatalf("release = %+v", rel)
	}
}

func TestGitHubFailuresAreErrors(t *testing.T) {
	tests := map[string]http.HandlerFunc{
		"404":     func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
		"500":     func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) },
		"garbage": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>not json")) },
		"no tag":  func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"html_url":"x"}`)) },
		"bad tag": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"tag_name":"nightly"}`)) },
	}
	for name, h := range tests {
		t.Run(name, func(t *testing.T) {
			g := NewGitHub("0.3.0")
			g.BaseURL = githubServer(t, h).URL
			if rel, err := g.Latest(context.Background()); err == nil {
				t.Fatalf("got %+v, nil error", rel)
			}
		})
	}
}

func TestGitHubTimeoutIsAnError(t *testing.T) {
	release := make(chan struct{})
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	g := NewGitHub("0.3.0")
	g.BaseURL = srv.URL
	g.Client.Timeout = 50 * time.Millisecond
	if _, err := g.Latest(context.Background()); err == nil {
		t.Fatal("a hung server returned no error")
	}
}

// fakeChecker counts calls and returns rel, err.
type fakeChecker struct {
	rel   Release
	err   error
	calls int
}

func (f *fakeChecker) Latest(context.Context) (Release, error) {
	f.calls++
	return f.rel, f.err
}

var (
	t0   = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	r031 = Release{Version: "0.3.1", URL: "https://example.test/v0.3.1"}
	r032 = Release{Version: "0.3.2", URL: "https://example.test/v0.3.2"}
)

func writeCache(t *testing.T, path string, at time.Time, rel Release) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"checked_at": at, "latest": rel})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func cached(inner Checker, path string, now time.Time) *Cached {
	c := NewCached(inner, path)
	c.Now = func() time.Time { return now }
	return c
}

func TestCachedUsesAFreshCacheWithoutCalling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	writeCache(t, path, t0.Add(-23*time.Hour), r031)
	inner := &fakeChecker{rel: r032}
	rel, err := cached(inner, path, t0).Latest(context.Background())
	if err != nil || rel != r031 {
		t.Fatalf("got %+v, %v; want the cached %+v", rel, err, r031)
	}
	if inner.calls != 0 {
		t.Fatalf("inner called %d times with a fresh cache", inner.calls)
	}
}

func TestCachedRefreshesAStaleCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	writeCache(t, path, t0.Add(-25*time.Hour), r031)
	inner := &fakeChecker{rel: r032}
	rel, err := cached(inner, path, t0).Latest(context.Background())
	if err != nil || rel != r032 || inner.calls != 1 {
		t.Fatalf("got %+v, %v after %d calls; want %+v after 1", rel, err, inner.calls, r032)
	}
	// The refreshed answer is cached: the next launch does not call.
	again := &fakeChecker{rel: Release{Version: "9.9.9"}}
	rel, _ = cached(again, path, t0.Add(time.Hour)).Latest(context.Background())
	if rel != r032 || again.calls != 0 {
		t.Fatalf("next launch got %+v after %d calls; want the cached %+v", rel, again.calls, r032)
	}
}

func TestCachedTreatsAFutureTimestampAsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	writeCache(t, path, t0.Add(48*time.Hour), r031)
	inner := &fakeChecker{rel: r032}
	if rel, _ := cached(inner, path, t0).Latest(context.Background()); rel != r032 || inner.calls != 1 {
		t.Fatalf("got %+v after %d calls; want a refresh", rel, inner.calls)
	}
}

func TestCachedWritesAPrivateCacheAndCreatesItsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nu11signal")
	path := filepath.Join(dir, "update.json")
	if _, err := cached(&fakeChecker{rel: r031}, path, t0).Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("cache mode = %o; want 600", perm)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"checked_at"`) || !strings.Contains(string(data), `"0.3.1"`) {
		t.Fatalf("cache = %s", data)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries; want only update.json (no temp file left)", len(entries))
	}
}

func TestCachedToleratesACorruptCache(t *testing.T) {
	for name, content := range map[string]string{"garbage": "{nope", "empty": "", "no version": `{"checked_at":"2026-10-01T11:00:00Z","latest":{}}`} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "update.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			inner := &fakeChecker{rel: r032}
			rel, err := cached(inner, path, t0).Latest(context.Background())
			if err != nil || rel != r032 || inner.calls != 1 {
				t.Fatalf("got %+v, %v after %d calls; want a refresh", rel, err, inner.calls)
			}
		})
	}
}

func TestCachedFailureFallsBackToAStaleCacheOrErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	inner := &fakeChecker{err: errors.New("offline")}
	if _, err := cached(inner, path, t0).Latest(context.Background()); err == nil {
		t.Fatal("a failure without a cache returned no error")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a failure wrote the cache")
	}
	writeCache(t, path, t0.Add(-72*time.Hour), r031)
	if rel, err := cached(inner, path, t0).Latest(context.Background()); err != nil || rel != r031 {
		t.Fatalf("got %+v, %v; want the stale %+v", rel, err, r031)
	}
}

func TestCachePathSitsBesideTheConfig(t *testing.T) {
	got := CachePath(filepath.Join("/x", "nu11signal", "config.json"))
	if want := filepath.Join("/x", "nu11signal", "update.json"); got != want {
		t.Fatalf("CachePath = %q; want %q", got, want)
	}
}

func TestUpgradeCommand(t *testing.T) {
	tests := map[string]string{
		"/opt/homebrew/Caskroom/nu11signal/0.3.0/nu11signal-0.3.0/bin/nu11signal": BrewUpgrade,
		"/usr/local/Caskroom/nu11signal/0.3.0/nu11signal-0.3.0/bin/nu11signal":    BrewUpgrade,
		"/opt/homebrew/bin/nu11signal":                                            BrewUpgrade,
		"/Users/me/bin/nu11signal-0.3.0/bin/nu11signal":                           "",
		"/usr/local/bin/nu11signal":                                               "",
		"/opt/homebrewish/bin/nu11signal":                                         "",
		"":                                                                        "",
	}
	for exe, want := range tests {
		if got := UpgradeCommand(exe); got != want {
			t.Errorf("UpgradeCommand(%q) = %q; want %q", exe, got, want)
		}
	}
}
