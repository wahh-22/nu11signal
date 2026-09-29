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

- [x] F1 — Remove legacy fallbacks: old profile names and `spike/` in `helper/build.sh` and `scripts/release.sh`, plus the README legacy mentions (the "formerly soul-king" note stays). Resolves the duplicated-fallback advisory. Route: delegated (writer trigger). Commit `d85fe3a`.
- [x] F2 — `release.sh` builds, notarizes, archives, and verifies in `dist/.staging-nu11signal-<ver>.XXXXXX` (same filesystem) and promotes `nu11signal-<ver>/`, the archive, and `.sha256` into `dist/` only after the final checks; the staging dir is removed on any exit. Dry runs write to `build/release-dry-run/` and never touch `dist/`. `make clean` removes `bin/` and `build/`; new `make clean-dist` removes `dist/`. Route: delegated (writer trigger). Commit `71e54ef`.
- [x] F3 — `run(args, deps) int` injects stdout/stderr, helper lookup/start, and the UI; `cmd/nu11signal/run_test.go` covers `--version` (0), `-h` (0), unknown flag (2), helper not found (1, message), helper start failure (1, `start helper:` prefix), helper player handed to the UI and closed once, `--demo` uses `*demo.Player` without the helper, UI interrupt (0) and UI failure (1). Route: delegated (writer trigger). Commit `a43fa98`.
- [x] F4 — `scripts/bump-cask.sh VERSION [--push]` / `make cask VERSION=x.y.z [PUSH=1]`: semver check, sha256 from `dist/…sha256` (fails clearly when missing or mismatched), tap checkout at `NU11SIGNAL_TAP_DIR` (default `../homebrew-tap`; clone when missing, `pull --ff-only` when present, refuse other uncommitted changes), render without the template header, `ruby -c`, `brew audit --cask --strict` through a temporary tap symlink (removed on exit), `git diff`; commit `chore: bump nu11signal to VERSION` and push only with `--push`. README "Releasing" documents it. Route: delegated (writer trigger). Commit `86c5aed`.

Route: delegated (one writer; writer trigger: 2+ non-trivial files).

### Follow-up evidence

- `make test`: Go 146 tests passed (6 packages, -race; +10 from F3); Swift 33 passed. `make vet` clean; `make fmt-check` exit 0.
- F1: `make build` signs `build/Nu11SignalHelper.app` with Apple Development, `codesign --verify --strict` valid, embedded profile byte-identical to `signing/Nu11Signal_Dev.provisionprofile`; `bin/nu11signal --version` → `dev`. `NU11SIGNAL_PROFILE=/nonexistent ./helper/build.sh` → `error: provisioning profile not found: /nonexistent` naming `signing/Nu11Signal_Dev.provisionprofile`, exit 1 (release mode names `signing/Nu11Signal_DeveloperID.provisionprofile`). No `SoulKing_` profile names left outside `odd/` and `spike/`.
- F2: `make release-dry-run VERSION=0.2.1` → layout in `build/release-dry-run/nu11signal-0.2.1/{bin,libexec,LICENSE,README.md}`, both executables `x86_64 arm64`, no staging left; `dist/` content hash unchanged (`0baa87b3…`). Forced failure of a real run for the published version (`GOFLAGS=-mod=bogus scripts/release.sh 0.2.0`, fails after staging, before notarization) → exit 1, `dist/` hash unchanged, no staging left. `make clean` removed `bin/` and `build/`, `dist/` intact (hash unchanged); `make build` restored the dev environment.
- F3: RED observed twice — compile failure (`undefined: deps`, `too many arguments in call to run`), then with a stub `run` returning -1 all 8 new tests failed on exit-code assertions; GREEN after the refactor. The pre-existing `parseFlags`/`printVersion` tests are unchanged characterization tests. Real binary: `--version` exit 0, `--nope` exit 2, `-h` exit 0, a lone copy without a helper → `nu11signal: nu11signal-helper not found: …` exit 1.
- F4: `NU11SIGNAL_TAP_DIR=<temp> make cask VERSION=0.2.0` cloned the tap, rendered, `ruby -c` OK, audit passed, empty `git diff`; file identical to the published `Casks/nu11signal.rb`. Rerun on the existing checkout fast-forwarded and passed. With a temporary fake `dist/…0.2.1….sha256` the diff showed only `version`/`sha256` and "Not pushed" (fake removed). `make cask VERSION=0.2` → semver error, exit 2; missing checksum → clear error; stray file in the tap → refused. The audit reads the linked checkout (a render without `homepage` fails it). Nothing was pushed.
- Note: an early F4 draft set `HOMEBREW_NO_INSTALL_FROM_API=1`, which made Homebrew tap `homebrew/core` (1.4 GB); the option was removed and `homebrew/core` untapped again, restoring the previous tap list.

