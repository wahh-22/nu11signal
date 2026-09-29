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
- [ ] N2 — Release `v0.2.0` (signed, notarized), verified as a quarantined download plus a live user check; GitHub release.
- [ ] N3 — Homebrew tap `wahh-22/homebrew-tap` with `Casks/nu11signal.rb`; `brew audit` passes; `brew install --cask wahh-22/tap/nu11signal` works end to end.

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

## Next step

N2 — release `v0.2.0`. Optionally rename the local profile files to the new default names first (builds work either way).
