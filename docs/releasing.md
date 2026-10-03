# Releasing

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

Cutting a signed, notarized release with its Linux archives, and updating the Homebrew cask and formula. The developer setup for local builds is in [Building from source](building.md).

macOS release builds are universal (arm64 + x86_64), signed with a Developer ID
certificate, the hardened runtime, and a secure timestamp, embed a Developer ID
provisioning profile (MusicKit needs it), and are notarized. Linux builds
(amd64 and arm64) are cross-compiled in the same run with `CGO_ENABLED=0` and
need none of the Apple setup below.

## One-time setup

1. **Developer ID Application certificate** (Account Holder role required).
   Xcode > Settings > Accounts > your team > Manage Certificates > `+` >
   Developer ID Application. Or in the portal (Certificates > `+` > Developer
   ID Application) upload a CSR from Keychain Access > Certificate Assistant >
   Request a Certificate From a Certificate Authority, then open the
   downloaded certificate. Check:
   `security find-identity -v -p codesigning | grep "Developer ID Application"`.
2. **Developer ID provisioning profile.** Portal > Profiles > `+` >
   Distribution > Developer ID, choose the App ID `dev.wahh.soulking.player`
   (MusicKit enabled) and the Developer ID certificate, generate, download,
   and save it as `signing/Nu11Signal_DeveloperID.provisionprofile`
   (gitignored).
3. **Notary credentials.** Create an app-specific password at
   [account.apple.com](https://account.apple.com) (Sign-In and Security >
   App-Specific Passwords), then store it in the keychain:

   ```sh
   xcrun notarytool store-credentials soulking-notary \
     --apple-id you@example.com --team-id W6GZP998GQ
   xcrun notarytool history --keychain-profile soulking-notary   # check
   ```

## Cutting a release

```sh
make release-dry-run VERSION=0.2.0   # optional: build the layout, list missing setup
make release VERSION=0.2.0
```

`scripts/release.sh` refuses to start while any setup item is missing or
tracked files have uncommitted changes. It builds both binaries, signs them,
notarizes the whole layout, staples the helper app, archives it, then checks
the unpacked archive with `spctl` and `codesign --verify --strict`. It also
cross-compiles `nu11signal` for linux/amd64 and linux/arm64 (same
`-X main.version` ldflags) and archives each with `LICENSE` and `README.md`;
each unpacked Linux archive is checked to hold exactly those files, to match
its checksum, and to contain an ELF executable for its architecture stamped
with the version. It cannot be run on the Mac, so `--version` is skipped:
every build also stamps `-X main.versionStamp=nu11signal-version:VERSION;`,
and the check looks for exactly those bytes in the binary. The fixed prefix
and the `;` terminator mean no other version (`v0.2.0`, `10.2.0`, `0.2.01`)
can pass for it.
Each version gets its own directory:

```text
dist/v0.2.0/
  nu11signal-0.2.0/                              bin/, libexec/, LICENSE, README.md
  nu11signal-0.2.0-macos-universal.tar.gz
  nu11signal-0.2.0-macos-universal.tar.gz.sha256
  nu11signal-0.2.0-linux-amd64.tar.gz            nu11signal-0.2.0/{bin/nu11signal, LICENSE, README.md}
  nu11signal-0.2.0-linux-amd64.tar.gz.sha256
  nu11signal-0.2.0-linux-arm64.tar.gz
  nu11signal-0.2.0-linux-arm64.tar.gz.sha256
```

Everything is built in a staging directory inside `dist/` and promoted with a
single rename to `dist/v0.2.0` only after those checks pass; a failed or
interrupted run removes the staging directory and leaves `dist/` untouched.
An existing `dist/v0.2.0` is never overwritten: `make release VERSION=0.2.0
FORCE=1` (`--force`) renames it to `dist/v0.2.0.replaced-<timestamp>` first
and restores it if the promotion fails or is interrupted. Earlier backups are
listed as a warning (delete them when no longer needed); if only a backup is
left (a run killed between the two renames), the release is refused until it
is restored or deleted. If notarization is rejected it prints
the `xcrun notarytool log` command. `make release-dry-run` writes to
`build/release-dry-run/v0.2.0/` instead (the macOS layout ad hoc and
unarchived, the Linux archives built and checked as in a release). Overrides:
`NU11SIGNAL_SIGN_IDENTITY`, `NU11SIGNAL_PROFILE`, `NU11SIGNAL_NOTARY_PROFILE`,
`NU11SIGNAL_TEAM_ID`.