- Template comment updated to describe `make cask` (inline, 1 file).
- F1–F4 native review (range `e9661a7..HEAD`): tier high; consent granted; 4-lens review approved, receipt acknowledged (lineage `review-6bc032f28c0027b1`).

## Backlog (non-blocking advisories)

- `bump-cask.sh --push`: a commit left unpushed after a failed push is not detected on retry (131-149); the dirty-check exclusion scope (86-88); no script tests.
- `release.sh` promotion into `dist/` is two renames, not a single atomic step (145-150, 232-233).

## Backlog work (branch `chore/release-backlog`, user-requested 2026-09-29)

- [x] B1 — `bump-cask.sh --push`: detect and push a bump commit left unpushed after an earlier failed push (tap ahead of origin); narrow the dirty-check exclusion to exactly `Casks/nu11signal.rb`. Route: delegated (writer trigger). Commit `c781f18`.
- [x] B2 — `release.sh`: promote into `dist/` as a single atomic step (one directory rename per version instead of several separate moves), so an interruption never leaves a half-promoted release. Route: delegated (writer trigger). Commit `1fb0c6c`.
- [x] B3 — Script tests: a hermetic test harness for `bump-cask.sh` and `release.sh` (stubbed `brew`/`git`/`xcrun`/`codesign` on PATH, temp dirs), runnable from `make test`. Route: delegated (writer trigger). Harness and release tests in `1fb0c6c`, bump-cask tests in `c781f18`.
- [x] B4 — Rename the local working directory `~/wahh22/soul-king` → `~/wahh22/nu11signal` (last step; the session must be restarted in the new path).

Route: delegated (one writer, B1–B3); B4 inline, last.

### Backlog evidence

- Design: B2 layout `dist/v<ver>/{nu11signal-<ver>/, nu11signal-<ver>-macos-universal.tar.gz, .sha256}`, built in `dist/.staging-v<ver>.XXXXXX` and promoted with one `mv` (staging chmod 755 first; `mktemp -d` makes it 0700). An existing `dist/v<ver>` is refused before building unless `--force`/`FORCE=1`; then it is renamed to `dist/v<ver>.replaced-<timestamp>` (kept) and the EXIT trap renames it back if the second rename does not happen (only SIGKILL/power loss between the two renames can leave it missing). Dry runs use `build/release-dry-run/v<ver>/` and replace an earlier dry run. B1: dirty check parses `git status --porcelain=v1 --untracked-files=all` and allows only `[ MA][ MA]`/`??` entries for exactly `Casks/nu11signal.rb`, listing everything else; commits ahead of `@{upstream}` must all be `chore: bump nu11signal to VERSION` and touch only the cask; the render is compared against `HEAD`, so an unchanged render never commits; a matching unpushed bump is reported (no `--push`) or pushed (`--push`); a mismatching one is refused with `--push`. B3: `scripts/test/run.sh` (plain bash runner; `bats` and `shellcheck` are not installed), `lib.sh` sandbox that copies the scripts into a temp ROOT (no env seams needed), PATH limited to stubs + system dirs; stubs for `brew`, `ruby`, `go`, `swift`, `lipo`, `codesign`, `security`, `xcrun`, `spctl`, a logging `mv`, and a release-only `git`; bump tests use real git against a local bare origin.
- RED (tests written first, against the scripts at `9da1953`): 18 of 22 failed. Release: single rename, other versions untouched, refuse without `--force`, `--force` (exit 2, unknown option), dry-run layout failed; the two failure-leaves-dist-untouched tests already passed. Bump: every test past validation failed on the checksum path; after only the `dist/v<ver>` path change, 7 still failed — the three dirty-check tests (offending path not listed), unpushed bump not reported, `--push` did not push the earlier bump ("nothing to commit"), mismatched bump not refused, and an unrelated local commit was pushed along with a new bump commit (exit 0).
- GREEN: `make test-scripts` → 23 passed, 0 failed (adds a `--force` promotion-failure restore test). `make test`: Go 146 passed (6 packages, -race), Swift 33 passed, scripts 23 passed; exit 0. `make vet` clean; `make fmt-check` exit 0. `bash -n` OK on every script and stub; shellcheck not installed.
- `make release-dry-run VERSION=0.2.1` → exit 0, `build/release-dry-run/v0.2.1/nu11signal-0.2.1/{bin,libexec,LICENSE,README.md}`, `lipo -archs` `x86_64 arm64`, `--version` 0.2.1, no staging left. `dist/` content hash (`find dist -type f -exec shasum {} + | sort | shasum`) `de9cc992…` identical before and after the session. `brew tap` output identical before and after; no `nu11signal-bump-*` tap left. Nothing pushed.
- Pending for the parent: migrate the local `dist/` to the new layout (`dist/v0.2.0/…`, and `dist/v0.1.0/` for the `soul-king-0.1.0*` files if wanted); until then `make cask VERSION=0.2.0` reports the checksum missing at `dist/v0.2.0/…`.

