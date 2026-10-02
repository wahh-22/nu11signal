// Package update finds out whether a newer nu11signal release exists: a
// Checker port answering the latest release, a GitHub adapter (GitHub)
// asking the releases API, and a Cached decorator keeping the answer in
// update.json beside the settings file for a day, so a launch hits the
// API at most once a day. Versions compare as strict semantic versions
// (Newer); what to show and when is the UI's business.
package update

import (
	"context"
	"strconv"
	"strings"
)

// Release is a published release: its version, without the leading v,
// and its page.
type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Checker answers the latest published release. Callers treat an error
// as "unknown", never as something to show.
type Checker interface {
	Latest(ctx context.Context) (Release, error)
}

// ReleasesURL is the releases page, the release URL when GitHub gives
// none.
const ReleasesURL = "https://github.com/wahh-22/nu11signal/releases/latest"

// BrewUpgrade upgrades a Homebrew cask install.
const BrewUpgrade = "brew upgrade --cask nu11signal"

// BrewUpgradeFormula upgrades a Homebrew formula install (Linux, or macOS
// without --cask).
const BrewUpgradeFormula = "brew upgrade nu11signal"

// UpgradeCommand is the command that upgrades the binary at exe (the
// resolved executable path): BrewUpgradeFormula when it is a formula's
// keg (Cellar/nu11signal/<version>/bin/nu11signal, under any Homebrew
// prefix), BrewUpgrade when it lives under the cask's Homebrew (a
// Caskroom, or the /opt/homebrew prefix), else "" for "download the
// release" (no subprocess is run to find out).
func UpgradeCommand(exe string) string {
	if inFormulaKeg(exe) {
		return BrewUpgradeFormula
	}
	if strings.Contains(exe, "/Caskroom/") || strings.HasPrefix(exe, "/opt/homebrew/") {
		return BrewUpgrade
	}
	return ""
}

// inFormulaKeg reports whether exe is .../Cellar/nu11signal/<version>/bin/nu11signal.
func inFormulaKeg(exe string) bool {
	_, rest, ok := strings.Cut(exe, "/Cellar/nu11signal/")
	if !ok {
		return false
	}
	version, bin, ok := strings.Cut(rest, "/")
	return ok && version != "" && bin == "bin/nu11signal"
}

// semver is a parsed MAJOR.MINOR.PATCH with its pre-release, if any;
// build metadata is dropped.
type semver struct {
	core [3]int
	pre  string
}

// parse reads a strict semantic version, an optional single leading v
// allowed: three dot-separated decimal numbers, then an optional
// non-empty -pre-release and +build. ok is false for anything else
// ("dev", "1.2", "1.2.x").
func parse(v string) (s semver, ok bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		if i == len(v)-1 {
			return semver{}, false
		}
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		if i == len(v)-1 {
			return semver{}, false
		}
		v, s.pre = v[:i], v[i+1:]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return semver{}, false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return semver{}, false
		}
		s.core[i] = n
	}
	return s, true
}

// Valid reports whether v is a version Newer can compare.
func Valid(v string) bool {
	_, ok := parse(v)
	return ok
}

// Newer reports whether latest is a release newer than current: its
// MAJOR.MINOR.PATCH is greater, or equal while current is a pre-release
// of it. A pre-release latest is never newer (nobody is nudged onto a
// release candidate), and an unparsable version on either side ("dev")
// is no update.
func Newer(latest, current string) bool {
	l, ok := parse(latest)
	if !ok || l.pre != "" {
		return false
	}
	c, ok := parse(current)
	if !ok {
		return false
	}
	for i := range l.core {
		if l.core[i] != c.core[i] {
			return l.core[i] > c.core[i]
		}
	}
	return c.pre != ""
}
