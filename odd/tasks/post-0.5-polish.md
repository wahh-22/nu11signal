# Post-0.5 polish

## Objective
Close the follow-ups left after v0.5.0 (user request: items 1-4).

## Tasks
- [x] P1 (nu11signal-web, delegated writer): the site says it runs on Linux — hero chips, install commands for macOS (cask) and Linux (formula / archive), requirements copy.
- [x] P2 (nu11signal, delegated writer; then web): ROSE and NEON ROSE renders of the main screen (goldens + `tools/readmeart` screens), copied to the site so each theme shows its own render.
- [x] P3 (nu11signal, delegated writer): local playback on an ALSA system with no sound card no longer stalls silently — detect output that never starts or stops consuming and report it (status line / error) instead of a frozen player.
- [ ] P4 (both repos, delegated writers): recent review suggestions —
  - release.sh: check the ELF type, match the Linux version stamp exactly, `--linux-only` FORCE must not carry macOS files of another build (mixed provenance); bump-cask: clearer error when Linux checksums are missing.
  - `--version`: tests for `wordmarkAt` fallbacks; do not drop the version line silently.
  - a test that the README logo path exists.
  - docs/usage.md update-check wording (`/opt/homebrew` ambiguity) and the UpgradeCommand comment.
  - web: NEON ROSE link vs hover contrast; og-card inlined logo kept in step with public/logo (check or generator).

## Scope note
Older backlog advisories in other odd/tasks files stay out of scope.

## Checks
- nu11signal: `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `make test-scripts`, readmeart byte-stable.
- web: `npm run build`, `npm run check:links`, screenshots of touched sections.

## Progress
- P1 done (web, branch feat/linux-on-site, `a78006d`): install toggle macOS/Linux, chips, requirements. Checks: build, check:links, astro check, screenshots per theme at 1280/375.
- P2 done: app `871ea3a` (goldens + readmeart screens) and `378b5e4` (SVGs), reviewed together, approved (`review-f7cabaa80c8b0f02`); web `3300589` (renders + per-theme switching). Web P1+P2 reviewed together, approved (`review-fa783f75308e100e`).
- P3 done: `e93a92b` — bounded audio open (5 s) and a stall watchdog on the 500 ms state ticker report `playback.ErrNoOutput`; TUI shows NO AUDIO OUTPUT // IS PULSEAUDIO OR PIPEWIRE RUNNING?. Docker repro (ALSA file plugin on an unread FIFO) stalled before, errors after. Review high, 4 lenses, approved (`review-a05fd9459be7d8f2`); warnings folded into P4: error-text parsing in the TUI, restart needed after fixing audio, sticky failed context.
