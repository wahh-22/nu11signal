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

- [x] S1 — Search branch resume. Branch `fix/search-resume`. Route: delegated (writer trigger: 2+ non-trivial files).
- [x] S2 — Search results page with top results, artists, albums, songs, playlists. Branch `feat/search-results`. Route: delegated.
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
- S1 done (route: delegated writer). RED: 8 new resume tests failed (e.g. `/ restored stack [0 1]; want [0 1 2]`). GREEN: `go test -race ./...` ok (parent spot check), `go vet` clean, `gofmt` empty. Design: `Model.parked []frame`; tab parks the branch, `/` or tab from stations restores it; parked loads keep running and settle into the parked frame; `/` on a detail page returns to SEARCH with the term; `esc` on SEARCH closes the branch (fresh next time). README keys updated. Commit `1c8b9df`. RDD: high, 375 lines, consent granted, lineage `review-0cb0f04f926847b7`, 4 lenses, APPROVED, acknowledged (burned); boundary → `1c8b9df`. Advisories (folded into S2): searchAgain fallback undocumented; rename openSearch to reflect restore; parked page failure status shown off-screen; no test for parked album/playlist settle; parkBranch should cancel previously parked frames it replaces.
- S2 done (route: delegated writer). RED: adapter round trip/sparse and demo tests (empty Top/Albums/Playlists), TUI behavioural failures (e.g. `top 1 input "abba"; want RESULTS`, parked failure reaching stations, replaced parked page kept loading), missing golden. GREEN: `go test -race ./...` ok (parent spot check), `go vet`, `gofmt` clean, `swift test` 55 passed. Live helper: "queen" limit 10 → suggestions 3, top 6 (song, artist, song, album, song, playlist), artists 3, albums 9, songs 10, playlists 1. Decisions: dropdown limit 10, RESULTS limit 25, helper caps top results at 6; S1 advisories (a)–(e) fixed. Commits `54aaf67` (backend, branch `feat/search-results-backend`) and `0580ba9` (view, branch `feat/search-results`). RDD: high, 1291 lines, consent granted, lineage `review-00c7d8f7e70f0800`, 4 lenses, APPROVED, acknowledged (burned); boundary → `0580ba9`. Advisories (folded into S3): comment searchLimit vs resultsLimit; openable top-result kinds repeated; resultItems renders the whole layout to count items; untested short recent term branch; esc onto SEARCH re-runs the live search even when unchanged.

## Next step

S3 on `feat/mouse-controls`.
