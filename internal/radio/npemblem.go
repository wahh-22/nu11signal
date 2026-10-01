package radio

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The NOW PLAYING emblem: the compact null emblem with its text (N U 1 1
// / S I G N A L / the slants), or alone where the text does not fit,
// right-aligned on the artist, album and blank rows under the title,
// its right edge the [<3] button's (nowPlayingMargin from the panel's
// inside edge). It is always drawn (playing, paused, nothing loaded) in
// the full layout, expanded or beside the list, wherever it fits on all
// three rows at least npEmblemGap cells right of the artist and the
// album: the song info is never cut for it, the emblem is left out
// instead. It is not clickable, adds no zone, and never sits under the
// splash, KEYS or SETTINGS (nowPlaying is not drawn then).
//
// With the signal effects active it glitches like the idle emblem (see
// idle.go) on a schedule of its own, npGlitch, whose streams come from
// its own salts, so it never runs in step with the idle emblem or the
// bursts: every idleGapMin..idleGapMax a glitch of
// idleGlitchMin..idleGlitchMax tears one block row 1 cell sideways and
// draws 1..3 block noise cells over the emblem's drawn cells, all inside
// the block's own cells (see glitchArt). The tick lands on its start and
// end and runs at burstTick while it draws, as for the idle glitch.
//
// The content intro compares the artist and album rows only up to the
// block (see introRegions), so neither the emblem nor its glitch ever
// scrambles them.

// npEmblemGap is the fewest blank cells kept between the song info and
// the block.
const npEmblemGap = 2

// Salts keep the NOW PLAYING emblem's streams apart from the other
// effects': saltNPGlitch derives its schedule's seed, saltNPLook its
// glitch's look.
const (
	saltNPGlitch uint64 = iota + 701
	saltNPLook
)

// npEmblemFor places the emblem in a NOW PLAYING inside iw cells wide
// whose artist and album rows end ends cells in (their drawn text, the
// leading margin included): the block with its text where it fits right
// of both with npEmblemGap to spare, else the emblem alone, else false.
// The art's x is its column in the inside, y its first row (npArtistRow).
func npEmblemFor(iw int, ends ...int) (idleArt, bool) {
	end := 0
	for _, e := range ends {
		end = max(end, e)
	}
	for _, text := range []bool{true, false} {
		bw := emblemCompact.width()
		if text {
			bw = emblemCompact.blockWidth()
		}
		if x := iw - nowPlayingMargin - bw; x >= end+npEmblemGap {
			return idleArt{e: emblemCompact, text: text, x: x, y: npArtistRow}, true
		}
	}
	return idleArt{}, false
}

// npEmblem places the emblem for m's frame: false outside the full
// layout's NOW PLAYING (compact or tiny layouts, the auth error screen,
// the splash, KEYS or SETTINGS over it) or where it does not fit beside
// the song info.
func (m Model) npEmblem() (idleArt, bool) {
	if m.width < fullMinWidth || m.height < fullMinHeight || m.boot || m.shutdown || m.help || m.settings || m.auth == authFailed {
		return idleArt{}, false
	}
	_, artist := m.titleLines()
	return npEmblemFor(m.playerPanelWidth()-2, nowPlayingMargin+ansi.StringWidth(artist), nowPlayingMargin+ansi.StringWidth(m.npAlbum()))
}

// npActive reports whether the emblem's glitch runs: the emblem shown and
// the signal effects active.
func (m Model) npActive() bool {
	if !m.fxActive() {
		return false
	}
	_, ok := m.npEmblem()
	return ok
}

// npAlbum is the album row's styled text in NOW PLAYING: the album while
// the player reports a state, blank otherwise.
func (m Model) npAlbum() string {
	if m.hasState && !m.signalLost() {
		return stMuted.Render(strings.ToUpper(m.state.Album))
	}
	return ""
}

// drawNPEmblem draws art a on lines, the NOW PLAYING inside's rows, each
// row of the block padded to its width at column a.x, glitched while its
// glitch draws. The rows it lands on hold only their text, left of a.x.
func (m Model) drawNPEmblem(lines []string, a idleArt) {
	bw := a.e.width()
	if a.text {
		bw = a.e.blockWidth()
	}
	block := idleArt{e: a.e, text: a.text}.lines(bw, len(a.e.rows))
	if m.npActive() && m.npGlitch.on(m.now()) {
		glitchArt(block, idleArt{e: a.e, text: a.text}, mix(m.seed, saltNPLook, m.npGlitch.seq, m.frame))
	}
	for i, row := range block {
		y := a.y + i
		lines[y] += strings.Repeat(" ", max(a.x-ansi.StringWidth(lines[y]), 0)) + row
	}
}
