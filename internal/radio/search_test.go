package radio

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// fakeRecents is an in-memory history.Recents that records Add calls and
// can be made to fail.
type fakeRecents struct {
	mu      sync.Mutex
	terms   []string
	added   []string
	loadErr error
	addErr  error
}

func (r *fakeRecents) Load() ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.terms...), r.loadErr
}

func (r *fakeRecents) Add(term string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.added = append(r.added, term)
	return r.addErr
}

func (r *fakeRecents) Added() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.added...)
}

func catalog() playback.SearchResults {
	return playback.SearchResults{
		Suggestions: []string{"daft punk", "daft punk discovery"},
		Artists:     []playback.Artist{{ID: "a1", Name: "Daft Punk", Genres: []string{"Electronic"}}},
		Songs: []playback.Song{
			{ID: "s1", Title: "One More Time", Artist: "Daft Punk"},
			{ID: "s2", Title: "Digital Love", Artist: "Daft Punk"},
		},
	}
}

// loadedWithRecents is loaded with r as the recent-searches store, already
// read.
func loadedWithRecents(t *testing.T, f *playbacktest.Fake, r *fakeRecents) Model {
	t.Helper()
	f.PlaylistsResult = stations()
	m := New(f, Options{Now: newClock().now, Seed: 2077, Recents: r})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	m, _ = step(t, m, run(t, m.loadRecentsCmd()))
	return m
}

// searchFor opens the search view, types term and lets the debounced
// search complete.
func searchFor(t *testing.T, m Model, term string) Model {
	t.Helper()
	m, _ = press(t, m, "/")
	m = typeText(t, m, term)
	m, cmd := step(t, m, searchDebounceMsg{seq: m.search.seq})
	m, _ = step(t, m, run(t, cmd))
	return m
}

// settle runs cmd, including every command of a batch, and feeds the
// resulting messages back into the model.
func settle(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range runAll(t, cmd) {
		m, _ = step(t, m, msg)
	}
	return m
}

// plain is the rendered frame without styling, so that text split across
// styles (a highlighted prefix) still matches.
func plain(m Model) string { return ansi.Strip(m.render()) }

func catalogCalls(f *playbacktest.Fake) []playbacktest.Call {
	var out []playbacktest.Call
	for _, c := range f.Calls() {
		if c.Method == "SearchCatalog" {
			out = append(out, c)
		}
	}
	return out
}

func TestSearchShowsRecentTermsWhenEmpty(t *testing.T) {
	r := &fakeRecents{terms: []string{"queen", "daft punk"}}
	m := loadedWithRecents(t, playbacktest.New(), r)
	m, _ = press(t, m, "/")
	if m.top().kind != viewSearch {
		t.Fatal("/ did not open the search view")
	}
	view := plain(m)
	for _, want := range []string{"SEARCH", "RECENT", "QUEEN", "DAFT PUNK"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Index(view, "QUEEN") > strings.Index(view, "DAFT PUNK") {
		t.Errorf("recent terms not most recent first:\n%s", view)
	}
}

func TestSearchDebouncesAndDropsStaleAnswers(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "/")

	m = typeText(t, m, "d")
	if _, cmd := step(t, m, searchDebounceMsg{seq: m.search.seq}); cmd != nil {
		t.Fatal("a one-character term was searched")
	}
	m = typeText(t, m, "a")
	first := m.search.seq
	m, cmd := step(t, m, key("f"))
	var debounced bool
	for _, msg := range runAll(t, cmd) {
		if d, ok := msg.(searchDebounceMsg); ok && d.seq == m.search.seq {
			debounced = true
		}
	}
	if !debounced {
		t.Fatal("typing did not schedule a debounced search")
	}
	if len(catalogCalls(f)) != 0 {
		t.Fatal("typing searched before the debounce elapsed")
	}

	if _, cmd := step(t, m, searchDebounceMsg{seq: first}); cmd != nil {
		t.Fatal("a superseded debounce started a search")
	}
	m, cmd = step(t, m, searchDebounceMsg{seq: m.search.seq})
	if !m.search.loading {
		t.Error("search in flight is not marked loading")
	}
	answer := run(t, cmd)
	if got := catalogCalls(f); len(got) != 1 || !reflect.DeepEqual(got[0].Args, []any{"daf", searchLimit}) {
		t.Fatalf("SearchCatalog calls = %v; want one for \"daf\"", got)
	}

	// An answer to an older request never replaces the latest.
	stale := catalogMsg{seq: first, term: "da", results: playback.SearchResults{Songs: []playback.Song{{ID: "x", Title: "Stale Hit"}}}}
	m, _ = step(t, m, stale)
	if strings.Contains(plain(m), "STALE HIT") {
		t.Fatal("a stale answer was shown")
	}
	m, _ = step(t, m, answer)
	if m.search.loading || !strings.Contains(plain(m), "ONE MORE TIME") {
		t.Fatalf("latest answer not shown:\n%s", plain(m))
	}
}

