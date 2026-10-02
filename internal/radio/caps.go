package radio

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Sources: a player may join Apple Music and the local files (see
// playback.Capabilities). What a source does not offer is hidden rather
// than refused: the SEARCH tab and + NEW PLAYLIST without a catalog, the
// favorite and + of a local song. A key pressed for it anyway says why on
// the status line.

// supports reports whether the player offers c for the song or playlist
// id, or at all with an empty id; a player without capabilities offers
// everything.
func (m Model) supports(c playback.Capability, id string) bool {
	return playback.Supports(m.player, c, id)
}

// canSearch reports whether there is a catalog to search.
func (m Model) canSearch() bool { return m.supports(playback.CapCatalogSearch, "") }

// unsupported is the notice for a song whose source cannot do what was
// asked (cannot: LOVED, ADDED).
func unsupported(s playback.Song, cannot string) string {
	if playback.SourceOf(s.ID) == playback.SourceLocal {
		return strings.ToUpper(s.Title) + " IS A LOCAL FILE // IT CANNOT BE " + cannot
	}
	return strings.ToUpper(s.Title) + " CANNOT BE " + cannot + " HERE"
}

// noSearch is the notice for the search keys without a catalog.
const noSearch = "NO CATALOG TO SEARCH // LOCAL FILES ONLY"

// Library changes: a player whose playlists change on their own (a local
// scan that ends after startup, see playback.LibraryWatcher) has them read
// again.
type (
	libraryChangedMsg struct{}
	libraryClosedMsg  struct{}
)

// waitLibrary delivers the next library change; nil for a player that
// does not report them. It is re-armed after each one.
func (m Model) waitLibrary() tea.Cmd {
	ch := m.libraryChanged
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		if _, ok := <-ch; !ok {
			return libraryClosedMsg{}
		}
		return libraryChangedMsg{}
	}
}

// onLibraryChanged reads the playlists again, once the library is
// reachable (until then, authorizing reads them).
func (m Model) onLibraryChanged() (Model, tea.Cmd) {
	if m.auth != authOK {
		return m, m.waitLibrary()
	}
	return m, tea.Batch(m.waitLibrary(), m.loadPlaylistsCmd())
}
