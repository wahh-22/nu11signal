package radio

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	updatecheck "github.com/wahh-22/nu11signal/internal/update"
)

// The update check runs once, from Init, as a command: it never blocks
// the UI and its failures are silent (no network, a rate limit). When it
// finds a release newer than Options.Version, the idle status line
// announces it with the command that upgrades (Options.Upgrade, else the
// release page), a status message still taking precedence while shown,
// and SETTINGS repeats it on its first line. There is nothing to
// dismiss: the notice stays until the next launch of the newer version.

// releaseCheckTimeout bounds the whole check; the GitHub adapter bounds
// its request tighter.
const releaseCheckTimeout = 10 * time.Second

// releaseMsg carries the latest release, or why it is unknown.
type releaseMsg struct {
	rel updatecheck.Release
	err error
}

// checkReleaseCmd asks the checker for the latest release; nil without a
// checker or with a version that does not compare (a "dev" build).
func (m Model) checkReleaseCmd() tea.Cmd {
	c := m.updates
	if c == nil || !updatecheck.Valid(m.version) {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), releaseCheckTimeout)
		defer cancel()
		rel, err := c.Latest(ctx)
		return releaseMsg{rel: rel, err: err}
	}
}

// onRelease keeps a release newer than the running version; anything
// else (an error, the same version, a pre-release) is dropped.
func (m Model) onRelease(msg releaseMsg) Model {
	if msg.err == nil && updatecheck.Newer(msg.rel.Version, m.version) {
		m.newer = msg.rel
	}
	return m
}

// releaseNotice is the newer release ("v0.3.1") and how to get it (the
// upgrade command, else the release page); ok is false while none is
// known.
func (m Model) releaseNotice() (version, how string, ok bool) {
	if m.newer.Version == "" {
		return "", "", false
	}
	how = m.upgrade
	if how == "" {
		how = m.newer.URL
	}
	return "v" + m.newer.Version, how, true
}
