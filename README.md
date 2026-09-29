# Nu11Signal

A lightweight terminal player for Apple Music, styled after a neon cyberpunk
car radio: your library playlists are "stations" on a pseudo FM dial, with a
now-playing panel, decorative EQ bars, and Apple Music style catalog browsing
(search, artist pages, albums, songs, and playlists).

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
| `make test` | `go test -race ./...`, `swift test` in `helper/`, and the script tests |
| `make test-scripts` | Only the hermetic tests for `scripts/release.sh` and `scripts/bump-cask.sh` |
| `make vet` / `make fmt-check` | `go vet`; fails if `gofmt -l .` lists files |
| `make release VERSION=x.y.z` | Signed, notarized archive and checksum in `dist/vx.y.z/`; `FORCE=1` replaces an existing one (see Releasing) |
| `make release-dry-run VERSION=x.y.z` | Same layout in `build/release-dry-run/vx.y.z/`, ad-hoc signed, not notarized; lists missing release setup |
| `make cask VERSION=x.y.z` | Renders the Homebrew cask into a tap checkout and audits it; `PUSH=1` commits and pushes (see Releasing) |
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

## Browsing the catalog

Views stack like Apple Music's: stations → SEARCH → RESULTS → ARTIST → ALBUM,
SONG, or PLAYLIST. `esc` goes back one view, `tab` returns to the stations. Leaving
with `tab` keeps the search branch as it was: `/` or `tab` from the stations
brings back the same view (an artist or album page included) with its cursor.

| View | Shows |
|------|-------|
| SEARCH | RECENT searches while the input is empty; once you type 2+ characters, live suggestions, then matching artists (with their genre) and songs |
| RESULTS | The full search for a submitted term, non-empty sections only: TOP RESULTS (tagged ARTIST, ALBUM, SONG, or PLAYLIST), ARTISTS, ALBUMS (with artist and year), SONGS (with artist), PLAYLISTS (with curator) |
| ARTIST | Its non-empty sections in Apple Music order: TOP SONGS, ESSENTIAL ALBUMS, ALBUMS, ARTIST PLAYLISTS, SINGLES & EPS, COMPILATIONS, then ABOUT (editorial notes folded behind MORE, FROM, FORMED, GENRE) |
| ALBUM | The tracks (by disc when there are several), release date, song count and length, copyright, record label, and notes |
| SONG | The album holding the song, with the cursor on the song; if the album cannot be loaded, the song alone, still playable |
| PLAYLIST | The tracks with their artists, song count and length, curator, and notes |

Recent searches are the last 10 terms you submitted with `enter` (typed, a
suggestion, or a recent term) or opened an artist or song from, stored in
`nu11signal/recent.json` under `os.UserConfigDir()`
(`~/Library/Application Support/nu11signal/recent.json` on macOS). Demo mode
keeps them in memory only. Delete one with `ctrl+d` / `delete` or its `✕`.

On ALBUM, SONG, and PLAYLIST pages, `▶` marks the track the player is on
(playing or paused), and moves with it; no track is marked when the player
is on a song from elsewhere.

Limitations:

- No artwork: artists, albums, and playlists are text rows.
- FROM and FORMED rely on an undocumented Apple Music API field; they are
  left out when it is absent.
