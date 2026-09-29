# Feature: catalog-browse

Locator: `odd/tasks/catalog-browse.md` · Engram mirror: `odd/catalog-browse/tasks` · Branches (stacked to main): `feat/catalog-search` → `feat/catalog-search-view` → `feat/catalog-artist` → `feat/catalog-song` → `feat/catalog-docs`

## Objective

Browse and play the full Apple Music catalog the way Apple Music does, keeping the Cyberpunk 2077 radio look: a SEARCH section with recent searches, live suggestions and mixed artist/song results; an ARTIST page (top songs, essential albums, albums, artist playlists, singles & EPs, compilations, about); and a SONG detail view (its album with the song highlighted).

## Problem / Why

Today only library playlists ("stations") and a songs-only search are reachable. There is no way to explore an artist or an album, and searches are not remembered.

## Scope

- Domain/port (`internal/playback`): `Artist`, `Album`, `SearchResults` (suggestions, artists, songs), `ArtistDetail` (sections + about), `AlbumDetail` (tracks + metadata); new `Player` methods; demo and fake adapters.
- Swift helper: `searchCatalog` (artists + songs + suggestions), `artist` (relationships + editorial notes), `album` / `songAlbum` detail, `playAlbum` starting at a track. Read-only commands stay off `playbackCommands`.
- Go helper adapter: wire types + commands, fake-helper scenarios.
- Recent searches: new port + file adapter at `os.UserConfigDir()/nu11signal/recent.json`, injected through `radio.Options`.
- TUI: navigation stack (search → artist → album/song, `esc` pops), SEARCH view, ARTIST view, SONG/ALBUM view, in the existing neon panel style.

## Constraints

- macOS 14+; MusicKit for Swift only (developer token via entitlement).
- "About": public MusicKit exposes `editorialNotes` + `genreNames` only. "From / Formed" is not documented publicly; it is attempted through `MusicDataRequest` with `extend=origin,bornOrFormed` and hidden when absent (graceful degradation, no hard dependency).
- `featuredAlbums` is not confirmed to equal Apple Music's "Essential Albums"; verify live and fall back if empty.
- Entity id args must not be named `id`/`cmd` (use `artistId`, `albumId`, `songId`).
- No artwork (terminal); artists/albums render as text rows.
- Artifacts in English.

## Delivery

- Strategy: `ask-on-risk`; chain strategy: `stacked-to-main` (user choice, 2026-09-29). Forecast: ~2000 authored lines (>400 budget).
- One PR per task (vertical slices), each branch stacked on the previous and merged to main in order.

## Tasks

- [x] C1 — Catalog search backend: domain `SearchResults` (suggestions, artists, songs) + `Player.SearchCatalog`; Swift `searchCatalog` (MusicCatalogSearchRequest artists+songs, MusicCatalogSearchSuggestionsRequest); Go adapter + fake-helper scenario; fake + demo. Branch `feat/catalog-search`. Route: delegated (writer trigger: 2+ non-trivial files).
- [x] C2 — Recent-searches port + JSON file adapter; TUI navigation stack + SEARCH view (recent, suggestions, artist/song rows) wired to `SearchCatalog`; retire songs-only `search`. Branch `feat/catalog-search-view`. Route: delegated.
- [x] C2f — C2 review advisories: Enter bypasses min runes (R3); stale results selectable during debounce (R3); run_test depends on host config dir (R3); cancel superseded searches (R4); minSearchRunes literal, station cursor top/root mix, listRows/listPanel dual dispatch, dead esc pop comment, shadowed receiver (R2). Branch `feat/catalog-search-view`. Route: delegated (writer trigger: 2+ non-trivial files).
- [x] C3 — Artist end to end: `ArtistDetail` (top songs, essential/featured albums, albums, playlists, singles & EPs, compilations, about: notes, genre, origin/formed best effort) + Swift `artist` + adapter + ARTIST view (MORE toggle). Branch `feat/catalog-artist`. Route: delegated.
- [ ] C4 — Song/album/playlist end to end: `AlbumDetail`/`PlaylistDetail` + Swift `album`/`songAlbum`/`catalogPlaylist` + adapter + ALBUM/SONG and PLAYLIST views (song highlighted, play from track via playSongs). Plus C3 advisories: artist lookup deadline + timeout budget coherence (R4/R3 WARNING), strip control chars from decoded notes (R1), lazy featuredPlaylists fallback, `facts` naming, explicit itemPlaylist case, no shadowed results, unused artistItem.song, demo artistPage guards/magic year, Swift artist validation testability. Branch `feat/catalog-song`. Route: delegated.
- [ ] C5 — Extract Swift term/limit validation into a testable pure helper + tests (C1 advisory); README/keys docs, golden refresh, live smoke test. Branch `feat/catalog-docs`. Route: delegated.

## Acceptance criteria

