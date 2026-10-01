# Boot splash, quit buttons, quiet exit

## Objective
User feedback 2026-10-01: (1) quitting while music plays makes a pop like a mic yanked out; (2) the null emblem should be the TUI's own boot/loading screen at launch, with glitch from the start, not tied to the Apple Music link; (3) the quit modal's options should use the app's button style.

## Tasks
- [x] B1 — Boot splash: on launch the body shows the large emblem (compact/text fallbacks as today) for a short boot (~1.5 s, injected clock), glitching from the first frame (burst-style tears/noise over the emblem, deterministic from the seed), with a boot line under it (e.g. `B O O T I N G   N U 1 1 S I G N A L . . .`) instead of the LINKING line; then the normal UI, whether or not Apple Music is linked yet (LINKING stays in the header only). Respect `x`/`--calm` (no glitch when effects are off; the splash still shows). Any key during boot skips it. Route: delegated writer (with B2).
- [x] B2 — Quit modal buttons in the HUD bracket style: QUIT as the filled primary button (enter's action), STAY as a cyan bracket button, hover/focus like the player buttons. Route: delegated writer (with B1).
- [ ] B3 — Quiet exit: investigate and remove the pop when quitting during playback (fade the app volume out and pause before tearing down the tap/player). Route: exploration, then delegated writer.

## Progress
- Created 2026-10-01 on branch `feat/boot-splash` from main cdb479d.
- B1+B2 implemented by a delegated writer (uncommitted). Design: `internal/radio/boot.go` owns the boot (Model.boot, bootEnd set at the first size, bootDur 1.5 s on the injected clock; tickInterval cut to burstTick while the boot glitch draws and to the boot's end; a dedicated seeded glitch over the splash body; any key/click skips, q/ctrl+c also open the quit modal; refused access ends the boot at once and shows the auth error; `Options.SkipBoot` for tests). The auth-pending splash is gone: after boot the normal UI shows with LINKING in the header. Quit modal: `[ Y QUIT ]` (stButtonOn) and `[ N STAY ]` (stHi cyan), 2 cells apart, modal width 32, stacked when narrow.
- Goldens: `linking_80x24`/`linking_blue_80x24` replaced by `boot_80x24`/`boot_blue_80x24`; `quit_80x24` updated; NIGHT CITY ANSI baselines unchanged.
- Checks: `go build ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .` all clean.

## Next step
- Review/commit B1+B2 as a work unit, then B3 (quiet exit) exploration.
