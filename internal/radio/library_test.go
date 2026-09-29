package radio

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// editableStations are library playlists as the API lists them: one
// followed from the catalog, which cannot take songs.
func editableStations() []playback.Playlist {
	return []playback.Playlist{
		{ID: "pl-f", Name: "Canciones favoritas"},
		{ID: "pl-1", Name: "Night Drive", Editable: true},
		{ID: "pl-2", Name: "Samurai", Editable: true},
	}
}

// withStations lists pls as the library playlists.
func withStations(t *testing.T, m Model, pls []playback.Playlist) Model {
	t.Helper()
	m, _ = step(t, m, playlistsMsg{playlists: pls})
	return m
}

// selectedRow is the unstyled list panel row the cursor is on.
func selectedRow(t *testing.T, m Model) string {
	t.Helper()
	_, zs := m.layout()
	for _, z := range zs {
		if _, ok := rowOf(z.id); !ok {
			continue
		}
		if row, _ := rowOf(z.id); row == m.cursor() {
			return textAt(m, z)
		}
	}
	t.Fatalf("no selected row in:\n%s", plain(m))
	return ""
}

func TestLoveTogglesTheSelectedTrackOptimistically(t *testing.T) {
	f := playbacktest.New()
	m := openSong(t, f, 1) // DIGITAL LOVE (s2), selected
	if row := selectedRow(t, m); !strings.Contains(row, "♡") || !strings.Contains(row, "+") {
		t.Fatalf("selected track %q; want its ♡ and + controls", row)
	}
	m, cmd := press(t, m, "l")
	if row := selectedRow(t, m); !strings.Contains(row, "♥") {
		t.Fatalf("after l the row shows %q; want ♥ at once", row)
	}
	m = settle(t, m, cmd)
	assertCall(t, f, "SetFavorite", "s2", true)
	if on, known := m.favoriteOf("s2"); !on || !known {
		t.Fatalf("favorite s2 = %v known %v; want loved", on, known)
	}
	m, cmd = press(t, m, "l")
	m = settle(t, m, cmd)
	assertCall(t, f, "SetFavorite", "s2", false)
	if row := selectedRow(t, m); strings.Contains(row, "♥") {
		t.Fatalf("after unloving the row shows %q", row)
	}
}

// pageReadFails makes the Favorites read a page starts as it loads fail,
// leaving the states to the tick's reads one by one (see prefetch_test.go
// for the page read).
func pageReadFails(f *playbacktest.Fake) {
	if f.MethodErr == nil {
		f.MethodErr = map[string]error{}
	}
	f.MethodErr["Favorites"] = errors.New("offline")
}

func TestFavoritesAreReadLazilyAndCached(t *testing.T) {
	f := playbacktest.New()
	f.Loved = map[string]bool{"s2": true}
	pageReadFails(f)
	m := openSong(t, f, 1) // DIGITAL LOVE (s2)
	if n := len(callsOf(f, "Favorite")); n != 0 {
		t.Fatalf("opening the page read %d favorites; want none before a tick", n)
	}
	// The tick reads the selected song's state, once.
	m, _ = step(t, m, tickMsg{gen: m.tickGen})
	if !m.favs["s2"].reading {
		t.Fatal("the tick did not start reading the selected song")
	}
	m, cmd := m.readFavorites()
	if cmd != nil {
		t.Fatal("a second read started while one is in flight")
	}
	m.favs = nil
	m, cmd = m.readFavorites()
	m = settle(t, m, cmd)
	if calls := callsOf(f, "Favorite"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{"s2"}) {
		t.Fatalf("Favorite calls = %v; want one for s2", calls)
	}
	if row := selectedRow(t, m); !strings.Contains(row, "♥") {
		t.Fatalf("loved song shows %q; want ♥", row)
	}
	if _, cmd = m.readFavorites(); cmd != nil {
		t.Fatal("a cached state was read again")
	}
	// A loved song keeps its ♥ when the cursor leaves it.
	m, _ = press(t, m, "up")
	if !strings.Contains(trackRows(m), "♥") {
		t.Fatalf("the loved track lost its ♥ off the cursor:\n%s", trackRows(m))
	}
	m, cmd = m.readFavorites()
	settle(t, m, cmd)
	if calls := callsOf(f, "Favorite"); len(calls) != 2 || !reflect.DeepEqual(calls[1].Args, []any{"s4"}) {
		t.Fatalf("Favorite calls = %v; want the newly selected s4 read", calls)
	}
}

