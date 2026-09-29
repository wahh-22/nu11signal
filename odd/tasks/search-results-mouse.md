# Feature: search-results-mouse

Locator: `odd/tasks/search-results-mouse.md` · Engram mirror: `odd/search-results-mouse/tasks` · Branches (stacked to main): `fix/search-resume` → `feat/search-results` → `feat/mouse-controls`

## Objective

Polish catalog browsing after user testing: a full search results page, resuming the search branch where it was left, and mouse-friendly navigation with cyberpunk-styled buttons.

## Problem / Why

User feedback (2026-09-29) after testing the catalog-browse release:
1. Selecting a suggestion (neither artist nor song) only refills the input; it should run the search and show TOP RESULTS, ARTISTS, ALBUMS, SONGS and PLAYLISTS.
2. The UI is keyboard-only; it should be usable with the mouse, with buttons in the cyberpunk style.
3. After opening an artist from search, switching to the stations (playlists) and coming back to search starts over; it should return to where the user left (the artist view).

## Scope

- S1 — Search branch resume: leaving the search branch (tab/stations) keeps its frames; `/` (and the future SEARCH button) restores the exact frame and cursor left; `/` while inside the search branch returns to the SEARCH input with the term kept for editing.
- S2 — Results page: `SearchCatalog` also returns albums, playlists and top results (MusicCatalogSearchRequest types Artist/Album/Song/Playlist + `includeTopResults`); Enter on a suggestion or on the input opens a RESULTS view with sections TOP RESULTS, ARTISTS, ALBUMS, SONGS, PLAYLISTS (non-empty only); rows open the existing ARTIST/ALBUM/SONG/PLAYLIST views.
- S3 — Mouse: enable mouse events; clickable rows (click = select + activate), wheel scrolling, clickable search input, MORE, retry; cyberpunk buttons: nav tabs (STATIONS / SEARCH), BACK, and transport (PREV / PLAY-PAUSE / NEXT); click on the progress bar seeks. Hit-testing via `github.com/lrstanley/bubblezone`.

## Constraints

- Keyboard behavior stays; mouse is additive. Compact/tiny layouts must not break.
- Text from the catalog stays sanitized; widths use cell widths.
- Artifacts in English.

## Delivery

- Strategy: `ask-on-risk`; chain strategy: `stacked-to-main` (reused from catalog-browse, stated to the user). Forecast: ~1600 lines (S1 ~250, S2 ~700, S3 ~650).

## Tasks

- [ ] S1 — Search branch resume. Branch `fix/search-resume`. Route: delegated (writer trigger: 2+ non-trivial files).
- [ ] S2 — Search results page with top results, artists, albums, songs, playlists. Branch `feat/search-results`. Route: delegated.
- [ ] S3 — Mouse support and cyberpunk buttons. Branch `feat/mouse-controls`. Route: delegated.

## Acceptance criteria

- Artist opened from search → tab to stations → `/` shows that artist view with the same cursor.
- Enter on a suggestion shows the RESULTS page with its non-empty sections; each row opens its view; `esc` returns to SEARCH.
- Every action reachable by keyboard that matters for browsing/playing is reachable by mouse; buttons render in the neon style.

## Checks

- `go test -race ./...`, `go vet ./...`, `gofmt -l .`; `swift build` + `swift test` in `helper/` when Swift changes.
- Manual: `make build && ./bin/nu11signal` walkthrough.

## Progress

- Branch `fix/search-resume` created from `main` `ccaadd9`.

## Next step

S1.
