# Local files

## Objective
Play the computer's own music files in nu11signal, on macOS and on Linux, next to Apple Music (user request 2026-10-01).

## Design (defaults chosen; adjustable)
- Library: folders from config `music_dirs` (default `~/Music`), scanned for mp3, flac, ogg/vorbis and wav. Each folder with audio becomes a station in PLAYLISTS under a LOCAL section (and `.m3u`/`.m3u8` files become playlists); tags (title, artist, album, duration) read from the files, falling back to the file name.
- Playback in Go: decoders (mp3, flac, ogg vorbis, wav) → one PCM pipeline → output with ebitengine/oto v3 (CoreAudio on macOS without cgo; ALSA on Linux with cgo). Software gain = the app's own volume (`VolumeApp`); FFT on the same PCM feeds the 24-band spectrum and the 64-point wave (`LevelSource`), so the rain reacts exactly to the file.
- Sources: a composite Player routes by id namespace (`local:` ids for files, Apple ids unchanged). macOS: Apple Music + local; Linux (or no helper): local only. Capabilities the local source lacks (catalog search, artist pages, favorites, playlist editing) are hidden or disabled for local items via a capability check, never shown as errors.
- Linux: the Go TUI builds and runs with the local backend; CI builds and tests on Linux. Packaging for Linux (release archives, Homebrew on Linux/AUR/deb) is a later task.

## Tasks
- [ ] L1 — `internal/playback/local`: scanner, tags, decoders, PCM pipeline, oto output, queue (next/prev/seek/repeat), volume gain, levels (FFT + wave), implementing playback.Player (+ LevelSource) with unsupported methods returning a sentinel. Route: delegated writer.
- [ ] L2 — Port capabilities (`ErrUnsupported`, a capabilities interface), composite router by id namespace, TUI hides unsupported controls and words messages per source, LOCAL section in PLAYLISTS. Route: delegated writer.
- [ ] L3 — Wiring: config `music_dirs` (user-owned on save), backend selection (macOS: helper + local; Linux/no helper: local), docs. Route: delegated writer.
- [ ] L4 — Linux: build/test in CI (GitHub Actions ubuntu, libasound2-dev), manual smoke notes, docs for Linux install from source. Route: delegated writer.

## Progress
- Created 2026-10-01 on branch `feat/local-files`. Mapping done (Player is monolithic; only LevelSource is optional; module cross-compiles for Linux; no audio deps yet).