func TestAReadAnsweringAfterAToggleIsDropped(t *testing.T) {
	f := playbacktest.New()
	pageReadFails(f)
	m := openSong(t, f, 1)
	m, read := m.readFavorites()
	msg := run(t, read) // answers false
	m, cmd := press(t, m, "l")
	m, _ = step(t, m, msg)
	if on, _ := m.favoriteOf("s2"); !on {
		t.Fatal("a stale read overwrote the toggle")
	}
	settle(t, m, cmd)
}

func TestLoveFailureRevertsAndReportsIt(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"SetFavorite": errors.New("ratings unavailable")}
	m := openSong(t, f, 1)
	m, cmd := press(t, m, "l")
	m = settle(t, m, cmd)
	if view := plain(m); !strings.Contains(view, "LOVE FAILED // RATINGS UNAVAILABLE") {
		t.Fatalf("no failure notice:\n%s", view)
	}
	if _, known := m.favoriteOf("s2"); known {
		t.Fatal("a failed change kept its optimistic state; want it read again")
	}
	if row := selectedRow(t, m); strings.Contains(row, "♥") {
		t.Fatalf("failed love still shows %q", row)
	}
}

func TestLibraryOnlySongsCannotBeLovedOrAdded(t *testing.T) {
	f := playbacktest.New()
	f.LibraryPlaylistResult = withUpload()
	m := openStation(t, loaded(t, f, newClock()), 0)
	m, _ = press(t, m, "down", "down") // RESONANCE, library-only
	before := len(f.Calls())
	for _, k := range []string{"l", "a"} {
		m, cmd := press(t, m, k)
		if cmd != nil {
			settle(t, m, cmd)
		}
		if calls := f.Calls()[before:]; len(calls) != 0 {
			t.Fatalf("%s on a library-only song called %v", k, calls)
		}
		if m.editor.mode != editClosed {
			t.Fatalf("%s on a library-only song opened the editor", k)
		}
		if view := plain(m); !strings.Contains(view, "RESONANCE IS NOT IN THE APPLE MUSIC CATALOG") {
			t.Fatalf("%s: no notice for the library-only song:\n%s", k, view)
		}
	}
	m.favs = nil
	if _, cmd := m.readFavorites(); cmd != nil {
		t.Fatal("the state of a library-only song was read")
	}
}

func TestLoveActsOnTheSongPlaying(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f) // Chippin' In, c1
	// On the PLAYLISTS root no song row is selected: l loves the song playing.
	m, cmd := press(t, m, "l")
	m = settle(t, m, cmd)
	assertCall(t, f, "SetFavorite", "c1", true)
	if got := textAt(m, zoneOf(t, m, zoneFavPlaying)); !strings.Contains(got, "♥") {
		t.Fatalf("NOW PLAYING heart shows %q; want ♥", got)
	}
	m, cmd = click(t, m, zoneFavPlaying)
	m = settle(t, m, cmd) // one change in flight at a time: let it answer
	assertCall(t, f, "SetFavorite", "c1", false)

	// With the player focused, l acts there too.
	m, _ = press(t, m, "right")
	m, cmd = press(t, m, "l")
	settle(t, m, cmd)
	assertCall(t, f, "SetFavorite", "c1", true)
	if m.focus != areaPlayer {
		t.Fatalf("focus %v; want the player kept", m.focus)
	}
	// a adds the song playing, even over a page with a song selected.
	m = openSong(t, f, 1)
	m, _ = step(t, m, stateMsg{state: playing(time.Minute, 3*time.Minute)})
	m, _ = press(t, m, "right", "a")
	if m.editor.mode != editPick || m.editor.song.ID != "c1" || m.focus != areaList {
		t.Fatalf("a on the player: editor %v song %q focus %v; want the picker for c1", m.editor.mode, m.editor.song.ID, m.focus)
	}
}

