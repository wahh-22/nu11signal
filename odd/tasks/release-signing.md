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

- [x] U1 Developer ID Application certificate in the login keychain (verified: `Developer ID Application: Wilmer Henao (W6GZP998GQ)`, SHA-1 F9597ECD…).
- [x] U2 Developer ID provisioning profile (verified: team W6GZP998GQ, app id `W6GZP998GQ.dev.wahh.soulking.player`, cert matches U1, all devices, expires 2044-09-23) for `dev.wahh.soulking.player` → `signing/SoulKing_Player_DeveloperID.provisionprofile`.
- [x] U3 `xcrun notarytool store-credentials soulking-notary` (verified: `notarytool history` authenticates).

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

- R1 native review (range `1073c9e..a0c6081`, includes timeouts, license, module path): tier high; consent granted; 4-lens review approved, receipt acknowledged (lineage `review-d07d29418cc29290`). Fixed the flaky non-finite Deadline test it flagged (NaN/negative now assert an immediate timeout; 3 stable runs).
- Follow-ups (minor advisories): `release.sh` deletes the previous archive before the new build succeeds (line 125); `main.go` exit-path split untested (46-51); release.sh literals/help text nits.

- R2 build (2026-09-28, from `91c4522`): `make release VERSION=0.1.0` exit 0; notarization submission `02332dda-4fd0-4815-b88d-a6cc0bfd9f77` accepted; helper stapled (`stapler validate` ok); archive `soul-king-0.1.0-macos-universal.tar.gz` sha256 `0ed8c3cc3d91e95dd8390c3332941eb97b5ef90cb53d996b7fc608499db83ff9`.
- R2 download simulation (quarantined copy): sha256 OK; helper `spctl` accepted (Notarized Developer ID); CLI runs under quarantine (`--version` → 0.1.0; bare binaries are not `spctl`-assessable); `lipo` x86_64 arm64; notarized helper authorizes and searches the catalog (Developer ID profile works with MusicKit). Pending: user live TUI + audio check from the extracted archive.

## Next step

R2 once the user finishes U1–U3: `make release VERSION=0.1.0`.
