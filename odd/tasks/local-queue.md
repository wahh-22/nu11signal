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
- [x] Q3 — Fix per Q2 result: stop dropping local songs. Route: delegated writer.

- [x] Q4 — Device test on list "Tocayo": NEXT never reaches the last song. Log: local start, 1825270819 (catalog-only "Dos de Corazón") left out, 3 entries queued, drop check inconclusive (entry ids differ in form → 1 s wait every play). Likely the catalog-only song comes after local ones, which a library queue cannot hold. Fix: play the list as consecutive segments (a run starting with a local song = library queue; a catalog-only start = startThenAppend); when the current segment ends (NEXT on its last entry, or natural end of its last song) the helper plays the next segment, and PREV on a segment's first entry goes back to the previous segment's last song. Inconclusive drop check returns at once instead of polling. Route: delegated writer (helper Swift + tests).

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
- Q3 done, commit `2b63126`. `PreparedQueue` → `QueuePlan` (`.startThenAppend(start, followers)` when a catalog-only start has local followers; `.upFront(items, start)` otherwise; local start reports catalog-only songs as skipped). `QueueCheck.dropped(submitted:queued:)` compares queue entries with submitted item ids (nil = inconclusive); helper polls entries 100 ms up to 1 s. `startedAlone` still only the Code=6 fallback. RED: `QueuePlan`/`QueueCheck` undefined; GREEN: `swift test` 203 passed, `go test -race ./...` ok, `make helper` signed.
- Review (slice c882966..2b63126, incl. spike): high, 694 lines, consent granted, lineage `review-9919e442e397fcc2`, 4 lenses, APPROVED, acknowledged (burned). Boundary now 2b63126. Advisory follow-ups: inconclusive drop check burns the full 1 s window (latency on every play if entry ids differ in form); songs before the start silently omitted in startThenAppend (PREV can't reach them, not reported); notQueued doc; poll sleep may overrun deadline; spike usage comment stale.
- Next: user tries "Canciones favoritas" from PLAY on a fresh `make build`; check helper.log for drop-check outcome.
- Q4 done (route: delegated writer), commit `41fff01`. Pure `QueueSegments(local:start:)` (local run around the start = library queue; catalog start with local followers runs to the end via startThenAppend; catalog start without local followers extends back over catalog songs), `advance`/`back` decisions (repeat-all wrap; back only on first entry under 3 s). Helper keeps the list; next/previous on segment edges play the neighbor segment; `SegmentEnd.ended(before:now:repeatMode:)` infers natural end (last entry within 2.5 s of end, then no/other entry or stopped), checked on state changes and 500 ms tick, queued on a shared `PlaybackChain`. Inconclusive drop check returns at once. RED: missing types, then 9 assertions on all-catalog start > 0; GREEN: `swift test` 220 passed, go tests ok, `make helper` signed.
- Review (slice 2b63126..41fff01): medium, 624 lines, slice_budget_reached, consent granted, single consolidated lens, APPROVED, acknowledged (burned). Boundary now 41fff01.
- Device checks pending: natural end on "Tocayo" (let song 3 end → song 4 plays), previous threshold, repeat-all across segments, drops on segment moves only logged.
- Device test 2026-09-30: "Tocayo" now reaches the last song. User asked NEXT on the last song to go to the first.
- [x] Q5 — NEXT pressed on the list's last song wraps to the first (any repeat mode, one or many segments); a natural end still stops unless repeat all. Route: inline (one pure decision + two call sites). RED: `extra argument 'pressed'`; GREEN: `swift test` 221 passed, `make helper` signed. Commit `7aedba1`; assess medium, 54 lines, under_budget → pending in slice (boundary 41fff01).
