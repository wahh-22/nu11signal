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
- [ ] C2 — Recent-searches port + JSON file adapter; TUI navigation stack + SEARCH view (recent, suggestions, artist/song rows) wired to `SearchCatalog`; retire songs-only `search`. Branch `feat/catalog-search-view`. Route: delegated.
- [ ] C3 — Artist end to end: `ArtistDetail` (top songs, essential/featured albums, albums, playlists, singles & EPs, compilations, about: notes, genre, origin/formed best effort) + Swift `artist` + adapter + ARTIST view (MORE toggle). Branch `feat/catalog-artist`. Route: delegated.
- [ ] C4 — Song/album end to end: `AlbumDetail` + Swift `album`/`songAlbum`/`playAlbum` + adapter + SONG/ALBUM view (song highlighted, play from track). Branch `feat/catalog-song`. Route: delegated.
- [ ] C5 — README/keys docs, golden refresh, live smoke test. Branch `feat/catalog-docs`. Route: delegated.

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

## Next step

Start C1.