func TestLoveOnSearchSongRows(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	// On the input, l is typed.
	m = typeText(t, m, "l")
	if m.input.Value() != "daftl" || len(callsOf(f, "SetFavorite")) != 0 {
		t.Fatalf("l on the input: value %q calls %v; want it typed", m.input.Value(), callsOf(f, "SetFavorite"))
	}
	m, _ = press(t, m, "backspace")
	m, cmd := step(t, m, searchDebounceMsg{seq: m.search.seq})
	m = settle(t, m, cmd)
	m, _ = press(t, m, "down", "down", "down", "down") // suggestions, artist, One More Time
	m, cmd = press(t, m, "l")
	settle(t, m, cmd)
	assertCall(t, f, "SetFavorite", "s1", true)
	if m.input.Value() != "daft" {
		t.Fatalf("l on a song row typed: %q", m.input.Value())
	}
	m, _ = press(t, m, "a")
	if m.editor.mode != editPick || m.editor.song.ID != "s1" {
		t.Fatalf("a on a song row: editor %+v; want the picker for s1", m.editor)
	}
}

func TestLoveOnArtistAndResultsSongs(t *testing.T) {
	f := playbacktest.New()
	m := openDaftPunk(t, f) // TOP SONGS first
	m, cmd := press(t, m, "l")
	settle(t, m, cmd)
	assertCall(t, f, "SetFavorite", "s1", true)

	f = playbacktest.New()
	f.SearchCatalogResult = fullCatalog()
	m = openResults(t, f, &fakeRecents{})
	m, _ = press(t, m, "down") // TOP RESULTS: the song One More Time
	m, cmd = press(t, m, "l")
	settle(t, m, cmd)
	assertCall(t, f, "SetFavorite", "s1", true)
}

func TestClickOnTheRowHeartLovesIt(t *testing.T) {
	f := playbacktest.New()
	m := openSong(t, f, 1)
	m, cmd := click(t, m, zoneRowFavorite)
	settle(t, m, cmd)
	assertCall(t, f, "SetFavorite", "s2", true)
	if n := len(callsOf(f, "PlaySongs")); n != 0 {
		t.Fatal("the heart also played the track")
	}
}

func TestAddToPlaylistPicker(t *testing.T) {
	f := playbacktest.New()
	m := withStations(t, openSong(t, f, 1), editableStations())
	m, _ = press(t, m, "a")
	if m.editor.mode != editPick {
		t.Fatalf("a: editor mode %v; want the picker", m.editor.mode)
	}
	view := plain(m)
	for _, want := range []string{"ADD TO PLAYLIST", "DIGITAL LOVE", "+ NEW PLAYLIST", "NIGHT DRIVE", "SAMURAI"} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "CANCIONES FAVORITAS") {
		t.Fatalf("the picker offers a playlist that cannot take songs:\n%s", view)
	}
	m, _ = press(t, m, "down", "down") // SAMURAI
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	assertCall(t, f, "AddToPlaylist", "pl-2", []string{"s2"})
	if m.editor.mode != editClosed || m.top().kind != viewAlbum {
		t.Fatalf("after adding: editor %v top %v; want the SONG view back", m.editor.mode, m.top().kind)
	}
	if view := plain(m); !strings.Contains(view, "ADDED DIGITAL LOVE TO SAMURAI") {
		t.Fatalf("no success notice:\n%s", view)
	}
}

func TestAddToPlaylistFailureKeepsThePicker(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"AddToPlaylist": errors.New("playlist is not editable")}
	m := withStations(t, openSong(t, f, 1), editableStations())
	m, _ = press(t, m, "a", "down")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	if m.editor.mode != editPick {
		t.Fatal("a failed add closed the picker")
	}
	if view := plain(m); !strings.Contains(view, "ADD FAILED // PLAYLIST IS NOT EDITABLE") {
		t.Fatalf("no failure notice:\n%s", view)
	}
}