func TestSearchResultsFollowAppleMusicOrder(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 160, Height: 40}) // room for full tags

	view := plain(m)
	order := []string{"DAFT PUNK DISCOVERY", "ARTIST · ELECTRONIC", "ONE MORE TIME", "SONG · DAFT PUNK"}
	last := -1
	for _, want := range order {
		i := strings.Index(view, want)
		if i < 0 {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
		if i < last {
			t.Fatalf("%q out of order:\n%s", want, view)
		}
		last = i
	}
	if n := len(m.searchRows()); n != 5 {
		t.Fatalf("search rows = %d; want 2 suggestions + 1 artist + 2 songs", n)
	}
}

func TestSearchCursorStopsAtInputAndLastRow(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = press(t, m, "up", "up")
	if m.top().cursor != -1 {
		t.Fatalf("cursor = %d above the first row; want the input (-1)", m.top().cursor)
	}
	for range 10 {
		m, _ = press(t, m, "down")
	}
	if m.top().cursor != 4 {
		t.Fatalf("cursor = %d past the last row; want 4", m.top().cursor)
	}
	// j and k are text in the search view.
	m = typeText(t, m, "jk")
	if got := m.input.Value(); got != "daftjk" {
		t.Fatalf("input = %q; want j and k typed", got)
	}
	if m.top().cursor != -1 {
		t.Fatal("typing did not return the cursor to the input")
	}
}

func TestEscPopsTheSearchViewButNeverTheRoot(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "/")
	m = typeText(t, m, "abc")
	m, _ = press(t, m, "esc")
	if m.top().kind != viewStations || len(m.stack) != 1 {
		t.Fatalf("esc left stack %v; want the stations root only", m.stack)
	}
	m, _ = press(t, m, "esc", "esc")
	if len(m.stack) != 1 || m.top().kind != viewStations {
		t.Fatalf("esc popped the root: %v", m.stack)
	}
	if strings.Contains(plain(m), "SEARCH") {
		t.Fatal("search panel still shown after esc")
	}
	if len(catalogCalls(f)) != 0 {
		t.Fatal("esc ran a search")
	}
}

func TestTabSwitchesBetweenStationsAndSearch(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")

	m, _ = press(t, m, "tab")
	if m.top().kind != viewStations {
		t.Fatal("tab did not return to stations")
	}
	// Player keys work again on the stations list.
	if _, cmd := press(t, m, "n"); cmd == nil {
		t.Fatal("n did nothing on the stations list")
	}
	m, _ = press(t, m, "tab")
	if m.top().kind != viewSearch || m.input.Value() != "daft" || !strings.Contains(plain(m), "ONE MORE TIME") {
		t.Fatalf("tab did not restore the last search:\n%s", plain(m))
	}
	// / does the same as tab; after esc closed the search, it is fresh.
	m, _ = press(t, m, "tab", "/")
	if m.top().kind != viewSearch || m.input.Value() != "daft" {
		t.Fatalf("/ did not restore the last search:\n%s", plain(m))
	}
	m, _ = press(t, m, "esc", "/")
	if m.input.Value() != "" || !strings.Contains(plain(m), "RECENT") {
		t.Fatalf("/ did not start a fresh search:\n%s", plain(m))
	}
}

