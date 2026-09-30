# Feature: wakeups

Locator: `odd/tasks/wakeups.md` · Engram mirror: `odd/wakeups/tasks` · Branch: `perf/wakeups`

## Objective

Reduce the TUI's interrupt wakeups (measured 2026-09-30 on main 7e15019: ~334/s idle, ~420/s playing, independent of effects) to save power without visible regressions.

## Tasks

- [x] W1 — Profile the wakeup sources (Bubbletea v2 renderer frame ticker, input reader, Go runtime, our tick scheduling), fix the dominant ones (e.g. renderer FPS matched to our animation needs, idle ticks coalesced), and re-measure with the same harness (idle / playing effects on / playing calm). Route: delegated.

## Checks

- `go test -race ./...`, `go vet ./...`, `gofmt -l .`; before/after wakeup and CPU table.

## Progress

- Branch `perf/wakeups` from `main` `7e15019`. Measurement harness: scratchpad `measure2.py` / `analyze.py`.
- W1 done (route: delegated writer). Attribution: Bubble Tea v2.0.10 renderer ticks at 60 fps even when idle (~3 wakeups/frame); the 10 fps playing tick costs ~22 wakeups/tick (sysmon polling grows with per-frame work); GC, helper client, input reader negligible. Changes: `radio.RenderFPS = 20` via `tea.WithFPS`; footer hints styled once (`fitHints`); panel borders styled once → View() 583 µs → 276 µs, 2852 → 1218 allocs. Tests added with the code (no strict RED): fitHints equivalence, FPS vs animation steps, paused EQ freezes. `go test -race ./...` 728 passed, vet/gofmt clean. TUI before → after (pid-tracked harness): idle interrupts 329 → 132/s, CPU 0.64% → 0.27%; playing effects 463 → 343/s, 1.83% → 1.45%; playing calm 475 → 347/s. Remaining playing cost is the 10 fps EQ (~200/s).