func TestAddToPlaylistCancel(t *testing.T) {
	f := playbacktest.New()
	m := withStations(t, openSong(t, f, 1), editableStations())
	before := len(f.Calls())
	m, _ = press(t, m, "a", "down", "esc")
	if m.editor.mode != editClosed || m.top().kind != viewAlbum || m.cursor() != 2 {
		t.Fatalf("esc: editor %v top %v cursor %d; want the page as it was", m.editor.mode, m.top().kind, m.cursor())
	}
	if calls := f.Calls()[before:]; len(calls) != 0 {
		t.Fatalf("cancelling called %v", calls)
	}
	// ◀ BACK cancels too, and a click on the row's + opens the picker.
	m, _ = click(t, m, zoneRowAdd)
	if m.editor.mode != editPick {
		t.Fatal("the row's + did not open the picker")
	}
	m, _ = click(t, m, zoneBack)
	if m.editor.mode != editClosed || m.top().kind != viewAlbum {
		t.Fatalf("BACK: editor %v top %v; want the page", m.editor.mode, m.top().kind)
	}
}

func TestClickOnAPickerRowAddsTheSong(t *testing.T) {
	f := playbacktest.New()
	m := withStations(t, openSong(t, f, 1), editableStations())
	m, _ = press(t, m, "a")
	m.pressGuardUntil = time.Time{}
	m, cmd := click(t, m, rowZone(1)) // NIGHT DRIVE
	settle(t, m, cmd)
	assertCall(t, f, "AddToPlaylist", "pl-1", []string{"s2"})
}

func TestNewPlaylistFromThePlaylistsRoot(t *testing.T) {
	f := playbacktest.New()
	f.CreatePlaylistResult = playback.Playlist{ID: "p.new", Name: "Neon Rain"}
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "up")
	if m.stationCursor() != -1 || !strings.Contains(selectedStationText(t, m), "+ NEW PLAYLIST") {
		t.Fatalf("up from the first playlist: cursor %d; want the + NEW PLAYLIST row", m.stationCursor())
	}
	m, _ = press(t, m, "enter")
	if m.editor.mode != editName {
		t.Fatalf("enter on + NEW PLAYLIST: editor %v; want the name input", m.editor.mode)
	}
	// An empty name creates nothing.
	m, cmd := press(t, m, "enter")
	if cmd != nil || len(callsOf(f, "CreatePlaylist")) != 0 {
		t.Fatal("an empty name created a playlist")
	}
	m = typeText(t, m, "Neon Rain")
	m, cmd = press(t, m, "enter")
	m, refresh := step(t, m, run(t, cmd))
	if calls := callsOf(f, "CreatePlaylist"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{"Neon Rain", "", []string(nil)}) {
		t.Fatalf("CreatePlaylist calls = %v; want Neon Rain, empty", calls)
	}
	if m.editor.mode != editClosed {
		t.Fatal("the name input stayed open")
	}
	// The list is read again, but the API has not listed it yet: it is
	// kept, selected, and editable.
	m = settle(t, m, refresh)
	if n := len(callsOf(f, "Playlists")); n != 2 {
		t.Fatalf("Playlists called %d times; want a refresh", n)
	}
	i := m.stationCursor()
	if i < 0 || i >= len(m.stations) || m.stations[i].ID != "p.new" || !m.stations[i].Editable {
		t.Fatalf("stations %v cursor %d; want p.new selected", m.stations, i)
	}
	if view := plain(m); !strings.Contains(view, "NEON RAIN") || !strings.Contains(view, "CREATED NEON RAIN") {
		t.Fatalf("the new playlist or its notice is missing:\n%s", view)
	}
	// Once the API lists it, it is not doubled.
	m = withStations(t, m, append(stations(), playback.Playlist{ID: "p.new", Name: "Neon Rain", Editable: true}))
	if n := len(slices.DeleteFunc(slices.Clone(m.stations), func(p playback.Playlist) bool { return p.ID != "p.new" })); n != 1 {
		t.Fatalf("p.new listed %d times: %v", n, m.stations)
	}
}

// selectedStationText is the unstyled row the stations cursor is on.
func selectedStationText(t *testing.T, m Model) string {
	t.Helper()
	id := zoneNewPlaylist
	if c := m.stationCursor(); c >= 0 {
		id = rowZone(c)
	}
	return textAt(m, zoneOf(t, m, id))
}

