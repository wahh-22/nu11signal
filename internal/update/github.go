package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GitHub is a Checker asking the GitHub releases API for the latest
// release of wahh-22/nu11signal (drafts and pre-releases excluded by
// GitHub).
type GitHub struct {
	// BaseURL is the API root; tests point it at a local server.
	BaseURL string
	// Client makes the request; its Timeout bounds the whole check.
	Client *http.Client
	// UserAgent identifies the caller, as the API requires.
	UserAgent string
}

// NewGitHub returns a GitHub checker for the running version: the
// public API, a 3 s timeout, User-Agent nu11signal/<version>.
func NewGitHub(version string) *GitHub {
	return &GitHub{
		BaseURL:   "https://api.github.com",
		Client:    &http.Client{Timeout: 3 * time.Second},
		UserAgent: "nu11signal/" + version,
	}
}

// Latest asks for the latest release. A status other than 200, a body
// that is not the release JSON, or a tag that is not a version is an
// error.
func (g *GitHub) Latest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(g.BaseURL, "/")+"/repos/wahh-22/nu11signal/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", g.UserAgent)
	resp, err := g.Client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("latest release: %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("latest release: %w", err)
	}
	if !Valid(body.TagName) {
		return Release{}, fmt.Errorf("latest release: tag %q is not a version", body.TagName)
	}
	rel := Release{Version: strings.TrimPrefix(body.TagName, "v"), URL: body.HTMLURL}
	// Only a plain link into this repository is shown; anything else (or
	// nothing) falls back to the releases page.
	if !releasePage(rel.URL) {
		rel.URL = ReleasesURL
	}
	return rel, nil
}

// releasePage reports whether u is a printable https link into the
// repository's GitHub pages.
func releasePage(u string) bool {
	if !strings.HasPrefix(u, "https://github.com/wahh-22/nu11signal/") {
		return false
	}
	for _, r := range u {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}
