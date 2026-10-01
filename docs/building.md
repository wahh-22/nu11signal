# Building from source

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

Requirements, the one-time MusicKit signing setup, and the make targets. Release builds are covered in [Releasing](releasing.md).

## Requirements

| Need | Why |
|------|-----|
| macOS 14 or later | MusicKit `ApplicationMusicPlayer` on macOS |
| Apple Music subscription | Catalog playback |
| Xcode (Swift 5.9+) | Builds the helper |
| Go (see `go.mod`) | Builds the UI |
| Apple Developer Program membership | Signing the helper with MusicKit entitlements when building from source |

## Quick path

```sh
make demo          # try the UI with a simulated player (no Apple Music, no signing)
make build         # signed helper + Go binary (needs the one-time setup below)
bin/nu11signal     # play for real
```

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
| `make test` | `go test -race ./...`, `swift test` in `helper/`, and the script tests |
| `make test-scripts` | Only the hermetic tests for `scripts/release.sh` and `scripts/bump-cask.sh` |
| `make vet` / `make fmt-check` | `go vet`; fails if `gofmt -l .` lists files |
| `make release VERSION=x.y.z` | Signed, notarized archive and checksum in `dist/vx.y.z/`; `FORCE=1` replaces an existing one (see [Releasing](releasing.md)) |
| `make release-dry-run VERSION=x.y.z` | Same layout in `build/release-dry-run/vx.y.z/`, ad-hoc signed, not notarized; lists missing release setup |
| `make cask VERSION=x.y.z` | Renders the Homebrew cask into a tap checkout and audits it; `PUSH=1` commits and pushes (see [Releasing](releasing.md)) |
| `make clean` | Removes `bin/` and `build/`; release archives in `dist/` are kept |
| `make clean-dist` | Removes `dist/` (release archives) |