func TestNewPlaylistCancelAndMouse(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	// One control for the keyboard and the mouse: the row, not a nav
	// button besides.
	_, zs := m.layout()
	if hasZone(zs, "tab:new-playlist") || strings.Count(plain(m), "+ NEW PLAYLIST") != 1 {
		t.Fatalf("+ NEW PLAYLIST is drawn more than once:\n%s", plain(m))
	}
	m, _ = click(t, m, zoneNewPlaylist)
	if m.editor.mode != editName || !m.nameInput.Focused() {
		t.Fatalf("NEW PLAYLIST row: editor %v; want the name input", m.editor.mode)
	}
	// Letters are typed, the player's and quit included.
	m = typeText(t, m, "xjkq")
	if m.nameInput.Value() != "xjkq" || m.volumePending != 0 {
		t.Fatalf("name %q; want every letter typed, no volume change", m.nameInput.Value())
	}
	m, _ = press(t, m, "esc")
	if m.editor.mode != editClosed || len(callsOf(f, "CreatePlaylist")) != 0 {
		t.Fatal("esc did not cancel")
	}
	m.pressGuardUntil = time.Time{}
	m, _ = click(t, m, zoneNewPlaylist)
	if m.editor.mode != editName || m.nameInput.Value() != "" {
		t.Fatalf("the + NEW PLAYLIST row: editor %v name %q; want an empty name input", m.editor.mode, m.nameInput.Value())
	}
	m = typeText(t, m, "Mix")
	m.pressGuardUntil = time.Time{}
	m, cmd := click(t, m, zoneEditCreate)
	settle(t, m, cmd)
	if calls := callsOf(f, "CreatePlaylist"); len(calls) != 1 || calls[0].Args[0] != "Mix" {
		t.Fatalf("CREATE: CreatePlaylist calls = %v; want Mix", calls)
	}
}

func TestNewPlaylistFromThePickerHoldsTheSong(t *testing.T) {
	f := playbacktest.New()
	f.CreatePlaylistResult = playback.Playlist{ID: "p.new", Name: "Robots"}
	m := withStations(t, openSong(t, f, 1), editableStations())
	m, _ = press(t, m, "a", "enter") // + NEW PLAYLIST
	if m.editor.mode != editName {
		t.Fatalf("editor %v; want the name input", m.editor.mode)
	}
	if view := plain(m); !strings.Contains(view, "DIGITAL LOVE") {
		t.Fatalf("the name input does not name the song it holds:\n%s", view)
	}
	// esc goes back to the picker.
	m, _ = press(t, m, "esc")
	if m.editor.mode != editPick {
		t.Fatalf("esc: editor %v; want the picker back", m.editor.mode)
	}
	m, _ = press(t, m, "enter")
	m = typeText(t, m, "Robots")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	if calls := callsOf(f, "CreatePlaylist"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{"Robots", "", []string{"s2"}}) {
		t.Fatalf("CreatePlaylist calls = %v; want Robots with s2", calls)
	}
	if m.editor.mode != editClosed || m.top().kind != viewAlbum {
		t.Fatalf("editor %v top %v; want the SONG view back", m.editor.mode, m.top().kind)
	}
	if m.stations[m.stationCursor()].ID != "p.new" {
		t.Fatalf("the new playlist is not selected on the root: %v", m.stations)
	}
	// The picker now offers it.
	m, _ = press(t, m, "a")
	if !strings.Contains(plain(m), "ROBOTS") {
		t.Fatalf("the picker lacks the new playlist:\n%s", plain(m))
	}
}

func TestNewPlaylistFailureKeepsTheName(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"CreatePlaylist": errors.New("name is too long")}
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "up", "enter")
	m = typeText(t, m, "Mix")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	if m.editor.mode != editName || m.nameInput.Value() != "Mix" {
		t.Fatalf("editor %v name %q; want the input kept", m.editor.mode, m.nameInput.Value())
	}
	if view := plain(m); !strings.Contains(view, "CREATE FAILED // NAME IS TOO LONG") {
		t.Fatalf("no failure notice:\n%s", view)
	}
}

