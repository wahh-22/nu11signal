# Feature: wakeups

Locator: `odd/tasks/wakeups.md` · Engram mirror: `odd/wakeups/tasks` · Branch: `perf/wakeups`

## Objective

Reduce the TUI's interrupt wakeups (measured 2026-09-30 on main 7e15019: ~334/s idle, ~420/s playing, independent of effects) to save power without visible regressions.

## Tasks

- [ ] W1 — Profile the wakeup sources (Bubbletea v2 renderer frame ticker, input reader, Go runtime, our tick scheduling), fix the dominant ones (e.g. renderer FPS matched to our animation needs, idle ticks coalesced), and re-measure with the same harness (idle / playing effects on / playing calm). Route: delegated.

## Checks

- `go test -race ./...`, `go vet ./...`, `gofmt -l .`; before/after wakeup and CPU table.

## Progress

- Branch `perf/wakeups` from `main` `7e15019`. Measurement harness: scratchpad `measure2.py` / `analyze.py`.