func TestSearchShowsFailureAndNoResults(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		f := playbacktest.New()
		f.MethodErr = map[string]error{"SearchCatalog": errors.New("catalog offline")}
		m := searchFor(t, loaded(t, f, newClock()), "daft")
		if view := plain(m); !strings.Contains(view, "SCAN FAILED") || !strings.Contains(view, "CATALOG OFFLINE") {
			t.Fatalf("view lacks the failure:\n%s", view)
		}
	})
	t.Run("no results", func(t *testing.T) {
		f := playbacktest.New()
		m := searchFor(t, loaded(t, f, newClock()), "zzz")
		if view := plain(m); !strings.Contains(view, "NO SIGNAL") {
			t.Fatalf("view lacks the empty state:\n%s", view)
		}
	})
}

func TestRecentSaveFailureOnlyShowsStatus(t *testing.T) {
	f := playbacktest.New()
	r := &fakeRecents{addErr: errors.New("disk full")}
	m := loadedWithRecents(t, f, r)
	m, _ = press(t, m, "/")
	m = typeText(t, m, "queen")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	if !strings.Contains(plain(m), "DISK FULL") {
		t.Fatalf("status line lacks the save failure:\n%s", plain(m))
	}
	if m.top().kind != viewResults || m.recents[0] != "queen" {
		t.Fatal("a save failure disturbed the search")
	}
}

func TestWithoutRecentsStoreSearchStillRemembersInMemory(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock()) // Options.Recents is nil
	m, _ = step(t, m, run(t, m.loadRecentsCmd()))
	m, _ = press(t, m, "/")
	m = typeText(t, m, "queen")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	// Back to the search, closed, then a fresh one.
	m, _ = press(t, m, "esc", "esc", "/")
	if !strings.Contains(plain(m), "QUEEN") {
		t.Fatalf("recent term not shown:\n%s", plain(m))
	}
}

func TestSearchEnterBelowMinimumOnlyAsksToKeepTyping(t *testing.T) {
	f := playbacktest.New()
	r := &fakeRecents{}
	m := loadedWithRecents(t, f, r)
	m, _ = press(t, m, "/")
	m = typeText(t, m, "q")
	m, cmd := press(t, m, "enter")
	if cmd != nil {
		m = settle(t, m, cmd)
	}
	if calls := catalogCalls(f); len(calls) != 0 {
		t.Fatalf("SearchCatalog calls = %v; want none below %d characters", calls, minSearchRunes)
	}
	if got := r.Added(); len(got) != 0 {
		t.Fatalf("recents added = %q; want none", got)
	}
	if !strings.Contains(plain(m), "KEEP TYPING") {
		t.Fatalf("view lacks the keep-typing notice:\n%s", plain(m))
	}
}

func TestSearchRowsHiddenBelowMinimumAreNotSelectable(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	r := &fakeRecents{terms: []string{"a"}} // stored before the minimum existed
	m := loadedWithRecents(t, f, r)
	m, _ = press(t, m, "/", "down")
	m, cmd := press(t, m, "enter")
	if cmd != nil {
		m = settle(t, m, cmd)
	}
	if got := m.input.Value(); got != "a" {
		t.Fatalf("input = %q; want the recent term", got)
	}
	if calls := catalogCalls(f); len(calls) != 0 {
		t.Fatalf("SearchCatalog calls = %v; want none for a one-character term", calls)
	}
	// Results for a short term never become rows, even if they are there.
	m.search = searchState{term: "a", results: catalog()}
	if rows := m.searchRows(); len(rows) != 0 {
		t.Fatalf("search rows = %d below the minimum; want none", len(rows))
	}
	m, _ = press(t, m, "down")
	if m.top().cursor != -1 {
		t.Fatalf("cursor = %d; want the input (-1): no rows are shown", m.top().cursor)
	}
}

func TestSearchResultsForAnOlderTermAreNotSelectable(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")

	// Typing more: the "daft" results no longer answer the input, and the
	// debounced search has not run yet.
	m = typeText(t, m, "x")
	if rows := m.searchRows(); len(rows) != 0 {
		t.Fatalf("search rows = %d for an older term; want none", len(rows))
	}
	view := plain(m)
	if strings.Contains(view, "ONE MORE TIME") || !strings.Contains(view, "SCANNING") {
		t.Fatalf("view shows old results instead of scanning:\n%s", view)
	}
	m, _ = press(t, m, "down")
	if m.top().cursor != -1 {
		t.Fatalf("cursor = %d; want the input (-1)", m.top().cursor)
	}
	before := len(f.Calls())
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	for _, c := range f.Calls()[before:] {
		if c.Method == "PlaySongs" {
			t.Fatal("enter played a result of the older term")
		}
	}
	if calls := catalogCalls(f); calls[len(calls)-1].Args[0] != "daftx" {
		t.Fatalf("searched %v; want the typed term", calls[len(calls)-1].Args)
	}
}