func TestLibraryEditHints(t *testing.T) {
	m := openSong(t, playbacktest.New(), 1)
	// L and A are the first to go from a narrow footer.
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	if got := hintsOf(m); !strings.Contains(got, "[L] LOVE") || !strings.Contains(got, "[A] ADD") {
		t.Fatalf("track footer %q; want L and A", got)
	}
	m, _ = press(t, m, "a")
	if got := hintsOf(m); !strings.Contains(got, "[ENTER] ADD") || !strings.Contains(got, "[ESC] CANCEL") {
		t.Fatalf("picker footer %q", got)
	}
	m, _ = press(t, m, "enter")
	if got := hintsOf(m); !strings.Contains(got, "[ENTER] CREATE") || !strings.Contains(got, "[CTRL+C] QUIT") {
		t.Fatalf("name footer %q", got)
	}
}

func TestLibraryEditGolden80x24(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		text string
	}{
		{"add_to_playlist", []string{"a", "down"}, ""},
		{"new_playlist", []string{"a", "enter"}, "Night Moves"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			m := withStations(t, openSong(t, f, 1), editableStations())
			m, _ = step(t, m, volumeMsg{level: 0.6})
			m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
			for range 12 {
				m, _ = step(t, m, tickMsg{gen: m.tickGen})
			}
			m, _ = press(t, m, tt.keys...)
			m = typeText(t, m, tt.text)
			assertGolden(t, tt.name+"_80x24.golden", ansi.Strip(m.View().Content))
		})
	}
}

func TestLibraryEditViewFitsEverySize(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{20, 5}, {40, 10}, {59, 15}, {80, 24}, {160, 50}} {
		f := playbacktest.New()
		m := withStations(t, openSong(t, f, 1), editableStations())
		m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
		for _, keys := range [][]string{nil, {"a"}, {"enter"}} {
			m, _ = press(t, m, keys...)
			m, _ = step(t, m, tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
			for i, l := range strings.Split(plain(m), "\n") {
				if w := ansi.StringWidth(l); w > sz.w {
					t.Errorf("%dx%d %v: line %d is %d cells: %q", sz.w, sz.h, keys, i, w, l)
				}
			}
		}
	}
}

// unknownOutcome is a write failure whose outcome is unknown, as the helper
// adapter reports a timed-out create or add.
type unknownOutcome struct{}

func (unknownOutcome) Error() string {
	return "addToPlaylist timed out: may or may not have been applied"
}
func (unknownOutcome) OutcomeUnknown() bool { return true }

func TestAddWithAnUnknownOutcomeClosesThePicker(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"helper timeout", unknownOutcome{}},
		{"call deadline", context.DeadlineExceeded},
		{"wrapped deadline", fmt.Errorf("add: %w", context.DeadlineExceeded)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			f.MethodErr = map[string]error{"AddToPlaylist": tt.err}
			m := withStations(t, openSong(t, f, 1), editableStations())
			m, _ = press(t, m, "a", "down") // NIGHT DRIVE
			m, cmd := press(t, m, "enter")
			m = settle(t, m, cmd)
			if m.editor.mode != editClosed {
				t.Fatal("an add of unknown outcome kept the picker armed for a blind retry")
			}
			if view := plain(m); !strings.Contains(view, "CHECK NIGHT DRIVE BEFORE TRYING AGAIN") {
				t.Fatalf("no unknown-outcome notice:\n%s", view)
			}
			if m.libraryWriting {
				t.Fatal("the write is still marked in flight")
			}
		})
	}
}

func TestCreateWithAnUnknownOutcomeRereadsThePlaylists(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"CreatePlaylist": unknownOutcome{}}
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "up", "enter")
	m = typeText(t, m, "Mix")
	m, cmd := press(t, m, "enter")
	m, reread := step(t, m, run(t, cmd))
	if m.editor.mode != editClosed || m.nameInput.Focused() {
		t.Fatal("a create of unknown outcome kept the name input armed")
	}
	if view := plain(m); !strings.Contains(view, "CHECK THE PLAYLISTS BEFORE TRYING AGAIN") {
		t.Fatalf("no unknown-outcome notice:\n%s", view)
	}
	// The playlist was created after all: the list read again shows it,
	// and it is selected.
	f.PlaylistsResult = append(stations(), playback.Playlist{ID: "p.mix", Name: "Mix", Editable: true})
	m = settle(t, m, reread)
	if n := len(callsOf(f, "Playlists")); n != 2 {
		t.Fatalf("Playlists called %d times; want the list read again", n)
	}
	if c := m.stationCursor(); c < 0 || m.stations[c].ID != "p.mix" {
		t.Fatalf("stations %v cursor %d; want the new Mix selected", m.stations, c)
	}
}

