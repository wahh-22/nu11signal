# soul-king

A lightweight terminal player for Apple Music, styled after a neon cyberpunk
car radio: your library playlists are "stations" on a pseudo FM dial, with a
now-playing panel, decorative EQ bars, and catalog search.

It plays through a tiny windowless MusicKit helper (about 31 MB RSS measured
during playback, near 0% CPU) instead of a browser.

> macOS only. Requires an Apple Music subscription. Not affiliated with Apple.

## Quick path

```sh
make demo          # try the UI with a simulated player (no Apple Music, no signing)
make build         # signed helper + Go binary (needs the one-time setup below)
bin/soul-king      # play for real
```

## Architecture

```text
┌──────────────────────┐  JSON lines on stdin/stdout  ┌──────────────────────────┐
│ bin/soul-king (Go)   │ ───── commands ────────────▶ │ SoulKingHelper.app       │
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
   `com.example.soulking.player`, and enable the **MusicKit** App Service.
2. **Register this Mac** under Devices (its Provisioning UDID is in System
   Information > Hardware).
3. **Create a macOS App Development profile** for that App ID, your
   development certificate, and this Mac. Download it.
4. **Save the profile** as `signing/SoulKing_Player.provisionprofile` at the
   repository root. `*.provisionprofile` is gitignored; never commit it.
5. **Export your settings** (defaults in `helper/build.sh`):

| Variable | Meaning | Default |
|----------|---------|---------|
| `SOULKING_BUNDLE_ID` | Bundle ID of the App ID | `dev.wahh.soulking.player` |
| `SOULKING_TEAM_ID` | Your Team ID | `W6GZP998GQ` |
| `SOULKING_PROFILE` | Path to the profile | `signing/SoulKing_Player.provisionprofile`, else `spike/SoulKing_Player.provisionprofile` |
| `SOULKING_SIGN_IDENTITY` | `codesign` identity | `Apple Development` |

```sh
export SOULKING_BUNDLE_ID=com.example.soulking.player
export SOULKING_TEAM_ID=ABCDE12345
make build
```

The first run asks for Apple Music access.

## Build and run

| Command | Result |
|---------|--------|
| `make build` | `build/SoulKingHelper.app` (signed) and `bin/soul-king` |
| `make helper` | Only the signed helper |
| `make demo` | Builds only the Go binary and runs `bin/soul-king --demo` |
| `make test` | `go test -race ./...` and `swift test` in `helper/` |
| `make vet` / `make fmt-check` | `go vet`; fails if `gofmt -l .` lists files |
| `make clean` | Removes `bin/` and `build/` |

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

`soul-king` never looks in the working directory. It uses the first match:

1. `$SOULKING_HELPER`: an absolute path to the helper executable (relative paths are rejected).
2. `<binary dir>/SoulKingHelper.app/Contents/MacOS/soulking-helper`
3. `<binary dir>/../libexec/SoulKingHelper.app/...` (packaged installs)
4. `<binary dir>/../build/SoulKingHelper.app/...` (this repo: `bin/` + `build/`)

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
| Helper exits with status 137 | AMFI killed it: entitlements without an embedded profile | Check `SOULKING_PROFILE`; run `codesign -d --entitlements - build/SoulKingHelper.app` |
| Authorization denied | Access was refused once | System Settings > Privacy & Security > Media & Apple Music, enable the helper |
| `soulking-helper not found` | Helper not built or not next to the binary | `make build`, or set `SOULKING_HELPER` to an absolute path |

## Repository notes

- `spike/` is the historical proof of concept (authorize, search, play from a
  signed windowless app). It is kept for reference and not used by the build,
  except as a fallback profile location.
- License: MIT — see [LICENSE](LICENSE).