### Linux archives only

```sh
make release-linux VERSION=0.2.0             # dist/v0.2.0/ with only the Linux archives
make release-linux VERSION=0.2.0 DRY_RUN=1   # same in build/release-dry-run/v0.2.0/
```

`make release-linux` (`scripts/release.sh --linux-only`) builds and checks
only the two Linux archives, with no Apple credentials, signing, or Xcode
tools; only the clean-tree check applies. It is promoted the same way (staging
directory, one rename) and also refuses an existing `dist/v0.2.0`. With
`FORCE=1` it replaces an existing `dist/v0.2.0` with a directory that holds
only the Linux archives of this build. Artifacts of different builds are
never mixed, so the macOS artifacts of the earlier build are not carried over:
the run lists them, and they stay in the previous directory, kept as the
`dist/v0.2.0.replaced-<timestamp>` backup as usual. To publish macOS and
Linux archives together, rebuild both with `make release VERSION=0.2.0
FORCE=1`; `make cask` refuses a `dist/v0.2.0` without the macOS checksum.

The Linux binaries cannot be run on the build machine, so each is checked
instead: a 64-bit little-endian ELF for its architecture whose type is an
executable (`ET_EXEC`, or `ET_DYN` for a position-independent build), stamped
with exactly the release version, matched literally and not as part of
another version: `0.5.0` matches none of `0.5.01`, `10.5.0`, `0.5.0.1` or
`0.5.0-rc1`, but letters the linker happens to place next to it do not hide
it (`go0.5.0abc` still matches). `-trimpath` keeps the ldflags out of the
build info, so the stamped string is the only evidence in the binary.

## Publishing and Homebrew

Tag `v0.2.0` and attach all three archives and their checksums to a GitHub
release. Then update the Homebrew cask and formula in one run:

```sh
make cask VERSION=0.2.0          # render both, ruby -c, brew audit and brew style, show the diff
make cask VERSION=0.2.0 PUSH=1   # same, then commit "chore: bump nu11signal to 0.2.0" and push
```

`scripts/bump-cask.sh` fills `packaging/homebrew/nu11signal.rb.template` with
the version and the sha256 from `dist/v0.2.0/nu11signal-0.2.0-macos-universal.tar.gz.sha256`
into `Casks/nu11signal.rb`, and `packaging/homebrew/nu11signal-formula.rb.template`
with the same macOS sha256 and those of both Linux archives into
`Formula/nu11signal.rb`, in a checkout of
[wahh-22/homebrew-tap](https://github.com/wahh-22/homebrew-tap):
`NU11SIGNAL_TAP_DIR` (default `../homebrew-tap`), cloned when missing and
fast-forwarded when behind its upstream. All three checksums must exist;
a missing one is named, with the `make release` command that builds it.

The formula installs on both systems. It shares its name with the cask, so on
macOS `brew install wahh-22/tap/nu11signal` without `--cask` resolves to it:
its top-level url and sha256 are the macOS universal archive (installed as
`bin/` and `libexec/` in the keg, so the helper is found through the
symlink), and `on_linux` replaces them per architecture (`on_intel`: amd64,
`on_arm`: arm64; `brew style` rejects a url directly inside `on_macos`). The
cask is checked with `brew audit --cask --strict`, the formula with `brew
audit --formula --strict` and `brew style` (the audit does not run every
style cop). It refuses a checkout
that has diverged from its upstream (it prints the local commits and how to
drop or rebase them), and one with uncommitted changes or untracked files
anywhere except `Casks/nu11signal.rb` and `Formula/nu11signal.rb`. Without
`PUSH=1` nothing is committed, and an unchanged render never creates a commit;
one bump commit holds both files. If an earlier `PUSH=1` committed but the
push failed, a rerun reports the unpushed `chore: bump nu11signal to 0.2.0`
commit, and `PUSH=1` pushes it (after checking it matches the render) instead
of committing again; any other local commit, or one touching other files, is
refused.

## Script tests

`make test-scripts` (part of `make test`) runs hermetic tests for both scripts
(`scripts/test/`): stubbed `brew`, `xcrun`, `codesign`, `go` (which writes a
fake ELF header per `GOARCH` for Linux builds), and friends, temp directories
for `dist/` and the tap, and a local bare repository as its origin.