func TestCreateWithAnUnknownOutcomeIgnoresAnOlderNamesake(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"CreatePlaylist": unknownOutcome{}}
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "up", "enter")
	m = typeText(t, m, "Samurai") // the name of a playlist already listed
	m, cmd := press(t, m, "enter")
	m, reread := step(t, m, run(t, cmd))
	m = settle(t, m, reread)
	if c := m.stationCursor(); c != -1 {
		t.Fatalf("cursor %d; want the + NEW PLAYLIST row kept, not the older Samurai", c)
	}
}

func TestEditorStaysBusyUntilTheWriteAnswers(t *testing.T) {
	f := playbacktest.New()
	m := withStations(t, openSong(t, f, 1), editableStations())
	m, _ = press(t, m, "a", "down")
	m, add := press(t, m, "enter") // NIGHT DRIVE, in flight
	// Within the editor: + NEW PLAYLIST opens, but creates nothing.
	m, _ = press(t, m, "up", "enter")
	if m.editor.mode != editName {
		t.Fatalf("editor %v; want the name input", m.editor.mode)
	}
	m = typeText(t, m, "Mix")
	m, cmd := press(t, m, "enter")
	if cmd != nil {
		t.Fatal("a create started while an add was in flight")
	}
	if view := plain(m); !strings.Contains(view, "WRITING") {
		t.Fatalf("no WRITING notice:\n%s", view)
	}
	// Back in the picker, no second add.
	m, _ = press(t, m, "esc", "down")
	if m, cmd = press(t, m, "enter"); cmd != nil {
		t.Fatal("a second add started while one was in flight")
	}
	// Closed and opened again, still none.
	m, _ = press(t, m, "esc", "a", "down")
	if m, cmd = press(t, m, "enter"); cmd != nil {
		t.Fatal("a reopened picker started an add while one was in flight")
	}
	m = settle(t, m, add)
	if n := len(callsOf(f, "AddToPlaylist")) + len(callsOf(f, "CreatePlaylist")); n != 1 {
		t.Fatalf("%d writes; want the one", n)
	}
	// Answered: writes start again.
	m, _ = press(t, m, "a", "down")
	if _, cmd = press(t, m, "enter"); cmd == nil {
		t.Fatal("no add after the write in flight answered")
	}
}

func TestRapidLoveTogglesAreSerialized(t *testing.T) {
	f := playbacktest.New()
	m := openSong(t, f, 1) // DIGITAL LOVE (s2)
	m, first := press(t, m, "l")
	m, second := press(t, m, "l")
	if second != nil {
		t.Fatal("a second favorite write started while one was in flight")
	}
	if on, _ := m.favoriteOf("s2"); on {
		t.Fatal("the row does not show the latest intent (unloved)")
	}
	m, next := step(t, m, run(t, first))
	if next == nil {
		t.Fatal("the queued intent was not sent once the first write answered")
	}
	m = settle(t, m, next)
	calls := callsOf(f, "SetFavorite")
	if len(calls) != 2 || !reflect.DeepEqual(calls[0].Args, []any{"s2", true}) || !reflect.DeepEqual(calls[1].Args, []any{"s2", false}) {
		t.Fatalf("SetFavorite calls = %v; want love, then unlove", calls)
	}
	if on, known := m.favoriteOf("s2"); on || !known {
		t.Fatalf("favorite s2 = %v known %v; want unloved", on, known)
	}

	// Three presses end where the first write goes: nothing more is sent.
	m, first = press(t, m, "l")
	m, _ = press(t, m, "l", "l")
	m, next = step(t, m, run(t, first))
	if next != nil {
		t.Fatal("a write was queued for the state already sent")
	}
	if on, _ := m.favoriteOf("s2"); !on || len(callsOf(f, "SetFavorite")) != 3 {
		t.Fatalf("calls %v; want three writes and s2 loved", callsOf(f, "SetFavorite"))
	}
}