- Local `dist/` migrated to `dist/v0.1.0/` and `dist/v0.2.0/` (content hash unchanged); `make cask VERSION=0.2.0` reads the new path.
- B1–B3 native review (range `c2b6003..HEAD`): tier high; consent granted; 4-lens review approved, receipt acknowledged (lineage `review-d70bd4536c0dc4d0`). Remaining minor advisories (not scheduled): `bump-cask.sh` net-diff guard wording and its tests for the unpushed-bump guards (114-129); upstream check ordering after pull (102-103); the `--force` backup window (release.sh:179-180).

- PR #6 merged (main `a481ba2`). B4: local directory renamed to `~/wahh22/nu11signal`; git/remote/profiles/dist intact; `go build` + `make test` (Swift 33, scripts 23, Go) pass from the new path.

- v0.2.1 released from `9c04b5c` (PR #8 ignores `.atl/`); cask bumped with `make cask VERSION=0.2.1 PUSH=1`; `brew upgrade` verified. Repo cleaned to `main` only.

## Advisory cleanup (branch `chore/release-advisories`, user-requested 2026-09-29)

- [x] A1 — `bump-cask.sh` unpushed-bump guard: clear wording for each net-diff check (non-bump commits, files other than the cask, empty net diff, mismatch with the render), each naming the drop command; tests for every guard branch. Commit `31a72ad`.
- [x] A2 — `bump-cask.sh`: fetch and compare with the upstream before touching the checkout (behind: fast-forward; ahead: checked by the guard; diverged: refused with the local commits and how to drop or rebase them; up to date: reported; no upstream: refused before any pull). Commit `8ef5125`.
- [x] A3 — `release.sh --force`: backup name prepared and `BACKUP` set before the first rename, the two renames adjacent (fixes a gap: an interruption right after the first rename did not restore); a lone `dist/vX.replaced-*` (killed run) refuses the release with the restore command; backups next to an existing `dist/vX` are listed as a warning. Commit `fc3e9cc`.

Route: delegated (one writer; writer trigger: script + tests).

Evidence:
- RED before the fixes: A1 3 of 4 new tests failed (missing wording; the empty net diff printed "touch more than" with an empty list); A2 4 of 4 failed (no status lines; diverged and no-upstream both failed inside the pull as "could not fast-forward ... (diverged?)"); A3 3 of 3 failed (interruption after the first rename left `dist/v0.2.1` missing; no leftover-backup report; a lone backup was built over).
- GREEN: `make test-scripts` 34 passed, 0 failed; `make test` exit 0 (Go ok, Swift 33 tests 0 failures, scripts 34 passed); `make vet` and `make fmt-check` exit 0; `bash -n` on every script and stub ok; shellcheck not installed.
- `dist/` content hash identical before and after (`64c2f12e…ccd9e`); `brew tap` output identical.

## Next step

Review, PR, merge.
