package radio

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The browse pages (RESULTS, ARTIST, ALBUM/SONG, PLAYLIST) share their mechanics:
// keys, a fixed head over a scrolling body of lines, some of them
// selectable, and editorial notes folded behind MORE.

// notesLines is how many lines of editorial notes a page shows until MORE
// expands them.
const notesLines = 3

// isPage reports whether kind is a browse page.
func isPage(kind viewKind) bool {
	return kind == viewResults || kind == viewArtist || kind == viewAlbum || kind == viewPlaylist
}

// pageLine is one line of a page body below its head; item is the
// selectable row it shows, or -1. actions marks the selected song row,
// which ends in its favorite mark and + controls.
type pageLine struct {
	text    string
	item    int
	actions bool
}

// handlePageKey handles the keys a browse page owns; ok is false for the
// others (player keys, quit), which keep their usual meaning.
func (m Model) handlePageKey(k string) (next Model, cmd tea.Cmd, ok bool) {
	switch k {
	case keyUp:
		if m.atListTop() {
			m.focusTabs()
			return m, nil, true
		}
		m.setCursor(m.cursor() - 1)
	case keyDown:
		m.setCursor(max(min(m.cursor()+1, m.pageItemCount()-1), 0))
	case keyEnter:
		next, cmd = m.pageEnter()
		return next, cmd, true
	case keyEsc:
		m.pop()
		if m.top().kind == viewSearch {
			// RESULTS may have left another term in the input.
			cmd = tea.Batch(m.input.Focus(), m.resumeSearch())
		}
	case keyTab:
		if m.onPlaylistsBranch() {
			// As tab from the PLAYLISTS root: over to SEARCH.
			m.popToRoot()
			next, cmd := m.resumeOrOpenSearch()
			return next.(Model), cmd, true
		}
		// Park the branch, loads included, for / or tab from the stations.
		m.parkBranch()
	case keySearch:
		next, cmd = m.searchAgain()
		return next, cmd, true
	case keyRetry:
		if !m.pageFailed() {
			return m, nil, false
		}
		cmd = m.reloadPage()
	case keyLove:
		next, cmd = m.loveTarget()
		return next, cmd, true
	case keyAdd:
		next, cmd = m.addTarget()
		return next, cmd, true
	case keyAlbum:
		if m.top().kind != viewResults {
			return m, nil, false
		}
		return m.resultsAlbum()
	default:
		return m, nil, false
	}
	return m, cmd, true
}

// pageItemCount is the number of selectable rows of the page on top.
func (m Model) pageItemCount() int {
	switch m.top().kind {
	case viewResults:
		return len(m.resultItems())
	case viewArtist:
		return len(m.artistItems())
	}
	return len(m.trackItems())
}

// pageEnter acts on the selected row of the page on top.
func (m Model) pageEnter() (Model, tea.Cmd) {
	switch m.top().kind {
	case viewResults:
		return m.resultsEnter()
	case viewArtist:
		return m.artistEnter()
	}
	return m.tracksEnter()
}

// pageFailed reports whether the load of the page on top failed.
func (m Model) pageFailed() bool {
	f := m.top()
	switch f.kind {
	case viewResults:
		return f.results.err != nil
	case viewArtist:
		return f.artist.err != nil
	}
	return f.tracks.err != nil
}

// reloadPage loads the page on top again, as it was selected.
func (m *Model) reloadPage() tea.Cmd {
	f := m.top()
	switch f.kind {
	case viewResults:
		return m.loadResults(f.results.term)
	case viewArtist:
		return m.loadArtist(f.artist.artist)
	}
	return m.loadTracks(f.tracks, f.kind)
}

// cancelLoad cancels the load of a page frame, if one is in flight.
func (f frame) cancelLoad() {
	switch {
	case f.kind == viewResults && f.results.cancel != nil:
		f.results.cancel()
	case f.kind == viewArtist && f.artist.cancel != nil:
		f.artist.cancel()
	case (f.kind == viewAlbum || f.kind == viewPlaylist) && f.tracks.cancel != nil:
		f.tracks.cancel()
	}
}

// loadFailure is the status line report of a page frame whose load
// failed; empty for any other frame.
func (f frame) loadFailure() string {
	switch {
	case f.kind == viewResults && f.results.err != nil:
		return "SEARCH FAILED // " + f.results.err.Error()
	case f.kind == viewArtist && f.artist.err != nil:
		return "ARTIST FEED FAILED // " + f.artist.err.Error()
	case (f.kind == viewAlbum || f.kind == viewPlaylist) && f.tracks.err != nil:
		return f.tracks.title() + " FEED FAILED // " + f.tracks.err.Error()
	}
	return ""
}

// pageNotice is the notice a page body shows instead of its lines while it
// loads, after it failed, or when it has nothing; empty otherwise.
func pageNotice(loading bool, err error, feed string, empty bool) string {
	switch {
	case loading:
		return stDim.Render("DECRYPTING " + feed + " FEED...")
	case err != nil:
		return stYellow.Render("▲ [R] RETRY // " + strings.ToUpper(cleanLine(err.Error())))
	case empty:
		return stDim.Render("NO DATA ON FILE")
	}
	return ""
}

// pageBody renders a page in w x h cells: its head lines, a rule, then the
// notice, if any, and the lines scrolled to keep the cursor near the
// middle. Every line is exactly w cells wide. Its zones are the selectable
// rows shown and, on a failed page, the retry notice.
func (m Model) pageBody(w, h int, head []string, lines []pageLine, notice string) ([]string, zones) {
	if h <= 0 || w <= 0 {
		return nil, nil
	}
	var out []string
	var zs zones
	for _, l := range head {
		if len(out) == h {
			return out, zs
		}
		out = append(out, fit(l, w))
	}
	if len(out) == h {
		return out, zs
	}
	out = append(out, stFrameDim.Render(strings.Repeat("─", w)))
	if notice != "" {
		if len(out) < h {
			if m.pageFailed() {
				zs.add(zoneRetry, 0, len(out), w)
			}
			out = append(out, fit(" "+notice, w))
		}
		if len(lines) == 0 || len(out) >= h {
			return out, zs
		}
		// A failed page may still offer rows (a SONG view's lone song).
		out = append(out, fit("", w))
	}
	room := h - len(out)
	if room <= 0 {
		return out, zs
	}

	at := 0
	for i, l := range lines {
		if l.item >= 0 && l.item == m.cursor() {
			at = i
		}
	}
	offset := max(0, min(at-(room-1)/2, len(lines)-room))
	for i := offset; i < len(lines) && i-offset < room; i++ {
		if lines[i].item >= 0 {
			zs.add(rowZone(lines[i].item), 0, len(out), w)
		}
		if lines[i].actions {
			addActionZones(&zs, w, len(out))
		}
		out = append(out, fit(lines[i].text, w))
	}
	return out, zs
}

// wrapNotes wraps notes to fit a page w cells wide. Folded, only the first
// notesLines lines are kept; more reports that there are more than that,
// so MORE/LESS is offered.
func wrapNotes(notes string, open bool, w int) (lines []string, more bool) {
	lines = strings.Split(ansi.Wrap(notes, max(w-2, 1), ""), "\n")
	more = len(lines) > notesLines
	if more && !open {
		lines = lines[:notesLines]
	}
	return lines, more
}

// moreLine renders the MORE (or, when open, LESS) row in w cells.
func moreLine(open, selected bool, w int) string {
	label := "▸ MORE"
	if open {
		label = "▴ LESS"
	}
	if selected {
		return stSelected.Render(fit("▌"+label, w))
	}
	return " " + stYellow.Render(label)
}
