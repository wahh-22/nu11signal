# Boot splash, quit buttons, quiet exit

## Objective
User feedback 2026-10-01: (1) quitting while music plays makes a pop like a mic yanked out; (2) the null emblem should be the TUI's own boot/loading screen at launch, with glitch from the start, not tied to the Apple Music link; (3) the quit modal's options should use the app's button style.

## Tasks
- [x] B1 — Boot splash: on launch the body shows the large emblem (compact/text fallbacks as today) for a short boot (~1.5 s, injected clock), glitching from the first frame (burst-style tears/noise over the emblem, deterministic from the seed), with a boot line under it (e.g. `B O O T I N G   N U 1 1 S I G N A L . . .`) instead of the LINKING line; then the normal UI, whether or not Apple Music is linked yet (LINKING stays in the header only). Respect `x`/`--calm` (no glitch when effects are off; the splash still shows). Any key during boot skips it. Route: delegated writer (with B2).
- [x] B2 — Quit modal buttons in the HUD bracket style: QUIT as the filled primary button (enter's action), STAY as a cyan bracket button, hover/focus like the player buttons. Route: delegated writer (with B1).
- [x] B3 — Quiet exit: investigate and remove the pop when quitting during playback (fade the app volume out and pause before tearing down the tap/player). Route: exploration, then delegated writer.

## Progress
- Created 2026-10-01 on branch `feat/boot-splash` from main cdb479d.
- B1+B2 implemented by a delegated writer (uncommitted). Design: `internal/radio/boot.go` owns the boot (Model.boot, bootEnd set at the first size, bootDur 1.5 s on the injected clock; tickInterval cut to burstTick while the boot glitch draws and to the boot's end; a dedicated seeded glitch over the splash body; any key/click skips, q/ctrl+c also open the quit modal; refused access ends the boot at once and shows the auth error; `Options.SkipBoot` for tests). The auth-pending splash is gone: after boot the normal UI shows with LINKING in the header. Quit modal: `[ Y QUIT ]` (stButtonOn) and `[ N STAY ]` (stHi cyan), 2 cells apart, modal width 32, stacked when narrow.
- Goldens: `linking_80x24`/`linking_blue_80x24` replaced by `boot_80x24`/`boot_blue_80x24`; `quit_80x24` updated; NIGHT CITY ANSI baselines unchanged.
- Checks: `go build ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .` all clean.

- B3 implemented by a delegated writer (uncommitted; checkbox stays open until confirmed by ear). Design: `ShutdownPlan` (helper/Sources/Nu11SignalProtocol/Shutdown.swift) owns the quiet exit's order and timings. With the app-volume IOProc rendering: fade the gain to 0 over `GainRamp.shutdownSeconds` (0.2 s, a separate slow increment in the render state, selected by a shared `fading` word) and wait 0.23 s, then `pause()` (never stop), wait up to 0.15 s for the player to report not playing, then teardown (IOProc stops on silence, aggregate then the still-muted tap destroyed), then exit. Without a rendering tap but playing: pause, settle 0.2 s, teardown. Idle: teardown. `Lifecycle.quietDown` runs it as a main-actor Task; AppVolume stops feeding the lifecycle and ignores level/permission changes once closing; the `_exit` backstop stays at 2 s (`ShutdownPlan.backstopSeconds`).
- Timeout chain documented once on `helper.DefaultCloseTimeout` (internal/helper/client.go): helper request grace 3 s + backstop 2 s (worst case 5 s) < client `DefaultCloseTimeout` 5.5 s (was 2 s) ≤ TUI `defaultCloseTimeout` 6 s (was 3 s). A Go test reads the helper's values from Shutdown.swift so they cannot drift.
- RED observed first: Swift `ShutdownPlanTests` (cannot find `ShutdownPlan`), Go `shutdown_budget_test.go` / `close_timeout_test.go` (undefined `helperRequestGrace`, `helper.DefaultCloseTimeout`). GREEN: `swift test` 230 tests, `go test -race ./...`, `go vet`, `gofmt -l .` clean, `make helper` built and signed.
- Pending: confirm by ear that quitting during playback (app volume and `NU11SIGNAL_VOLUME_MODE=system`) no longer pops.

## Next step
- Review/commit B1+B2 as a work unit; confirm B3 by ear, then review/commit it as its own work unit.
- B1+B2 commit `386713d`. Review (main..386713d): high, 770 lines, consent granted, 4 lenses, lineage `review-26f5f07588bb13ca`, APPROVED, acknowledged (burned). Advisories (not scheduled): tiny-layout boot swallows the first key; boot glitch precedence/naming; quit modal width literals; boot resize untested.
- B3 commit `2901c27`. Review (386713d..2901c27): medium, 417 lines, consent granted, consolidated lens, lineage `review-90fc7d166b5f0647`, APPROVED, acknowledged (burned). Advisory: if the player does not report paused within 0.15 s teardown proceeds anyway (gain is already 0, so silent by design); fading-word selection untested. Pending: user confirms by ear that the pop is gone.
- B3 confirmed by ear 2026-10-01: no pop on quit during playback.
- [x] B4 — User feedback: the boot glitch was too aggressive and flashed letters. Its noise is now shades/blocks only (`bootNoiseGlyphs`) and as many cells as a periodic burst (4..10, `bootNoiseMin`/`bootNoiseSpan`); tears and static bar already matched. Route: inline. RED: TestBootNoiseIsBlocksOnly undefined symbols; GREEN: `go test -race ./...` pass.