func TestAFailedWriteStillSendsTheQueuedIntent(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"SetFavorite": errors.New("ratings unavailable")}
	m := openSong(t, f, 1)
	m, first := press(t, m, "l")
	m, _ = press(t, m, "l")
	f.MethodErr = nil
	m, next := step(t, m, run(t, first))
	if next == nil {
		t.Fatal("the queued intent was dropped with the failed write")
	}
	m = settle(t, m, next)
	if on, known := m.favoriteOf("s2"); on || !known {
		t.Fatalf("favorite s2 = %v known %v; want the latest intent, unloved", on, known)
	}
}

func TestFailedFavoriteReadsAreRetriedAfterABackoff(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"Favorite": errors.New("offline")}
	pageReadFails(f)
	c := newClock()
	m := openSong(t, f, 1)
	m.now = c.now
	m, cmd := m.readFavorites()
	m = settle(t, m, cmd)
	if _, cmd = m.readFavorites(); cmd != nil {
		t.Fatal("a failed read was retried at once")
	}
	c.advance(favoriteRetryAfter - time.Second)
	if _, cmd = m.readFavorites(); cmd != nil {
		t.Fatal("a failed read was retried before the backoff")
	}
	c.advance(time.Second)
	delete(f.MethodErr, "Favorite")
	m, cmd = m.readFavorites()
	if cmd == nil {
		t.Fatal("a failed read was never retried")
	}
	m = settle(t, m, cmd)
	if _, known := m.favoriteOf("s2"); !known {
		t.Fatal("the retried read did not settle")
	}
}

func TestAuthorizationClearsFailedFavoriteReads(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"Favorite": errors.New("not authorized")}
	pageReadFails(f)
	m := openSong(t, f, 1)
	m, cmd := m.readFavorites()
	m = settle(t, m, cmd)
	m, _ = step(t, m, authMsg{status: playback.AuthAuthorized})
	if _, cmd = m.readFavorites(); cmd == nil {
		t.Fatal("a read failed before authorization was not retried after it")
	}
}

func TestEscBackStaysInTheFooterAt80Columns(t *testing.T) {
	playlist := func(t *testing.T, f *playbacktest.Fake) Model {
		return openStation(t, loaded(t, f, newClock()), 0)
	}
	results := func(t *testing.T, f *playbacktest.Fake) Model { return openResults(t, f, &fakeRecents{}) }
	song := func(t *testing.T, f *playbacktest.Fake) Model { return openSong(t, f, 1) }
	for _, tt := range []struct {
		name string
		open func(*testing.T, *playbacktest.Fake) Model
	}{
		{"artist", openDaftPunk},
		{"playlist", playlist},
		{"song", song},
		{"results", results},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.open(t, playbacktest.New())
			if got := hintsOf(m); !strings.Contains(got, "[ESC] BACK") {
				t.Fatalf("80-col footer %q lacks [ESC] BACK", got)
			}
		})
	}
}

func TestReturningFromThePickerGivesTheSearchInputItsKeysBack(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = step(t, m, stateMsg{state: playing(time.Minute, 3*time.Minute)})
	if !m.input.Focused() {
		t.Fatal("setup: the SEARCH input lacks the keys")
	}
	// From the player, a opens the picker for the song playing.
	m, _ = click(t, m, zonePlay)
	if m.focus != areaPlayer || m.input.Focused() {
		t.Fatalf("setup: focus %v input focused %v; want the player", m.focus, m.input.Focused())
	}
	m, _ = press(t, m, "a")
	if m.editor.mode != editPick || m.input.Focused() {
		t.Fatalf("editor %v input focused %v; want the picker with the keys", m.editor.mode, m.input.Focused())
	}
	m, _ = press(t, m, "esc")
	if m.editor.mode != editClosed || !m.input.Focused() {
		t.Fatalf("editor %v input focused %v; want the SEARCH input typing again", m.editor.mode, m.input.Focused())
	}
}
