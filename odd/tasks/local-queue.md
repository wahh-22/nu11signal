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
- [x] Q1 — Diagnostics: helper stderr appended to a log file under ~/Library/Logs/nu11signal/; decode `playSongs` result in Go (`missing`, `skipped`, `startedAlone`) and report on the status line (e.g. `2 SONGS SKIPPED`). Route: delegated writer (Go client + TUI + main).
- [x] Q2 — Spike: does MusicKit accept a queue mixing catalog songs with library-copy Song items (or all library items) without Code=6? Route: delegated, spike/ only, consented playback.
- [ ] Q3 — Fix per Q2 result: stop dropping local songs. Route: TBD.

## Acceptance criteria
- Playing "Canciones favoritas" from PLAY queues all 6 songs; NEXT advances through all of them.
- Any song still left out is reported on the status line and in the log.
- `make test` (go + swift) green; vet/gofmt clean.

## Delivery
Forecast ~350 authored lines; ask-on-risk. Branch `fix/local-queue` stacked on `feat/rain-rhythm` (rain slice 0a214c3 unreviewed, under budget).

## Progress
- Created 2026-09-30.
- Q1 done (route: delegated writer), commit `c882966`. Port `PlaySongs` returns `playback.QueueReport{Missing, Skipped []string; StartedAlone bool}`; helper `queueResult` decodes it; TUI `queueNotice`: `PLAYING ALONE // QUEUE REFUSED` or `N SONGS SKIPPED // NOT IN QUEUE`, latest play only. Helper stderr appended to ~/Library/Logs/nu11signal/helper.log (0700/0600, rotated to .1 over 1 MB at startup). RED: build failures on new symbols; GREEN: `go test -race ./...` 874 passed, vet/gofmt clean.
- Review (slice c650bbb..c882966: rain R1 + Q1): high, 714 lines, consent granted, lineage `review-5b6ebe62ec1b629e`, 4 lenses, APPROVED, acknowledged (burned). Boundary now c882966. Advisory follow-ups: log grows unbounded within a session; log fd not closed if helper start fails; queueResult doc mentions playPlaylist whose report is dropped; stat-error nil deref in helperlog_test; rain pulse cutoff constant and rainBeat wait doc.
- Q2 spike done (route: delegated, spike/local-queue/, ~14 s consented playback). Cause confirmed: 5 of 6 songs local, only #0 catalog-only; playSongs returned skipped=5, NEXT stayed. Findings: the start song decides the queue type (catalog-only start → catalog queue; local start → library queue even via its catalog item); mixed initial queue with catalog start → Code=6; library queue silently drops catalog songs (initial or insert); catalog start alone then `queue.insert(rest, .tail)` with library copies for local songs → 6/6 and NEXT reaches last. Device playlist ("Favourite Songs", 5 songs, other order) and `Queue(playlist:)` unusable.
- Q3 plan: catalog-only start → queue start alone, play, insert followers at tail (library copy for local, catalog otherwise); local start → library copies, catalog-only followers reported skipped; after queueing compare `player.queue.entries` with the request and report silent drops. Route: delegated writer (helper Swift + tests).
