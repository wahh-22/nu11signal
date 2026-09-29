# Feature: rename-nu11signal

Locator: `odd/tasks/rename-nu11signal.md` · Engram mirror: `odd/rename-nu11signal/tasks` · Branch: `feat/rename-nu11signal`

## Objective

Rename the project from soul-king to **Nu11Signal** (display name) / **nu11signal** (repo, binary, module, cask), release it as `v0.2.0`, and publish a Homebrew cask.

## Decisions

- Keep the bundle ID `dev.wahh.soulking.player`. Changing it would require a new App ID with MusicKit and regenerating both provisioning profiles. The bundle ID is not user-visible; the app name (CFBundleName) is, and it changes to Nu11Signal.
- Keep the keychain notary profile name `soulking-notary`; it is internal.
- `v0.1.0` stays published under the old name; the cask ships `v0.2.0`.
- Historical ODD documents (`odd/tasks/radio-tui.md`, `release-signing.md`) and `spike/` are not rewritten.

## Tasks

- [x] N0 — GitHub repo renamed `wahh-22/soul-king` → `wahh-22/nu11signal` (old URLs redirect); origin updated. Route: inline.
- [x] N1 — Code rename: Go module `github.com/wahh-22/nu11signal`, `cmd/nu11signal`, binary `nu11signal`, helper `Nu11SignalHelper.app` / `nu11signal-helper`, Swift package/targets (`Nu11SignalProtocol`, `Nu11SignalProtocolTests`), env vars `NU11SIGNAL_*`, UI wordmark `NU11SIGNAL`, Makefile, scripts, cask template `packaging/homebrew/nu11signal.rb.template`, README (with a "formerly soul-king" note), release archive `nu11signal-<ver>-macos-universal.tar.gz`. Profile defaults `signing/Nu11Signal_Dev.provisionprofile` / `signing/Nu11Signal_DeveloperID.provisionprofile`, falling back to the pre-rename names (and `spike/`) when missing. Route: delegated (writer trigger: many non-trivial files). Commit `3758765`.
- [x] N2 — Release `v0.2.0` (signed, notarized), verified as a quarantined download plus a live user check; GitHub release.
- [x] N3 — Homebrew tap `wahh-22/homebrew-tap` with `Casks/nu11signal.rb`; `brew audit` passes; `brew install --cask wahh-22/tap/nu11signal` works end to end.

## Acceptance criteria

- No `soul-king`/`soulking`/`SoulKing`/`SOUL KING` left in live code, build, docs, or UI (except the bundle ID, the notary profile default, historical ODD docs, and `spike/`).
- `make test`, `make vet`, `make fmt-check`, `make build`, and `make release-dry-run` pass.

## Progress

- Base: main `0f28376`.
- N1 done in `3758765` (41 files, +238/-218). Evidence:
  - `make test`: Go 136 tests passed (6 packages, -race); Swift 33 tests passed (`Nu11SignalProtocolTests`).
  - `make vet`: clean. `make fmt-check`: exit 0.
  - `make build`: `build/Nu11SignalHelper.app` signed with Apple Development, `codesign --verify --strict --deep` valid; embedded profile came from the legacy fallback `signing/SoulKing_Player.provisionprofile`; Info.plist CFBundleName `Nu11SignalHelper`, executable `nu11signal-helper`, bundle ID `dev.wahh.soulking.player`; `bin/nu11signal --version` → `dev`.
  - `make release-dry-run VERSION=0.2.0`: preflight picked `signing/SoulKing_Player_DeveloperID.provisionprofile` via fallback and `soulking-notary`; layout `dist/nu11signal-0.2.0/{bin/nu11signal, libexec/Nu11SignalHelper.app, LICENSE, README.md}`; `lipo -archs` both executables `x86_64 arm64`. Dry-run output removed; the existing v0.1.0 artifacts in `dist/` were left untouched (so `make clean` was not used).
  - Helper smoke (dev build, no audio): `authorize` → `{"status":"authorized"}`; `search` "daft punk" limit 2 → two songs.
  - Golden `view_80x24.golden` regenerated with `-update`: only the wordmark changed (`SOUL KING` → `NU11SIGNAL`, one padding space less).
  - `git grep -i 'soul.?king|soulking'` outside spike/ and historical ODD docs: only bundle ID defaults, `soulking-notary`, the legacy profile fallbacks, and the README "formerly soul-king" note.

- Local profiles renamed to the new defaults (`signing/Nu11Signal_Dev.provisionprofile`, `signing/Nu11Signal_DeveloperID.provisionprofile`); `make build` signs with them; tests green (Go 136, Swift 33).
- N1 native review (range `a0c6081..e9661a7`): tier high; consent granted; 4-lens review approved, receipt acknowledged (lineage `review-f45cf61dc4ffd80d`). Advisories (follow-ups): legacy `SOULKING_*` env vars are silently ignored (build.sh:34-37, locate.go:12) — documented as breaking in the v0.2.0 notes; duplicated legacy profile fallback (release.sh:52-55); profile fallback untested; CLI signing identifier change undocumented.

- PR #3 merged (main `326871f`). N2 build from `326871f`: notarization `b471d916-74f1-4b29-8eb0-cc19c8ca1ff7` accepted, helper stapled; archive sha256 `0bf3eb78773c73221fa0704bdfa4162e5803c735f540c0e7b868f8213bb0fedc`. Quarantined download check: sha256 OK, helper `spctl` accepted (Notarized Developer ID), `--version` 0.2.0, universal, helper authorizes + searches. User confirmed live audio.

- N2 published: tag `v0.2.0` → `326871f`; release https://github.com/wahh-22/nu11signal/releases/tag/v0.2.0 (latest) with archive + .sha256; the downloaded asset's sha256 matches.
- N3: public tap https://github.com/wahh-22/homebrew-tap with `Casks/nu11signal.rb` (0.2.0); `brew audit --cask --online --strict` exit 0; `brew install --cask wahh-22/tap/nu11signal` links `/opt/homebrew/bin/nu11signal` → Caskroom; `nu11signal --version` from /tmp → 0.2.0; installed helper `spctl` accepted (Notarized Developer ID). README install section updated.

- PR #4 merged (main `78aed45`).

## Follow-ups (branch `chore/release-followups`)

User decision (2026-09-29): nobody installed the old name, so there is no backward compatibility for `SOULKING_*` or the legacy profile names.

- [ ] F1 — Remove legacy fallbacks: old profile names and `spike/` in `helper/build.sh` and `scripts/release.sh`, plus any legacy mentions in the README (keep the "formerly soul-king" note). This resolves the duplicated-fallback advisory.
- [ ] F2 — `release.sh` builds into a temporary staging area and replaces `dist/nu11signal-<ver>*` only on success. `make clean` no longer deletes published archives: `clean` removes `bin/` and `build/`; a new `clean-dist` removes `dist/`.
- [ ] F3 — Tests for the `cmd/nu11signal` exit paths (review advisory, main.go:46-51).
- [ ] F4 — Automate cask bumps: `scripts/bump-cask.sh VERSION` (or `make cask VERSION=`) renders the template with the sha256 from `dist/…sha256` into a local checkout of `wahh-22/homebrew-tap` and runs `brew audit`; the commit and push stay explicit, with a documented `--push` flag.

Route: delegated (one writer; writer trigger: 2+ non-trivial files).

## Next step

F1–F4.
