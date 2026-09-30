# Local queue

## Objective
Playing a library playlist whose songs live in the Mac's local library must queue every playable song, so NEXT always advances and no song is silently left out.

## Problem / why
- `PreparedQueue` (helper `SongQueue.swift:88-103`) drops every local-library song when the start song is not local, to avoid MusicKit Code=6 on mixed queues. In "Canciones favoritas" this leaves a one-song queue: NEXT does nothing. When only the last song is local, it is the one left out.
- The Go client discards the `playSongs` result (`missing`, `skipped`, `startedAlone`) and the helper's stderr only goes to a 4 KB buffer, so the user never learns songs were dropped.

## Scope
Diagnostics: helper stderr to a log file, `playSongs` result decoded and shown on the status line. Fix: queue local songs through their library copies (pending spike), so none must be dropped.

## Constraints
No library writes in automated tests; no system volume changes; spike playback only with user consent (granted 2026-09-30 for a few seconds of "Canciones favoritas"); do not touch the user's running app. Conventional commits.

## Tasks
- [ ] Q1 — Diagnostics: helper stderr appended to a log file under ~/Library/Logs/nu11signal/; decode `playSongs` result in Go (`missing`, `skipped`, `startedAlone`) and report on the status line (e.g. `2 SONGS SKIPPED`). Route: delegated writer (Go client + TUI + main).
- [ ] Q2 — Spike: does MusicKit accept a queue mixing catalog songs with library-copy Song items (or all library items) without Code=6? Route: delegated, spike/ only, consented playback.
- [ ] Q3 — Fix per Q2 result: stop dropping local songs. Route: TBD.

## Acceptance criteria
- Playing "Canciones favoritas" from PLAY queues all 6 songs; NEXT advances through all of them.
- Any song still left out is reported on the status line and in the log.
- `make test` (go + swift) green; vet/gofmt clean.

## Delivery
Forecast ~350 authored lines; ask-on-risk. Branch `fix/local-queue` stacked on `feat/rain-rhythm` (rain slice 0a214c3 unreviewed, under budget).

## Progress
- Created 2026-09-30.
