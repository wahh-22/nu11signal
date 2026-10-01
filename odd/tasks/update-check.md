# Update check

## Objective
On launch, find out whether a newer release exists and, if so, tell the user in the TUI with the command to update (user request 2026-10-01).

## Design
- New package `internal/update`: `Checker` port (`Latest(ctx) (Release, error)`) with a GitHub adapter (GET https://api.github.com/repos/wahh-22/nu11signal/releases/latest, 3 s timeout, User-Agent nu11signal/<version>, tag `vX.Y.Z`), semver comparison against the version stamped at link time (`dev` builds never check), and a 24 h cache file next to config.json (`update.json`: checked_at, latest) so launches don't hit the API every time.
- Opt-out: config.json `"update_check": false` or env `NU11SIGNAL_NO_UPDATE_CHECK=1`.
- TUI: after boot, when a newer version is known, show a notice that stays until the user acts: the idle status line becomes `◢◤◢◤ UPDATE vX.Y.Z AVAILABLE // brew upgrade --cask nu11signal` (status messages still take precedence while shown), plus a line in the KEYS overlay or SETTINGS. Failures are silent (no network, rate limit). Only one network call per launch at most, never blocking startup.
- Install-method awareness: Homebrew command shown when the binary lives under a Homebrew prefix/Caskroom; otherwise point to the releases page URL.

## Tasks
- [x] U1 — Update check package + cache + opt-outs + TUI notice + docs. Route: delegated writer (writer trigger: 2+ non-trivial files across internal/update, internal/radio, cmd/nu11signal). Commit: pending (parent).

## Progress
- Created 2026-10-01 on branch `feat/update-check` from main.
- U1 implemented (uncommitted): `internal/update` (Release, Checker, GitHub adapter, Cached 24 h `update.json`, strict semver `Newer`/`Valid`, `UpgradeCommand`); `config.Config.UpdateCheck *bool` (`UpdateCheckOn`, kept by Save); `cmd/nu11signal` `openUpdates` (opt-outs: demo, non-semver version, `NU11SIGNAL_NO_UPDATE_CHECK=1`, `"update_check": false`, no config dir) passed to `runUI`, upgrade command from the resolved executable; radio `Options.Updates/Version/Upgrade`, check from Init (`checkReleaseCmd`), notice on the idle status line (accent bold) and an UPDATE line heading SETTINGS (chosen over KEYS: SETTINGS is the app-state panel; KEYS stays a pure binding list). Docs: usage.md "Update check", README mention.
- Decisions: a pre-release latest is never offered; equal core with a pre-release current is an update; on a failed refresh a stale cache is still answered; demo stays offline.
- Evidence: RED observed per package (undefined symbols / unknown fields) before implementation; GREEN: go build, go test -race ./..., go vet, gofmt -l (empty), make test-scripts (34 passed). Goldens unchanged.

## Next step
Parent: review, work-unit commit `feat(update): check for a newer release at launch`, RDD assess.
- U1 commit `2c9a380`. Review (main..2c9a380): high, 1146 lines, consent granted, 4 lenses, lineage `review-50ad023a936b5c25`, APPROVED, acknowledged (burned). Hardened after review (test-first): a save that leaves `update_check` unset keeps the user's value (a theme picked before the file loaded no longer drops the opt-out); the release URL is shown only when it is a plain https link into github.com/wahh-22/nu11signal, else the releases page (no terminal escapes from the API). Other advisories (not scheduled): Homebrew prefix detection only covers /opt/homebrew and Caskroom; a failing refresh is retried every launch (one request per launch at most); settings click-zone test wording.