- Typing in SEARCH shows suggestions and mixed Artist/Song rows; submitted terms appear under RECENT and persist across runs.
- Opening an artist shows the non-empty sections in Apple Music order plus ABOUT (notes, genre, and origin/formed when available).
- Selecting a song opens its album with the song highlighted; enter plays from that track.
- `esc` returns to the previous view; existing stations/playback keep working.

## Checks

- `go test -race ./...`, `go vet ./...`, `gofmt -l .`
- `swift test` in `helper/`
- Manual: live search "queen", open the artist, open a song, play.

## Progress

- Exploration done (architecture map + MusicKit research). Branch `feat/catalog-browse` created (renamed `feat/catalog-search` for slice 1).
- C1 done (route: delegated writer; trigger: writer, 2+ non-trivial files). RED: compile failure on missing `SearchCatalog`, then assertion failures with stub returns (6 tests); GREEN: `go test -race ./...` 151 passed, `go vet` clean, `gofmt -l .` empty, `swift build` OK, `swift test` 34 passed. Parent spot check: `go test -race ./...` 151 passed. MusicKit: `MusicCatalogSearchSuggestionsRequest(term:)` + `suggestions.map(\.searchTerm)`, limit capped at 10; suggestions failure degrades to empty. Open (verify in C5 smoke): suggestion cap, artist `genreNames` presence.
  - Commit `5a2e7df` `feat(search): add catalog search for artists, songs and suggestions`. RDD assess (base `112b5ee`, committed-only): risk medium, 394 lines, `review_due=false` (`under_budget`) → pending in slice; reviewed boundary stays `112b5ee`.
  - Native review (hook-prompted, consent granted): lineage `review-03156733c6d971ac`, lens review-reliability, APPROVED and acknowledged (authority burned); reviewed boundary → `647a796`. Advisories: R3-demo-contract-divergence (demo empty term / limit<=0 differ from helper) → folded into C2; R3-swift-searchcatalog-untested (move term/limit validation into a testable helper) → C5.
- C2 done (route: delegated writer). RED: history 8 failing tests with stubs; radio compile failure then 5 failing tests; demo contract RED on `ErrEmptyTerm`/limit. GREEN: `go test -race ./...` 192 passed (parent spot check identical), `go vet` clean, `gofmt -l .` empty, `swift build`+`swift test` 34 passed. Slicing (one pass): `06266a9` `feat(history): add persistent recent-searches store` (327 lines, branch `feat/catalog-recents`) and `641ab39` `feat(radio): add Apple Music style search view with recent searches` (~1514 lines, branch `feat/catalog-search-view`; size:exception — search view + its tests is not cohesively splittable further).
  - RDD assess (base `647a796`): high, 1841 lines, review_due (`high_risk`). Consent granted; lineage `review-68de54d2d80eba15`, 4 lenses, APPROVED, acknowledged (burned). First capture launch failed preflight (my shell variable collapsed the tokens; no mutation), relaunched after STATUS reoffered the same slots. Reviewed boundary → `641ab39`. 9 advisories → C2f.
- C2f done (route: delegated writer). RED: 4 new radio tests (Enter below min, hidden rows selectable, stale rows selectable, 5 cancellation subtests) + run test failing with HOME unset. GREEN: `go test -race ./...` all ok (parent spot check), `go vet` clean, `gofmt -l .` empty. Tab back into a cancelled search restarts it. Commit `f43a026`. RDD assess (base `641ab39`): high, 359 lines, due; consent granted; lineage `review-19e25e32802213fb`, 4 lenses, APPROVED, acknowledged (burned). Reviewed boundary → `f43a026`. Advisories (folded into C3): cancellation test asserts m.status instead of m.search.err (R3); stale-results rule repeated in searchRows/searchCode/searchBody (R2); stop+reset sequence duplicated in openSearch/inputChanged/searchNow (R2).
- C3 done (route: delegated writer). RED: demo/fake/adapter/Swift (6)/TUI (7) failures against stubs; GREEN: `go test -race ./...` 238 passed (parent spot check), `go vet` clean, `gofmt` empty, `swift test` 42 passed. MusicKit: topSongs, featuredAlbums (Essential), fullAlbums, singles, compilationAlbums, playlists → featuredPlaylists fallback; per-section `with` + 8s deadline degrade to empty; origin/bornOrFormed via MusicDataRequest extend (best effort). Go artist call timeout 15s. ARTIST view widens the left panel. Slicing (one pass): `695d9e7` `feat(artist): load artist pages from the catalog` (~645 lines, branch `feat/catalog-artist-backend`) and `05d84e5` `feat(radio): add Apple Music style artist page` (~1120 lines, branch `feat/catalog-artist`; size:exception — view + tests).
  - RDD assess (base `634c8f7`): high, 1766 lines, due; consent granted; lineage `review-e6eef9b46c10cfc9`, 4 lenses, APPROVED, acknowledged (burned). Reviewed boundary → `05d84e5`. 12 advisories → C4 (see C4 item list).

## Next step

C4 on `feat/catalog-song` (stacked on `feat/catalog-artist`).
