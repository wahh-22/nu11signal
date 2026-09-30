# Feature: real-eq

Locator: `odd/tasks/real-eq.md` · Engram mirror: `odd/real-eq/tasks` · Branch: `feat/real-eq` (stacked on `feat/glitch-smooth`)

## Objective

The EQ bars are decorative; the user wants them to follow the music. Now that the helper captures the real audio through the Core Audio tap (app volume mode), compute spectrum levels from those samples and drive the bars with them.

## Scope

- Helper: the IOProc copies samples into a lock-free ring buffer (no work in the RT thread); a non-RT timer (~15 Hz) runs a vDSP FFT, groups bins into N log-spaced bands (N = bar count requested by the TUI or a fixed 16–32 resampled in Go), applies dB scaling, fast attack / slow release smoothing, and emits a small `levels` event only while playing in app mode.
- Go/TUI: port gains a levels stream (or State field); the EQ renders real levels when available; decorative animation stays as the fallback in system-volume mode or when no levels arrive; bars fall to zero when paused.
- Resources: FFT 1024–2048 at ~15 Hz with vDSP; events small; measure CPU/wakeups.

## Tasks

- [x] E1 — Real spectrum levels from the tap + TUI rendering + fallback + tests + measurement. Route: delegated.

## Constraints

- No allocation or locks in the IOProc; pure testable band mapping/smoothing in Nu11SignalProtocol and Go.
- Artifacts in English.

## Progress

- E1 done (route: delegated writer). Pre-gain mono samples into a lock-free SPSC ring (Synchronization.Atomic → macOS 15+, decorative on 14.x); 15 Hz utility timer while the IOProc runs: 2048 Hann, vDSP FFT, 24 log bands 40 Hz–16 kHz, −50..−10 dBFS, attack 0.7 / release 0.2; droppable `levels` event; Go optional `playback.LevelSource` (latest-only channel); TUI reads without blocking on the 10 fps tick, real bars only while playing with a reading < 500 ms old, else decorative/decay; demo stays decorative. RED/GREEN: Swift 191, Go 756. Measured (60 s playing, app mode): TUI 2.7% CPU (~580 interrupt wakeups/s vs 419 decorative), helper 0.6% (vs ~0.1%); bars follow structure (frame correlation 0.83 vs 0.25), bass > treble, quiet 2–3 rows vs loud 5–6. Open: bars average ~70% height (adaptive peak could help), extra TUI wakeups partly unexplained, helper Outbox wake per event.
- E1 review: commit `d729883`, RDD high, 1030 lines, consent granted, lineage `review-71471cd769e93d41`, 4 lenses, APPROVED, acknowledged (burned). Follow-ups (not scheduled): eqBarCount only knows the full layout (compact layout bar count may mismatch); stale pre-pause reading can flash once on resume; empty bands → treat as no reading; ring reset vs in-flight read; misleading test name; channel-pick test for AppVolume.capture.