- Relationships (an artist's albums, singles, playlists, and so on) show the
  first page the catalog returns, not the full list.

## Keys

Stations (and anywhere the key is not taken by the view):

| Key | Action |
|-----|--------|
| `↑`/`↓` or `k`/`j` | Move the cursor; `↑` on the first station moves the focus to the nav tabs (see below) |
| `enter` | Tune the station |
| `space` | Play / pause |
| `n` / `p` | Next / previous track |
| `shift+←` / `shift+→` or `,` / `.` | Seek -10 s / +10 s |
| `→` | Move the focus to the player (see below) |
| `f` / `ctrl+f` | Expand the player to the full width, or restore it |
| `/` | Back to the search left with `tab` (same view and cursor); otherwise open SEARCH with an empty input |
| `tab` | Same as `/` |
| `esc` | Back one view |
| `r` | Retry loading stations after a failure |
| `q` / `ctrl+c` | Quit |

SEARCH (typing goes to the input, so letter shortcuts are off):

| Key | Action |
|-----|--------|
| `↑`/`↓` | Move between the input and the rows; `↑` on the input moves the focus to the nav tabs |
| `←`/`→` | On the input, move the text cursor; on a row, `→` moves the focus to the player |
| `shift+←` / `shift+→` | Seek -10 s / +10 s (`,` and `.` are typed) |
| `ctrl+f` | Expand or restore the player (`f` is typed) |
| `enter` | On the input, a recent term, or a suggestion, open the RESULTS for that term (the input keeps it); on an artist, open its page; on a song, open its SONG view |
| `ctrl+d` / `delete` | On a recent term, delete it (on the input, they edit the text) |
| `tab` | Back to the stations (the search is kept for the next `/` or `tab`) |
| `esc` | Back one view (closing the search: the next `/` starts empty) |
| `ctrl+c` | Quit |

RESULTS, ARTIST, ALBUM, SONG, and PLAYLIST:

| Key | Action |
|-----|--------|
| `↑`/`↓` or `k`/`j` | Move the cursor; `↑` on the first row moves the focus to the nav tabs |
| `enter` | On RESULTS, open the selected artist, album, song (its SONG view), or playlist; elsewhere, play the top songs or tracks from the selected one, open an album or playlist, or fold the notes (MORE/LESS) |
| `esc` | Back one view |
| `tab` | Back to the stations (the page and the ones below it are kept for the next `/` or `tab`) |
| `/` | Back to the SEARCH input, with the term kept for editing (the pages above it are closed) |
| `r` | Retry after the page failed to load |
| `space`, `n` / `p`, seek keys, `→`, `f` / `ctrl+f`, `q` | As on the stations |

Player (after `→` from the list, or a click on its controls; the lit
panel frame shows which side has the focus):

| Key | Action |
|-----|--------|
| `←` / `→` | Walk `PREV`, `PLAY`/`PAUSE`, `NEXT`, `EXPAND`; `←` from `PREV` goes back to the list |
| `↑` / `↓` | Move to the progress bar (when the song can seek) and back to the buttons; `↑` from the bar (or from the buttons, with nothing to seek) moves the focus to the nav tabs |
| `←` / `→` on the progress bar | Seek -10 s / +10 s |
| `enter` | Press the focused button (as a click) |
| `esc` | Back to the list (restoring an expanded player) |
| `space`, `n` / `p`, seek keys, `f` / `ctrl+f` | As on the stations |

Nav tabs (after `↑` past the top of the list, the SEARCH input, or the
player; the focused tab shows a `▸`):

| Key | Action |
|-----|--------|
| `←` / `→` | Walk `STATIONS`, `SEARCH` and, on a page, `◀ BACK` |
| `enter` | Press the focused tab (as a click); the list takes the focus |
| `↓` / `esc` | Back where the focus came from (the same row, the SEARCH input, or the player's bar or button) |

On the tabs, any other key goes back where the focus came from and acts
there. On the player, any other key goes back to the list and acts there (on
SEARCH, a letter is typed). The expanded player hides the list, so it holds the focus: `←` from
`PREV` stays put, and going back to the list (`esc`, another key, `f`,
`ctrl+f`, or `RESTORE`) restores it. Expanding, by key or button, focuses
the player on the control it already had (`PLAY` from the list, `EXPAND`
once its button is pressed); restoring always gives the focus back to the
list, as it was left (on SEARCH, the same row or the input).

The footer names the keys of the side and view in focus; when it does not
fit, it keeps the essential ones first (on the stations: `enter`, `/`,
`space`, `→`) and always quit.

The list panel keeps one width in every view (the browse pages' width),
leaving NOW PLAYING at least 30 columns.

## Mouse

The mouse does what the keys do; the keys keep working.

| Click | Action |
|-------|--------|
| A row (station, search row, page row, MORE/LESS) | Select it and act as `enter` |
| The SEARCH input | Select the input |
| `✕` at the end of a recent term | Delete that term (as `ctrl+d` / `delete`) |
| A `[R] RETRY` notice | Retry, as `r` |
| `STATIONS` / `SEARCH` tabs (header rule) | As `tab` / `/`; the lit tab is the view shown |
| `◀ BACK` (on RESULTS, ARTIST, ALBUM, SONG, and PLAYLIST) | As `esc` |
| `◀◀ PREV`, `▶ PLAY` / `❚❚ PAUSE`, `NEXT ▶▶` (NOW PLAYING) | As `p`, `space`, `n`; the focus moves to the button |
| `⤢ EXPAND` / `⤡ RESTORE` (NOW PLAYING) | Expand the player to the full width, or restore it |
| The progress bar | Seek to that point of the song; the focus moves to the bar |

Clicks elsewhere move the focus back to the list. The wheel moves the
cursor like `↑`/`↓` (nothing while the player is expanded). Narrow layouts
shorten the transport buttons to their glyphs (`EXPAND` first) and leave
out buttons that do not fit; the tiny layout has none. In the compact
layout, expanding hides the list under the player.

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
| `searchCatalog` | `term` (not blank), `limit` (clamped to 1-25 per type; suggestions to 10, top results to 6) | `{"suggestions":[...],"top":[{"kind":"artist","artist":{...}},...],"artists":[...],"albums":[...],"songs":[...],"playlists":[...]}`; `kind` is `artist`, `album`, `song`, or `playlist` (other top result kinds are left out) |
| `artist` | `artistId` | `{"artist":{...},"topSongs":[...],"essentialAlbums":[...],"albums":[...],"singles":[...],"compilations":[...],"playlists":[...],"about":{"notes","genre","origin","formed"}}` |
| `album` | `albumId` | `{"album":{...},"tracks":[...],"genre","releaseDate","recordLabel","copyright","notes"}` |
| `songAlbum` | `songId` | Same as `album`, for the album holding the song |
| `catalogPlaylist` | `playlistId` | `{"playlist":{...},"tracks":[...],"notes"}` |
| `playlists` | none | `{"playlists":[{"id":...,"name":...}]}` |
| `playSongs` | `ids`, `startIndex` | `{}` or `{"missing":[...]}` |
| `playPlaylist` | `playlistId` | `{}` |
| `pause`, `resume`, `next`, `previous`, `stop` | none | `{}` |
| `seek` | `seconds` (>= 0) | `{}`, followed by a `state` event |

Playback commands run one at a time in arrival order, each bounded by 10 s
(a hung one is answered with a timeout error); `authorize`, `playlists`, and
the catalog commands run concurrently. Catalog commands stay within their own
time budget (`CatalogBudget` in `helper/Sources/Nu11SignalProtocol/Catalog.swift`),
below the Go client's deadline; an artist page section that fails or hangs is
left empty instead of failing the page. At stdin EOF the helper finishes in-flight work
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
