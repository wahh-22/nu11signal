# Feature: signal-glitch

Locator: `odd/tasks/signal-glitch.md` · Engram mirror: `odd/signal-glitch/tasks` · Branch: `feat/signal-glitch`

## Objective

More Cyberpunk 2077 atmosphere (user request, 2026-09-30): the screen recurrently seems to lose signal, numbers and letters appear, and alert signals show — without hurting usability or resources.

## Scope

- Recurrent signal loss: every ~20–45 s (seeded random), a 0.2–0.6 s glitch burst: horizontal line shifts/tears, a few corrupted cells, static bars; occasionally a brief (<1 s) `NO SIGNAL` flash. Never longer.
- Data rain: subtle changing hex/codes/coordinates in free space only (header, borders, empty panel areas), never over readable content.
- Alerts: rotating Night City style alerts on the status line with a blinking `▲` (e.g. `▲ SIGNAL DEGRADED // RETUNING`, `▲ ICE TRACE DETECTED`, `▲ PACKET LOSS 37%`), yielding to real status messages.
- Controls: a key to toggle effects and a `--calm` launch flag (and demo keeps working); effects pause while typing in the SEARCH input.
- Resources: reuse the existing animation tick; only raise the frame rate during bursts; measure CPU.

## Constraints

- Deterministic under tests (seeded, injectable clock); goldens stay stable (effects off or seeded in goldens).
- Cell-width safe glyphs; readability first. Artifacts in English.

## Tasks

- [ ] G1 — Glitch engine + data rain + alerts + controls. Route: delegated (writer trigger: 2+ non-trivial files).

## Checks

- `go test -race ./...`, `go vet ./...`, `gofmt -l .`; CPU sample of `bin/nu11signal --demo`.

## Progress

- Branch `feat/signal-glitch` from `main` `ddf3b86`.
