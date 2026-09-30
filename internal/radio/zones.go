package radio

import (
	"fmt"
	"strconv"
	"strings"
)

// Mouse hit testing. Every renderer that draws something clickable also
// returns the zones it drew, in its own coordinates; the layers composing
// the frame shift them to where they put that output. The zones therefore
// always come from the same pass that drew the frame, and a click is
// resolved by laying out the frame again (see Model.layout).

// Zone IDs. Rows of the list panel use rowZone.
const (
	zoneInput = "input"
	// zoneNameInput is the NEW PLAYLIST name input.
	zoneNameInput   = "input:name"
	zoneRetry       = "retry"
	zoneTabStations = "tab:stations"
	zoneTabSearch   = "tab:search"
	zoneBack        = "back"
	zonePrev        = "prev"
	zonePlay        = "play"
	zoneNext        = "next"
	zoneSeek        = "seek"
	zoneExpand      = "expand"
	zoneVolDown     = "vol:down"
	zoneVolUp       = "vol:up"
	zoneLoop        = "loop"
	// zoneRowFavorite and zoneRowAdd are the favorite mark and + ending
	// the selected song row; zoneFavPlaying the [<3] button of the song
	// playing.
	zoneRowFavorite = "fav:row"
	zoneRowAdd      = "add:row"
	zoneFavPlaying  = "fav:playing"
	// zoneNewPlaylist is the + NEW PLAYLIST row over the playlists.
	zoneNewPlaylist = "new-playlist"
	// zoneEditCreate and zoneEditCancel are the buttons under the NEW
	// PLAYLIST name.
	zoneEditCreate = "edit:create"
	zoneEditCancel = "edit:cancel"
	// zonePanelList and zonePanelPlayer cover the whole list and NOW
	// PLAYING panels, frame included, under the zones drawn in them.
	zonePanelList    = "panel:list"
	zonePanelPlayer  = "panel:player"
	rowZonePrefix    = "row:"
	deleteZonePrefix = "delete:"
)

// rowZone is the ID of selectable row i of the view on top: a station, a
// search row or a page item.
func rowZone(i int) string { return rowZonePrefix + strconv.Itoa(i) }

// recentDeleteZone is the ID of the ✕ that deletes the recent term on
// search row i.
func recentDeleteZone(i int) string { return deleteZonePrefix + strconv.Itoa(i) }

// rowOf parses a rowZone ID.
func rowOf(id string) (int, bool) { return indexOf(id, rowZonePrefix) }

// recentDeleteOf parses a recentDeleteZone ID.
func recentDeleteOf(id string) (int, bool) { return indexOf(id, deleteZonePrefix) }

// indexOf parses an ID made of prefix and an index.
func indexOf(id, prefix string) (int, bool) {
	s, ok := strings.CutPrefix(id, prefix)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(s)
	return i, err == nil
}

// zone is a clickable region one line tall: w cells from x on line y.
type zone struct {
	id      string
	x, y, w int
}

func (z zone) String() string { return fmt.Sprintf("%s@%d,%d+%d", z.id, z.x, z.y, z.w) }

// zones is the set of clickable regions of a frame; later ones are on top.
type zones []zone

// add registers a region; empty ones are dropped.
func (zs *zones) add(id string, x, y, w int) {
	if w > 0 {
		*zs = append(*zs, zone{id: id, x: x, y: y, w: w})
	}
}

// addBox registers a region h lines tall as one zone per line.
func (zs *zones) addBox(id string, x, y, w, h int) {
	for i := range h {
		zs.add(id, x, y+i, w)
	}
}

// addAt registers the zones of a part drawn at (dx, dy).
func (zs *zones) addAt(dx, dy int, part zones) {
	for _, z := range part {
		zs.add(z.id, z.x+dx, z.y+dy, z.w)
	}
}

// shifted returns the zones moved by (dx, dy).
func (zs zones) shifted(dx, dy int) zones {
	var out zones
	out.addAt(dx, dy, zs)
	return out
}

// clip drops the parts of the zones outside a w x h frame.
func (zs zones) clip(w, h int) zones {
	var out zones
	for _, z := range zs {
		if z.y < 0 || z.y >= h || z.x >= w {
			continue
		}
		x := max(z.x, 0)
		out.add(z.id, x, z.y, min(z.x+z.w, w)-x)
	}
	return out
}

// at returns the topmost zone under cell (x, y).
func (zs zones) at(x, y int) (zone, bool) {
	for i := len(zs) - 1; i >= 0; i-- {
		z := zs[i]
		if y == z.y && x >= z.x && x < z.x+z.w {
			return z, true
		}
	}
	return zone{}, false
}

// find returns the zone with id.
func (zs zones) find(id string) (zone, bool) {
	for _, z := range zs {
		if z.id == id {
			return z, true
		}
	}
	return zone{}, false
}
