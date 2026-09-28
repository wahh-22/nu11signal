# Feature: release-signing

Locator: `odd/tasks/release-signing.md` · Engram mirror: `odd/release-signing/tasks` · Branch: `feat/release-signing`

## Objective

Distribute signed, notarized soul-king binaries that anyone with an Apple Music subscription can install (no Apple Developer account needed), via GitHub Releases and a Homebrew cask.

## Why

Development signing only works on registered Macs. Gatekeeper requires Developer ID signing + notarization, and MusicKit needs a Developer ID provisioning profile embedded in the helper bundle.

## Scope

- Release build: universal (arm64 + x86_64) helper and Go binary, hardened runtime, secure timestamp, Developer ID identity, Developer ID provisioning profile.
- Package layout `soul-king-<version>/{bin/soul-king, libexec/SoulKingHelper.app, LICENSE, README.md}` (matches `Locate()`'s `../libexec` path).
- Notarization with `notarytool` (keychain profile `soulking-notary`), stapling the helper bundle, archive + SHA-256.
- `--version` flag.
- Homebrew cask template; GitHub release publishing stays a user decision.

## Constraints

- Team `W6GZP998GQ`, bundle ID `dev.wahh.soulking.player`.
- Secrets never committed: certificates live in the login keychain, notary credentials in the keychain profile, profiles under gitignored `signing/`.
- Development builds (`make build`) keep working unchanged.

## User steps (portal / keychain; cannot be automated)

- [ ] U1 Developer ID Application certificate in the login keychain.
- [ ] U2 Developer ID provisioning profile for `dev.wahh.soulking.player` → `signing/SoulKing_Player_DeveloperID.provisionprofile`.
- [ ] U3 `xcrun notarytool store-credentials soulking-notary` (app-specific password).

## Tasks

- [x] R1 — Release tooling: `helper/build.sh` release mode, `scripts/release.sh`, Makefile `release` target, `--version`, cask template, README release section. Route: delegated (writer trigger: 2+ non-trivial files). Commits `81a9672` (--version, RED→GREEN), `5b42148` (cask template + Homebrew-symlink Locate test, characterization: passed on first run), `6b6a8f7` (release mode, release script, Makefile, README).
- [ ] R2 — First signed + notarized build `v0.1.0`, verify with `spctl`/`stapler`/live run on a clean path. Blocked on U1–U3.
- [ ] R3 — Publish: tag, GitHub release with artifacts, cask (user decides tap location).

## Acceptance criteria

- `make release VERSION=x.y.z` produces a notarized, stapled archive; `spctl --assess` accepts the helper app and the binary passes `codesign --verify --strict`.
- The unpacked archive runs from any directory and plays music on this Mac.

## Checks

- `make test`, `make vet`, `make fmt-check`; `shellcheck` if available; release script dry run.

## Progress

- PR #1 merged (`9906b53`). Branch `feat/release-signing` created from `main`.

- R1 evidence (2026-09-28): `make build` dev helper signs and verifies (`Apple Development`, same entitlements, embedded profile); `make release-dry-run VERSION=0.1.0` lists 3 missing Developer ID items (+ dirty tree before commit), `lipo -archs` = `x86_64 arm64` for `bin/soul-king` and the helper, layout `dist/soul-king-0.1.0/{bin,libexec/SoulKingHelper.app,LICENSE,README.md}`; `dist/.../bin/soul-king --version` from `/tmp` prints `0.1.0`; `make test`/`vet`/`fmt-check` pass; `make release VERSION=0.1.0` stops in preflight (identity, profile, notary profile) before building. `shellcheck` not installed (skipped; `bash -n` passes). Helper slices report `minos 14.0` despite an x86_64 deprecation warning from SwiftPM.

## Next step

R2 once the user finishes U1–U3: `make release VERSION=0.1.0`.
