# Linux spectrum for Apple Music

## Objective
Make the rain follow the real sound of Apple Music on Linux, as it already
does on macOS (helper Core Audio tap) and on Android TV (Visualizer), user
request 2026-10-11.

## Design
- Today on Linux the web player (headless Chrome + Widevine) exposes no
  audio, so the rain is decorative for Apple Music; local files already have
  real levels (`internal/playback/local` DSP: analyzer, smoother, decimate).
- Capture the PulseAudio monitor of the default sink (works with PipeWire
  through pipewire-pulse) with the pure-Go client `github.com/jfreymuth/pulse`
  (already in go.sum through oto), only while Apple Music plays, and feed the
  samples through the same DSP as local files (24 bands, 64-point wave), so
  the web player becomes a `playback.LevelSource`.
- Move the DSP out of `internal/playback/local` into a shared package
  (e.g. `internal/playback/spectrum`) so local, web player and Android share
  one implementation; desktop output must stay identical for local files.
- The monitor hears the whole system mix (like the TV). No permission prompt
  on Linux; document it. No PulseAudio/PipeWire, or capture failure:
  decorative rain, no error for the user.
- Verification: unit tests with synthetic PCM; a Docker Linux container with
  a PulseAudio daemon, a null sink and a test tone proves capture + spectrum
  end to end; Apple Music with Widevine needs a real Linux desktop.

## Tasks
- [ ] L1 — Share the DSP (`internal/playback/spectrum`), local player unchanged
  in behavior, tests.
- [ ] L2 — Pulse monitor capture -> web player LevelSource on Linux, started
  and stopped with playback, graceful fallback, tests.
- [ ] L3 — Docker end-to-end check (pulse daemon, null sink, tone) + docs
  (docs/audio.md Spectrum, docs/usage.md Linux).
- [ ] L4 — Real Linux desktop check with Apple Music (needs a Linux machine).

## Progress
- Created 2026-10-11 on branch `feat/linux-spectrum` from main 8e99dd3.
