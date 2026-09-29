# Nu11Signal

A lightweight terminal player for Apple Music, styled after a neon cyberpunk
car radio: your library playlists are "stations" on a pseudo FM dial, with a
now-playing panel, decorative EQ bars, and catalog search.

It plays through a tiny windowless MusicKit helper (about 31 MB RSS measured
during playback, near 0% CPU) instead of a browser.

> macOS only. Requires an Apple Music subscription. Not affiliated with Apple.

Nu11Signal was formerly named soul-king; v0.1.0 was released under that name.

## Quick path

```sh
make demo          # try the UI with a simulated player (no Apple Music, no signing)
make build         # signed helper + Go binary (needs the one-time setup below)
bin/nu11signal     # play for real
```

## Install

Signed, notarized builds (macOS 14 or later, Apple Silicon and Intel) are
published on [GitHub Releases](https://github.com/wahh-22/nu11signal/releases).
No Apple Developer account is needed to run them.

With [Homebrew](https://brew.sh):

```sh
brew install --cask wahh-22/tap/nu11signal
```

Or manually from a release archive:

```sh
tar -xzf nu11signal-<version>-macos-universal.tar.gz
nu11signal-<version>/bin/nu11signal          # keep bin/ and libexec/ together
nu11signal-<version>/bin/nu11signal --version
```

Symlink `bin/nu11signal` onto your `PATH` if you like; the helper is found
through the symlink. The cask lives in
[wahh-22/homebrew-tap](https://github.com/wahh-22/homebrew-tap) and is generated
from `packaging/homebrew/nu11signal.rb.template`. The first launch asks for
Apple Music access.

## Architecture

```text
┌──────────────────────┐  JSON lines on stdin/stdout  ┌──────────────────────────┐
│ bin/nu11signal (Go)  │ ───── commands ────────────▶ │ Nu11SignalHelper.app     │
│ Bubble Tea radio UI  │ ◀──── responses, events ──── │ (Swift, MusicKit,        │
│ Player port + adapter│                              │  ApplicationMusicPlayer) │
└──────────────────────┘                              └──────────────────────────┘
```

| Part | Where | Role |
|------|-------|------|
| Radio UI | `internal/radio` | Model/update/view; depends only on the `Player` port |
| Player port | `internal/playback` | Domain types and the `Player` interface |
| Helper adapter | `internal/helper` | Starts the helper, correlates requests, streams state |
| Demo player | `internal/playback/demo` | In-process simulated player for `--demo` |
| Helper | `helper/` | SwiftPM package; `build.sh` bundles and signs the `.app` |

MusicKit exposes no audio samples, so the EQ bars follow the playback state,
not the sound.

## Requirements

| Need | Why |
|------|-----|
| macOS 14 or later | MusicKit `ApplicationMusicPlayer` on macOS |
| Apple Music subscription | Catalog playback |
| Xcode (Swift 5.9+) | Builds the helper |
| Go (see `go.mod`) | Builds the UI |
| Apple Developer Program membership | Signing the helper with MusicKit entitlements when building from source |

## One-time setup (building from source)

MusicKit only works in a signed app with an embedded provisioning profile.

1. **Register an App ID** in the Apple Developer portal (Certificates, IDs &
   Profiles > Identifiers) with your bundle ID, for example
   `com.example.nu11signal.player`, and enable the **MusicKit** App Service.
2. **Register this Mac** under Devices (its Provisioning UDID is in System
   Information > Hardware).
3. **Create a macOS App Development profile** for that App ID, your
   development certificate, and this Mac. Download it.
4. **Save the profile** as `signing/Nu11Signal_Dev.provisionprofile` at the
   repository root. `*.provisionprofile` is gitignored; never commit it.
5. **Export your settings** (defaults in `helper/build.sh`):

| Variable | Meaning | Default |
|----------|---------|---------|
| `NU11SIGNAL_BUNDLE_ID` | Bundle ID of the App ID | `dev.wahh.soulking.player` |
| `NU11SIGNAL_TEAM_ID` | Your Team ID | `W6GZP998GQ` |
| `NU11SIGNAL_PROFILE` | Path to the profile | `signing/Nu11Signal_Dev.provisionprofile` (`signing/Nu11Signal_DeveloperID.provisionprofile` in release mode) |
| `NU11SIGNAL_SIGN_IDENTITY` | `codesign` identity | `Apple Development` |
| `NU11SIGNAL_BUILD_DIR` | Where `Nu11SignalHelper.app` is written | `build` |
| `NU11SIGNAL_SIGN_MODE` | `development`, or `release` (universal, hardened runtime; used by `make release`) | `development` |

```sh
export NU11SIGNAL_BUNDLE_ID=com.example.nu11signal.player
export NU11SIGNAL_TEAM_ID=ABCDE12345
make build
```

The first run asks for Apple Music access.

## Build and run

| Command | Result |
|---------|--------|
| `make build` | `build/Nu11SignalHelper.app` (signed) and `bin/nu11signal` |
| `make helper` | Only the signed helper |
| `make demo` | Builds only the Go binary and runs `bin/nu11signal --demo` |
| `make test` | `go test -race ./...` and `swift test` in `helper/` |
| `make vet` / `make fmt-check` | `go vet`; fails if `gofmt -l .` lists files |
| `make release VERSION=x.y.z` | Signed, notarized `dist/nu11signal-x.y.z-macos-universal.tar.gz` (see Releasing) |
| `make release-dry-run VERSION=x.y.z` | Same layout in `build/release-dry-run/`, ad-hoc signed, not notarized; lists missing release setup |
| `make clean` | Removes `bin/` and `build/`; release archives in `dist/` are kept |
| `make clean-dist` | Removes `dist/` (release archives) |

## Releasing

Release builds are universal (arm64 + x86_64), signed with a Developer ID
certificate, the hardened runtime, and a secure timestamp, embed a Developer ID
provisioning profile (MusicKit needs it), and are notarized.

### One-time setup

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

### Cutting a release

```sh
make release-dry-run VERSION=0.2.0   # optional: build the layout, list missing setup
make release VERSION=0.2.0
```

`scripts/release.sh` refuses to start while any setup item is missing or
tracked files have uncommitted changes. It builds both binaries, signs them,
notarizes the whole layout, staples the helper app, and writes
`dist/nu11signal-0.2.0-macos-universal.tar.gz` plus `.sha256`, then checks the
unpacked archive with `spctl` and `codesign --verify --strict`. Everything is
built in a staging directory and moved into `dist/` only after those checks
pass, so a failed run leaves an earlier `dist/nu11signal-0.2.0*` untouched. If
notarization is rejected it prints the `xcrun notarytool log` command.
Overrides: `NU11SIGNAL_SIGN_IDENTITY`, `NU11SIGNAL_PROFILE`,
`NU11SIGNAL_NOTARY_PROFILE`, `NU11SIGNAL_TEAM_ID`.

Publishing stays manual: tag `v0.2.0`, attach the archive and checksum to a
GitHub release, and fill `version` and `sha256` in the cask template.

## Keys

| Key | Action |
|-----|--------|
| `↑`/`↓` or `k`/`j` | Move the cursor |
| `enter` | Tune the station or play the selected result |
| `space` | Play / pause |
| `n` / `p` | Next / previous track |
| `←` / `→` | Seek -10 s / +10 s |
| `/` | Catalog scan (search); `enter` runs it, `esc` cancels |
| `tab` | Switch between stations and results |
| `esc` | Back to stations |
| `r` | Retry loading stations after a failure |
| `q` / `ctrl+c` | Quit |

## Helper lookup

`nu11signal` never looks in the working directory. It uses the first match:

1. `$NU11SIGNAL_HELPER`: an absolute path to the helper executable (relative paths are rejected).
2. `<binary dir>/Nu11SignalHelper.app/Contents/MacOS/nu11signal-helper`
3. `<binary dir>/../libexec/Nu11SignalHelper.app/...` (packaged installs)
4. `<binary dir>/../build/Nu11SignalHelper.app/...` (this repo: `bin/` + `build/`)

The binary directory is resolved through symlinks. The error lists every path tried.

## Protocol

One JSON object per line.

| Direction | Shape |
|-----------|-------|
| Request | `{"id":"<string>","cmd":"<name>", ...args}` |
| Response | `{"id":"<id>","ok":true,"result":{...}}` or `{"id":"<id>","ok":false,"error":"<msg>"}` |
| Event | `{"event":"ready"}`, `{"event":"state","state":{...}}`, `{"event":"error","message":"<msg>"}` |

| Command | Args | Result |
|---------|------|--------|
| `authorize` | none | `{"status":...}`: `authorized`, `denied`, `restricted`, or `notDetermined` |
| `search` | `term`, `limit` (1-25) | `{"songs":[...]}` |
| `playlists` | none | `{"playlists":[{"id":...,"name":...}]}` |
| `playSongs` | `ids`, `startIndex` | `{}` or `{"missing":[...]}` |
| `playPlaylist` | `playlistId` | `{}` |
| `pause`, `resume`, `next`, `previous`, `stop` | none | `{}` |
| `seek` | `seconds` (>= 0) | `{}`, followed by a `state` event |

Playback commands run one at a time in arrival order, each bounded by 10 s
(a hung one is answered with a timeout error); `authorize`, `search`, and
`playlists` run concurrently. At stdin EOF the helper finishes in-flight work
(up to 3 s), stops playback, and exits.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `developerTokenRequestFailed` | Missing entitlements or provisioning profile | Rebuild with a valid profile for your bundle ID and team (`make helper`) |
| Helper exits with status 137 | AMFI killed it: entitlements without an embedded profile | Check `NU11SIGNAL_PROFILE`; run `codesign -d --entitlements - build/Nu11SignalHelper.app` |
| Authorization denied | Access was refused once | System Settings > Privacy & Security > Media & Apple Music, enable the helper |
| `nu11signal-helper not found` | Helper not built or not next to the binary | `make build`, or set `NU11SIGNAL_HELPER` to an absolute path |

## Repository notes

- `spike/` is the historical proof of concept (authorize, search, play from a
  signed windowless app). It is kept for reference and not used by the build.
- License: MIT — see [LICENSE](LICENSE).
