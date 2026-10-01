# Update check

## Objective
On launch, find out whether a newer release exists and, if so, tell the user in the TUI with the command to update (user request 2026-10-01).

## Design
- New package `internal/update`: `Checker` port (`Latest(ctx) (Release, error)`) with a GitHub adapter (GET https://api.github.com/repos/wahh-22/nu11signal/releases/latest, 3 s timeout, User-Agent nu11signal/<version>, tag `vX.Y.Z`), semver comparison against the version stamped at link time (`dev` builds never check), and a 24 h cache file next to config.json (`update.json`: checked_at, latest) so launches don't hit the API every time.
- Opt-out: config.json `"update_check": false` or env `NU11SIGNAL_NO_UPDATE_CHECK=1`.
- TUI: after boot, when a newer version is known, show a notice that stays until the user acts: the idle status line becomes `◢◤◢◤ UPDATE vX.Y.Z AVAILABLE // brew upgrade --cask nu11signal` (status messages still take precedence while shown), plus a line in the KEYS overlay or SETTINGS. Failures are silent (no network, rate limit). Only one network call per launch at most, never blocking startup.
- Install-method awareness: Homebrew command shown when the binary lives under a Homebrew prefix/Caskroom; otherwise point to the releases page URL.

## Tasks
- [ ] U1 — Update check package + cache + opt-outs + TUI notice + docs. Route: delegated writer.

## Progress
- Created 2026-10-01 on branch `feat/update-check` from main.
