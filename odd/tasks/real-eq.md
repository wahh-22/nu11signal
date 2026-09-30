# Feature: real-eq

Locator: `odd/tasks/real-eq.md` · Engram mirror: `odd/real-eq/tasks` · Branch: `feat/real-eq` (stacked on `feat/glitch-smooth`)

## Objective

The EQ bars are decorative; the user wants them to follow the music. Now that the helper captures the real audio through the Core Audio tap (app volume mode), compute spectrum levels from those samples and drive the bars with them.

## Scope

- Helper: the IOProc copies samples into a lock-free ring buffer (no work in the RT thread); a non-RT timer (~15 Hz) runs a vDSP FFT, groups bins into N log-spaced bands (N = bar count requested by the TUI or a fixed 16–32 resampled in Go), applies dB scaling, fast attack / slow release smoothing, and emits a small `levels` event only while playing in app mode.
- Go/TUI: port gains a levels stream (or State field); the EQ renders real levels when available; decorative animation stays as the fallback in system-volume mode or when no levels arrive; bars fall to zero when paused.
- Resources: FFT 1024–2048 at ~15 Hz with vDSP; events small; measure CPU/wakeups.

## Tasks

- [ ] E1 — Real spectrum levels from the tap + TUI rendering + fallback + tests + measurement. Route: delegated.

## Constraints

- No allocation or locks in the IOProc; pure testable band mapping/smoothing in Nu11SignalProtocol and Go.
- Artifacts in English.
