# Releasing

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

Cutting a signed, notarized release and updating the Homebrew cask. The developer setup for local builds is in [Building from source](building.md).

Release builds are universal (arm64 + x86_64), signed with a Developer ID
certificate, the hardened runtime, and a secure timestamp, embed a Developer ID
provisioning profile (MusicKit needs it), and are notarized.

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
the unpacked archive with `spctl` and `codesign --verify --strict`. Each
version gets its own directory:

```text
dist/v0.2.0/
  nu11signal-0.2.0/                              bin/, libexec/, LICENSE, README.md
  nu11signal-0.2.0-macos-universal.tar.gz
  nu11signal-0.2.0-macos-universal.tar.gz.sha256
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
`build/release-dry-run/v0.2.0/` instead. Overrides: `NU11SIGNAL_SIGN_IDENTITY`,
`NU11SIGNAL_PROFILE`, `NU11SIGNAL_NOTARY_PROFILE`, `NU11SIGNAL_TEAM_ID`.

Publishing: tag `v0.2.0` and attach the archive and checksum to a GitHub
release. Then update the Homebrew cask:

```sh
make cask VERSION=0.2.0          # render, ruby -c, brew audit --cask --strict, show the diff
make cask VERSION=0.2.0 PUSH=1   # same, then commit "chore: bump nu11signal to 0.2.0" and push
```

`scripts/bump-cask.sh` fills `packaging/homebrew/nu11signal.rb.template` with
the version and the sha256 from `dist/v0.2.0/nu11signal-0.2.0-macos-universal.tar.gz.sha256`
and writes `Casks/nu11signal.rb` in a checkout of
[wahh-22/homebrew-tap](https://github.com/wahh-22/homebrew-tap):
`NU11SIGNAL_TAP_DIR` (default `../homebrew-tap`), cloned when missing and
fast-forwarded when behind its upstream. It refuses a checkout that has
diverged from its upstream (it prints the local commits and how to drop or
rebase them), and one with uncommitted changes or untracked files anywhere
except `Casks/nu11signal.rb`. Without `PUSH=1` nothing
is committed, and an unchanged render never creates a commit. If an earlier
`PUSH=1` committed but the push failed, a rerun reports the unpushed
`chore: bump nu11signal to 0.2.0` commit, and `PUSH=1` pushes it (after
checking it matches the render) instead of committing again; any other local
commit is refused.

`make test-scripts` (part of `make test`) runs hermetic tests for both scripts
(`scripts/test/`): stubbed `brew`, `xcrun`, `codesign`, `go`, and friends, temp
directories for `dist/` and the tap, and a local bare repository as its origin.
