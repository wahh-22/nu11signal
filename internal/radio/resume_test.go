package radio

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

func stackKinds(m Model) []viewKind {
	kinds := make([]viewKind, len(m.stack))
	for i, f := range m.stack {
		kinds[i] = f.kind
	}
	return kinds
}

func TestLeavingTheSearchBranchParksItAndSearchKeysRestoreIt(t *testing.T) {
	for _, k := range []string{"/", "tab"} {
		t.Run(k, func(t *testing.T) {
			f := playbacktest.New()
			m := openDaftPunk(t, f)
			m, _ = press(t, m, "down", "down")
			cursor := m.cursor()

			m, _ = press(t, m, "tab")
			if !reflect.DeepEqual(stackKinds(m), []viewKind{viewStations}) {
				t.Fatalf("tab left stack %v; want the stations root", stackKinds(m))
			}
			m, _ = press(t, m, k)
			if want := []viewKind{viewStations, viewSearch, viewArtist}; !reflect.DeepEqual(stackKinds(m), want) {
				t.Fatalf("%s restored stack %v; want %v", k, stackKinds(m), want)
			}
			if m.cursor() != cursor || !strings.Contains(plain(m), "DAFT PUNK") {
				t.Fatalf("artist cursor %d; want %d, view:\n%s", m.cursor(), cursor, plain(m))
			}
			if n := len(artistCalls(f)); n != 1 {
				t.Fatalf("Artist calls = %d; want the parked page reused, not reloaded", n)
			}
			if m.input.Focused() {
				t.Fatal("the input took the keys on the artist page")
			}
			// Back goes down the restored branch, to the search as it was.
			m, _ = press(t, m, "esc")
			if m.top().kind != viewSearch || m.input.Value() != "daft" || !m.input.Focused() {
				t.Fatalf("esc after restoring: top %v, input %q", m.top().kind, m.input.Value())
			}
		})
	}
}

func TestParkedAlbumAndArtistKeepTheirCursors(t *testing.T) {
	f := playbacktest.New()
	m := openFromArtist(t, f, itemAlbum)
	artistCursor := m.stack[2].cursor
	m, _ = press(t, m, "down")
	albumCursor := m.cursor()

	m, _ = press(t, m, "tab", "/")
	if want := []viewKind{viewStations, viewSearch, viewArtist, viewAlbum}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("restored stack %v; want %v", stackKinds(m), want)
	}
	if m.cursor() != albumCursor {
		t.Fatalf("album cursor %d; want %d", m.cursor(), albumCursor)
	}
	m, _ = press(t, m, "esc")
	if m.top().kind != viewArtist || m.cursor() != artistCursor {
		t.Fatalf("esc: top %v cursor %d; want the artist at %d", m.top().kind, m.cursor(), artistCursor)
	}
	if n := len(callsOf(f, "Album")); n != 1 {
		t.Fatalf("Album calls = %d; want the parked page reused", n)
	}
}

func TestParkedPageSettlesItsLoad(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.ArtistResult = artistDetail()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "down", "down", "down")
	m, load := press(t, m, "enter")

	// Leave while the page loads; its answer fills the parked page.
	m, _ = press(t, m, "tab")
	m = settle(t, m, load)
	if strings.Contains(m.status, "FAILED") {
		t.Fatalf("parking reported a failure: %q", m.status)
	}
	m, _ = press(t, m, "/")
	if m.top().kind != viewArtist || m.top().artist.loading || !strings.Contains(plain(m), "GET LUCKY") {
		t.Fatalf("restored page did not settle:\n%s", plain(m))
	}
}

func TestSlashOnADetailPageReturnsToTheSearchInput(t *testing.T) {
	f := playbacktest.New()
	m := openFromArtist(t, f, itemAlbum)

	m, cmd := press(t, m, "/")
	m = settle(t, m, cmd)
	if want := []viewKind{viewStations, viewSearch}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("stack %v; want %v", stackKinds(m), want)
	}
	if m.cursor() != -1 || !m.input.Focused() || m.input.Value() != "daft" {
		t.Fatalf("cursor %d, focused %v, input %q; want the input focused with the term", m.cursor(), m.input.Focused(), m.input.Value())
	}
	if !strings.Contains(plain(m), "ONE MORE TIME") {
		t.Fatalf("the results for the kept term are not shown:\n%s", plain(m))
	}
	// The term stays editable.
	m = typeText(t, m, "x")
	if m.input.Value() != "daftx" {
		t.Fatalf("input %q; want the kept term edited", m.input.Value())
	}
	// Nothing is parked: tab then / come back to this search, not the album.
	m, _ = press(t, m, "tab", "/")
	if m.top().kind != viewSearch {
		t.Fatalf("top %v; want the search", m.top().kind)
	}
}

func TestSearchKeysWithNothingParkedOpenAFreshSearch(t *testing.T) {
	for _, k := range []string{"/", "tab"} {
		t.Run(k, func(t *testing.T) {
			f := playbacktest.New()
			f.SearchCatalogResult = catalog()
			m := searchFor(t, loaded(t, f, newClock()), "daft")
			// esc closes the search: nothing is parked.
			m, _ = press(t, m, "esc", k)
			if m.top().kind != viewSearch || m.input.Value() != "" || !m.input.Focused() || !strings.Contains(plain(m), "RECENT") {
				t.Fatalf("%s did not open a fresh search:\n%s", k, plain(m))
			}
		})
	}
}
