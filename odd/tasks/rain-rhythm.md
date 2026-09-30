# Rain rhythm

## Objective
Make the rain visualizer read clearly as following the music, especially on flat, heavily compressed tracks where the bars still show the rhythm but the rain does not.

## Problem / why
- Drops fix speed and length at spawn and live for seconds: the rain shows the past, not the current beat.
- Compressed music moves in a narrow level range (e.g. 0.5–0.6), which the fixed energy curve maps to nearly constant rain.
- Hits only lift head color one or two steps and need a large rise over a 35% moving average, which flat music rarely produces.

## Scope
`internal/radio/viz_rain.go` and its tests (`viz_test.go`, goldens if affected). No helper, config or other visualizer changes.

## Constraints
Deterministic (seed + frames), no extra timers, negligible CPU, paused rain still holds, drizzle without readings unchanged, silence stays dry. Conventional commits.

## Tasks
- [ ] R1 — Adaptive per-band gain (running min/max per band, level normalized into the full scale, with a minimum span so noise is not amplified); live drops take their column's current speed each frame (smoothed); adaptive beat pulse on total spectral flux (threshold relative to recent flux) that briefly boosts the whole rain's speed and brightness for a couple of frames. Route: delegated writer (one non-trivial file + tests, needs preparation reading).

## Acceptance criteria
- A flat track (levels oscillating 0.50–0.60 with a periodic beat) produces clearly different rain density/speed at peaks vs troughs and a pulse on each beat.
- Silence stays dry; no readings still drizzles; paused holds.
- `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean.

## Delivery
Forecast ~300 authored lines; strategy ask-on-risk. Branch `feat/rain-rhythm` stacked on `feat/intro-smooth`.

## Progress
- Created 2026-09-30.