// blockingPlayer hands every SearchCatalog context to the test and blocks
// until that context is done.
type blockingPlayer struct {
	*playbacktest.Fake
	ctxs chan context.Context
}

func (p *blockingPlayer) SearchCatalog(ctx context.Context, _ string, _ int) (playback.SearchResults, error) {
	p.ctxs <- ctx
	<-ctx.Done()
	return playback.SearchResults{}, ctx.Err()
}

func TestInFlightSearchIsCancelled(t *testing.T) {
	tests := []struct {
		name string
		act  func(t *testing.T, m Model) Model
	}{
		{"by the results page", func(t *testing.T, m Model) Model {
			m = typeText(t, m, "x")
			m, _ = press(t, m, "enter")
			return m
		}},
		{"by typing on", func(t *testing.T, m Model) Model { return typeText(t, m, "x") }},
		{"when the term drops below the minimum", func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "backspace", "backspace", "backspace")
			return m
		}},
		{"by esc", func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "esc")
			return m
		}},
		{"by tab to the stations", func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "tab")
			return m
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			f.PlaylistsResult = stations()
			p := &blockingPlayer{Fake: f, ctxs: make(chan context.Context, 4)}
			m := New(p, Options{Now: newClock().now, Seed: 2077})
			m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
			m, _ = press(t, m, "/")
			m = typeText(t, m, "daft")
			m, cmd := step(t, m, searchDebounceMsg{seq: m.search.seq})
			done := make(chan tea.Msg, 4)
			launch(cmd, done)
			var ctx context.Context
			select {
			case ctx = <-p.ctxs:
			case <-time.After(2 * time.Second):
				t.Fatal("the search never reached the player")
			}

			m = tt.act(t, m)
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("the superseded search was not cancelled")
			}
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("ctx err = %v; want cancelled", ctx.Err())
			}
			// The cancelled answer is dropped, not reported as a failure.
			for answered := false; !answered; {
				select {
				case msg := <-done:
					_, answered = msg.(catalogMsg)
					m, _ = step(t, m, msg)
				case <-time.After(2 * time.Second):
					t.Fatal("the cancelled search never answered")
				}
			}
			if m.search.err != nil {
				t.Fatalf("a cancelled search set the search error: %v", m.search.err)
			}
			if strings.Contains(m.status, "SCAN FAILED") || strings.Contains(plain(m), "SCAN FAILED") {
				t.Fatalf("a cancelled search was reported: status %q\n%s", m.status, plain(m))
			}
		})
	}
}

// launch runs cmd, and every command of a batch, in the background and
// sends their messages to out, so that a blocking command does not stall
// the test.
func launch(cmd tea.Cmd, out chan<- tea.Msg) {
	if cmd == nil {
		return
	}
	go func() {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				launch(c, out)
			}
			return
		}
		out <- msg
	}()
}

func TestReturningToAStoppedSearchResumesIt(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "/")
	m = typeText(t, m, "daft")
	// Leave before the debounced search runs: tab stops it.
	pending := searchDebounceMsg{seq: m.search.seq}
	m, _ = press(t, m, "tab")
	m, cmd := step(t, m, pending)
	if cmd != nil {
		t.Fatal("a debounce from before leaving started a search")
	}
	m, cmd = press(t, m, "tab")
	m = settle(t, m, cmd)
	if calls := catalogCalls(f); len(calls) != 1 || calls[0].Args[0] != "daft" {
		t.Fatalf("SearchCatalog calls = %v; want the stopped search resumed", calls)
	}
	if !strings.Contains(plain(m), "ONE MORE TIME") {
		t.Fatalf("resumed search not shown:\n%s", plain(m))
	}
}
