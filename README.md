# Nu11Signal

A lightweight terminal player for Apple Music, styled after a neon cyberpunk
car radio: your library PLAYLISTS sit on a pseudo FM dial, with a
now-playing panel, a volume readout, music-driven data rain, and Apple Music style catalog browsing
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

### Font

The UI is designed with [Kode Mono](https://fonts.google.com/specimen/Kode+Mono),
which the cask installs (`depends_on cask: "font-kode-mono"`; by hand:
`brew install --cask font-kode-mono`). A terminal UI cannot choose its font:
the terminal draws every character with the font it is set to, so Kode Mono
shows once your terminal uses it, for example `font-family = Kode Mono` in
Ghostty's config or the profile font in Terminal.app or iTerm2. Any
monospaced font works; the frames, blocks and rain glyphs look the same in
all of them.

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

MusicKit exposes no audio samples, but in [app volume](#app-volume) mode the
helper renders the music itself, so the rain plays its real spectrum (see
[Spectrum](#spectrum) and [Rain](#rain)). Otherwise (system volume,
`--demo`, macOS before 15) the rain is decorative: it follows the playback
state, not the sound.

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

Views stack like Apple Music's: PLAYLISTS → SEARCH → RESULTS → ARTIST → ALBUM,
SONG, or PLAYLIST. `esc` goes back one view, `tab` returns to the playlists. Leaving
with `tab` keeps the search branch as it was: `/` or `tab` from the playlists
brings back the same view (an artist or album page included) with its cursor.

`enter` on a library playlist opens its PLAYLIST page over the PLAYLISTS
root: `▶ PLAY` plays it from the start, and `enter` on a track plays the
playlist from that track (the dial then shows the playlist on air). The songs
the page loaded are queued by their catalog ids (`playSongs`), so playing
never reads the playlist again. Songs
that are only in your library (uploads, or songs no longer in the Apple Music
catalog) are listed muted and skipped: `enter` on one only shows a notice. `esc` or
the `PLAYLISTS` tab goes back to the list; `tab` or `/` goes to SEARCH (the
page is closed).

A song picked from a list plays with the rest of that list queued around it,
so playback goes on into the list:

| Picked from | Queue |
|-------------|-------|
| SEARCH song row | The songs listed under the input, from the picked one |
| RESULTS, SONGS | The SONGS section, from the picked song |
| RESULTS, TOP RESULTS song | The SONGS section, from the song's copy there; a top song missing from SONGS plays first, the section after it |
| ARTIST, TOP SONGS | The top songs, from the picked one |
| ALBUM, SONG | The album, from the picked track (a SONG view whose album failed to load has only the song) |
| PLAYLIST | The playlist, from the picked track (a library playlist without its library-only songs) |

On SEARCH and RESULTS, `g` on a song row opens its SONG view (the album
holding it) instead.

| View | Shows |
|------|-------|
| SEARCH | RECENT searches while the input is empty; once you type 2+ characters, live suggestions, then matching artists (with their genre) and songs |
| RESULTS | The full search for a submitted term, non-empty sections only: TOP RESULTS (tagged ARTIST, ALBUM, SONG, or PLAYLIST), ARTISTS, ALBUMS (with artist and year), SONGS (with artist), PLAYLISTS (with curator) |
| ARTIST | Its non-empty sections in Apple Music order: TOP SONGS, ESSENTIAL ALBUMS, ALBUMS, ARTIST PLAYLISTS, SINGLES & EPS, COMPILATIONS, then ABOUT (editorial notes folded behind MORE, FROM, FORMED, GENRE) |
| ALBUM | The tracks (by disc when there are several), release date, song count and length, copyright, record label, and notes |
| SONG | The album holding the song (`g` on a SEARCH or RESULTS song row), with the cursor on the song; if the album cannot be loaded, the song alone, still playable |
| PLAYLIST | The tracks with their artists, song count and length, curator, and notes; a library playlist starts with `▶ PLAY` and names its frequency |

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
- Playlists cannot be renamed or deleted, nor songs removed from them: the
  Apple Music API does not offer it. A page already open does not show songs
  added from the picker until it is opened again.

## Editing the library

- **Love** a song (Apple Music's favorite): `l` on a song row (a track, a
  top song, a song among the results or the search rows), or with no song
  row selected (the playlists, the player), the song playing. The selected
  song row ends in `-- +` (`<3 +` once loved, the `<3` lit); a loved song
  keeps its `<3` on any row, and NOW PLAYING shows a `[--]` / `[<3]` button
  beside the title. A page reads the state of all its songs in one
  `favorites` call as it loads (album, song, playlist, artist top songs,
  results, search rows), so its marks show at once; states are cached per song. The song playing, when
  it is not on the page, and the selected song, when the page read failed,
  are read on their own when the selection rests (on the next animation
  tick). A change shows at once: a refused one is reported on the status
  line and read back, and a failed single read is tried again 30 seconds
  later.
  One change per song is sent at a time; pressing `l` again meanwhile only
  moves the heart, and the latest state is sent once the first answers.
- **Add to a playlist**: `a` (or the row's `+`) opens ADD TO PLAYLIST over
  the list: `+ NEW PLAYLIST`, then the playlists songs can be added to
  (ones followed from the catalog, such as "Canciones favoritas", are left
  out). `enter` adds the selected song and closes the picker; a refusal keeps
  it open. An add that timed out may still be applied, so the picker closes
  and the status line asks you to check the playlist before trying again.
  `esc` or `◀ BACK` cancels.
- **New playlist**: the `+ NEW PLAYLIST` row over the playlists (for the keys
  and the mouse alike), or the picker's row (the new playlist then holds the
  song).
  Type the name, `enter` (or `CREATE`) creates it, `esc` (or `CANCEL`)
  cancels (back to the picker when it came from there). The new playlist is
  listed and selected at once; the list is read again, and a playlist the
  API does not list yet is kept. A create that timed out closes the name
  input too (check the playlists before trying again); the list is read
  again and selects the new playlist if it shows up.
- One add or create is in flight at a time: until it answers, another one
  shows `WRITING…` and is not sent.

Songs only in your library cannot be loved or added (the ratings and
playlist endpoints take catalog ids): `l` and `a` show a notice.

## Keys

PLAYLISTS (and anywhere the key is not taken by the view):

| Key | Action |
|-----|--------|
| `↑`/`↓` | Move the cursor; `↑` on the top row (`+ NEW PLAYLIST`) moves the focus to the nav tabs (see below) |
| `enter` | Open the playlist's page; on `+ NEW PLAYLIST`, name a new playlist |
| `l` | Love the song playing, or unlove it |
| `a` | Add the song playing to a playlist |
| `o` | Cycle the loop (repeat) mode: `OFF`, `ALL` (the queue starts over), `ONE` (the song starts over) |
| `space` | Play / pause |
| `n` / `p` | Next / previous track |
| `shift+←` / `shift+→` or `,` / `.` | Seek -10 s / +10 s |
| `k` / `j`, `+` / `=` and `-`, or `shift+↑` / `shift+↓` | Volume up / down by 5% |
| `→` | Move the focus to the player (see below) |
| `f` / `ctrl+f` | Expand the player to the full width, or restore it |
| `/` | Back to the search left with `tab` (same view and cursor); otherwise open SEARCH with an empty input |
| `tab` | Same as `/` |
| `esc` | Back one view |
| `r` | Retry loading the playlists after a failure |
| `x` | Turn the signal effects off or on (see [Signal effects](#signal-effects)) |
| `q` / `ctrl+c` | Quit |
| `?` | Open or close KEYS, the list of every key (see below) |
| `s` | Open or close SETTINGS, the color themes (see [Settings](#settings)) |

SEARCH (typing goes to the input, so letter shortcuts are off):

| Key | Action |
|-----|--------|
| `↑`/`↓` | Move between the input and the rows; `↑` on the input moves the focus to the nav tabs |
| `←`/`→` | On the input, move the text cursor; on a row, `→` moves the focus to the player |
| `shift+←` / `shift+→` | Seek -10 s / +10 s (`,` and `.` are typed) |
| `shift+↑` / `shift+↓` | Volume up / down (`k`, `j`, `+`, `=` and `-` are typed) |
| `ctrl+f` | Expand or restore the player (`f` is typed) |
| `enter` | On the input, a recent term, or a suggestion, open the RESULTS for that term (the input keeps it); on an artist, open its page; on a song, play it with the other songs listed queued (SEARCH stays open) |
| `ctrl+d` / `delete` | On a recent term, delete it (on the input, they edit the text) |
| `l` / `a` / `g` | On a song row, love it / add it to a playlist / open its SONG view (on the input or another row, they are typed) |
| `tab` | Back to the playlists (the search is kept for the next `/` or `tab`) |
| `esc` | Back one view (closing the search: the next `/` starts empty) |
| `ctrl+c` | Quit |

RESULTS, ARTIST, ALBUM, SONG, and PLAYLIST:

| Key | Action |
|-----|--------|
| `↑`/`↓` | Move the cursor; `↑` on the first row moves the focus to the nav tabs |
| `enter` | On RESULTS, open the selected artist, album, or playlist, or play the selected song with the rest of its list (see above); elsewhere, play the top songs or tracks from the selected one, open an album or playlist, or fold the notes (MORE/LESS) |
| `g` | On a RESULTS song, open its SONG view |
| `esc` | Back one view |
| `tab` | Back to the playlists (the page and the ones below it are kept for the next `/` or `tab`); from a library playlist page, over to SEARCH |
| `/` | Back to the SEARCH input, with the term kept for editing (the pages above it are closed) |
| `r` | Retry after the page failed to load |
| `l` | Love the selected song (a track, a top song, a song result), or unlove it; on another row, the song playing |
| `a` | Add the selected song to a playlist; on another row, the song playing |
| `space`, `n` / `p`, seek, volume and loop keys, `→`, `f` / `ctrl+f`, `x`, `q` | As on the playlists |

ADD TO PLAYLIST and NEW PLAYLIST (over the list):

| Key | Action |
|-----|--------|
| `↑`/`↓` | Move between `+ NEW PLAYLIST` and the playlists (picker) |
| `enter` | Add the song to the selected playlist, or on `+ NEW PLAYLIST` name a new one (picker); create the playlist (name) |
| `esc` | Cancel (from a name opened in the picker, back to the picker) |
| `space`, `n` / `p`, seek and volume keys, `→`, `q` | As on the playlists (picker only: the name types every key but `enter` and `esc`; `ctrl+c` quits) |

Player (after `→` from the list, or a click on its panel; the lit
panel frame shows which side has the focus):

| Key | Action |
|-----|--------|
| `←` / `→` | Walk a row of buttons as drawn: `PREV`, `PLAY`/`PAUSE`, `NEXT`, `LOOP`, `VOL-`, `VOL+`, `EXPAND` when the player is wide enough for one row, else `PREV` to `EXPAND` on the transport row and `VOL-`, `VOL+` on the volume row below; the `[<3]` of the song playing is alone on its row; `←` from the first button of a row goes back to the list |
| `↑` / `↓` | Move between the `[<3]` of the song playing (while there is one), the progress bar (when the song can seek), the transport row and, with two rows, the volume row (`VOL-` under `PREV`/`PLAY`, `VOL+` under `NEXT`, `LOOP` and `EXPAND`); `↑` from the `[<3]` (or from what is under it, with no song) moves the focus to the nav tabs |
| `←` / `→` on the progress bar | Seek -10 s / +10 s |
| `enter` | Press the focused button (as a click) |
| `l` / `a` | Love / add to a playlist the song playing |
| `esc` | Back to the list (restoring an expanded player) |
| `space`, `n` / `p`, seek, volume and loop keys, `f` / `ctrl+f` | As on the playlists |

Nav tabs (after `↑` past the top of the list, the SEARCH input, or the
player; the focused tab shows a `▸`):

| Key | Action |
|-----|--------|
| `←` / `→` | Walk `PLAYLISTS`, `SEARCH` and, on a page, `◀ BACK` |
| `enter` | Press the focused tab (as a click); the list takes the focus |
| `↓` / `esc` | Back where the focus came from (the same row, the SEARCH input, or the player's bar or button) |
| `space`, `n` / `p`, seek, volume and loop keys | Act on the player; the tabs keep the focus (on SEARCH too: nothing is typed) |

On the tabs, any other key goes back where the focus came from and acts
there. On the player, any other key goes back to the list and acts there (on
SEARCH, a letter is typed). The expanded player hides the list, so it holds the focus: `←` from
`PREV` stays put, and going back to the list (`esc`, another key, `f`,
`ctrl+f`, or `RESTORE`) restores it. Expanding, by key or button, focuses
the player on the control it already had (`PLAY` from the list, `EXPAND`
once its button is pressed); restoring always gives the focus back to the
list, as it was left (on SEARCH, the same row or the input).

The footer names the keys of the side and view in focus; when it does not
fit, it keeps the essential ones first (on the playlists: `enter`, `/`,
`space`, `→`) and always `[?] KEYS` and quit (where `?` is typed, on SEARCH
and in the NEW PLAYLIST name, only quit). `[S] SETTINGS` shows only where
the whole footer fits.

`?` opens KEYS, every binding grouped (PLAYBACK, NAVIGATION, VIEW, SEARCH,
APP) in a panel over the body, wherever `q` quits (on SEARCH and in the
NEW PLAYLIST name `?` is typed). `?` or `esc` (or a click) closes it; while
it is open the other keys do nothing, `q` included, and `ctrl+c` quits.

The HUD names live state: the nav bar ends in the Night City net node the
radio is patched through (`NODE 7F // NC-GRID`, flavor, fixed for a session
by its seed, cut to fit); the NOW PLAYING frame says where the rain's levels come from
(`SPECTRUM LIVE` from the player's readings, `SPECTRUM SIM` animated,
`SPECTRUM HOLD` paused or stopped); and with no message the status line
names the song `n` moves to (`UP NEXT // RESONANCE · HOME`) when the song
playing is in the list nu11signal queued (after the last one, the first
while the loop repeats), else the volume driven and the effects
(`APP VOLUME // FX ON`).

The list panel keeps one width in every view (the browse pages' width),
leaving NOW PLAYING at least 30 columns.

## Signal effects

The screen now and then loses the signal: every 10–22 s a short glitch
burst of 0.6–1 s tears one to three rows 1–2 cells sideways, corrupts a
few cells (4–10, twice as many on NO SIGNAL) and, on about one frame in
three, runs a static bar across a row, all changing every frame; about one burst in four also flashes a bold
red `NO SIGNAL` framed in red static for its whole length. A real message
on the status line is never glitched. The effects are drawn over the
frame, so clicks and keys work during a burst.

New content scrambles in: when text appears that was not on screen
(another tab or page, a list or search results arriving, the ADD TO
PLAYLIST picker or NEW PLAYLIST editor opening, the KEYS or SETTINGS
overlay opening or closing, a new artist, album or feed in NOW PLAYING),
about half of its new characters (a seeded pick) show
light glyphs, uppercase letters, digits and a few thin symbols, each
keeping its color. They hold for a moment, then resolve left to right
within about 0.9 s, quickly at first and settling gently at the end; the
glyphs drift every ~140 ms, each cell at its own moment, instead of jumping
all at once. Only text that changed intros: the clock, progress, volume and
rain never do, moving the
cursor (in a list or in SETTINGS) or scrolling a list does not, and the SEARCH input and the playlist
name never scramble while you type (live results intro once as they
arrive). Intros follow the same switch as the effects.

`x` turns them off or on; `nu11signal --calm` (or `NU11SIGNAL_CALM=1`)
starts with them off. They pause while the SEARCH input or a NEW PLAYLIST
name takes the keys (intros keep running, off the input line), and on the
tiny layout. They add no timer: the animation tick sleeps until the next
burst is due; a burst runs at about 15 fps and an intro at 20 fps. The terminal is redrawn at most 20 times a second, not Bubble
Tea's default 60, to keep the process's wakeups (and battery use) low.

## Rain

The spectrum area at the bottom of NOW PLAYING draws data rain: hex digits
and half-width katakana fall down every other column, a bright head over a
trail that dims, in the spectrum's colors (yellow, bold red, red, dim red).
In app volume mode (see [Spectrum](#spectrum)) it plays the music:

- Each column follows the band under it through a curve that keeps quiet
  bands completely dry and stands the loud ones out: the louder the band,
  the more often drops start there, the faster and longer they fall, and
  the brighter their heads burn, up to yellow. Silence is dry.
- A hit (a band jumping over its recent average, the bass counting more)
  bursts: a bass hit sends a wave of new drops across every sounding
  column, a higher one under its own band, as many as the hit is strong,
  and every head flashes brighter for a few frames. A steady passage never
  flashes.
- The waveform's loudness scales how often drops start.

Without readings (system volume, `--demo`, macOS before 15) it drizzles
slowly and dimly instead. When paused the rain holds still and the
animation tick slows down to once a second. It draws on the existing
animation frames, from the seed: the signal effects run over it.

The other visualizers (bars, oscilloscope, synthwave, random) and the `v`
key are gone. A `nu11signal/config.json` under `os.UserConfigDir()`
(`~/Library/Application Support/nu11signal/config.json` on macOS) is still
read once at startup (it also holds the theme, see [Settings](#settings)),
but its `"visualizer"` value,
whatever it names, is accepted and ignored without a notice; a file that is
not valid JSON says so once on the status line.

## Settings

`s` opens SETTINGS, a panel over the body like KEYS, wherever `?` opens
KEYS (on SEARCH and in the NEW PLAYLIST name `s` is typed). Its THEMES
section lists the color themes, the active one marked `◉`:

| Theme | Look |
|-------|------|
| `NIGHT CITY` | The default: neon red frames and text, cyan and yellow highlights |
| `BLUE` | NIGHT CITY recolored from the gentleman-blue palette, with as many colors: electric blue where NIGHT CITY is red, violet where it is yellow, cyan where it is cyan, blue-grey shades behind |

`↑`/`↓` move, `enter` (or a click on a row) applies the theme at once, the
whole UI recolored (frames, text, buttons, the rain, the signal effects),
and the panel stays open to compare; `s` or `esc` (or a click off the rows)
closes it. The other keys do nothing while it is open; `ctrl+c` quits.

The choice is saved to `"theme"` in `nu11signal/config.json` under
`os.UserConfigDir()` and applied at the next start:

```json
{"theme": "BLUE"}
```

The file is written only when a theme is chosen, atomically (a temporary
file renamed over it), private (`0600`, its directory `0700`), keeping the
fields it does not know. A file that is not valid JSON is left alone and
the choice is not saved (the status line says so); a theme name nu11signal
does not know starts `NIGHT CITY` silently.

## Mouse

The mouse does what the keys do; the keys keep working. Every control is
one control for both: each button the mouse can press, the keyboard focus
reaches too (or, for the controls ending a row, the row's own keys: `l`,
`a`, `delete`), and there is no mouse-only duplicate of a row.

| Click | Action |
|-------|--------|
| A row (playlist, search row, page row, `▶ PLAY`, MORE/LESS) | Select it and act as `enter` |
| The SEARCH input | Select the input |
| `✕` at the end of a recent term | Delete that term (as `ctrl+d` / `delete`) |
| `--` / `<3` and `+` at the end of the selected song row | Love or unlove it (as `l`); add it to a playlist (as `a`) |
| `[--]` / `[<3]` beside the title (NOW PLAYING) | Love or unlove the song playing; the focus moves to the button |
| `+ NEW PLAYLIST` (row over the playlists) | Name a new playlist |
| A picker row, `CREATE`, `CANCEL` | As `enter` on the row; create; cancel |
| A `[R] RETRY` notice | Retry, as `r` |
| `PLAYLISTS` / `SEARCH` tabs (header rule) | `PLAYLISTS` as `tab` (from a library playlist page, back to the list); `SEARCH` as `/`; the lit tab is the branch shown |
| `◀ BACK` (on RESULTS, ARTIST, ALBUM, SONG, and PLAYLIST, and over ADD TO PLAYLIST and NEW PLAYLIST) | As `esc` |
| `[◀◀]`, `[ ▶ PLAY ]` / `[ ❚❚ PAUSE ]`, `[▶▶]` (NOW PLAYING) | As `p`, `space`, `n`; the focus moves to the button |
| `[⤢]` EXPAND / `[⤡]` RESTORE (NOW PLAYING, at the right edge) | Expand the player to the full width, or restore it |
| `[−]` / `[+]` around the `VOL` meter (NOW PLAYING) | Volume down / up by 5%, as `j` / `k`; the focus moves to the button |
| `[↻ OFF]` / `[↻ ALL]` / `[↻ ONE]` (NOW PLAYING, after `NEXT`) | Cycle the loop mode, as `o`; the focus moves to the button |
| Anywhere else in a panel (its frame and empty space included) | The panel takes the focus: the list keeps its cursor (nothing opens); NOW PLAYING focuses `PLAY`, or keeps the button it had |
| The progress bar | Seek to that point of the song; the focus moves to the bar |

Other clicks on the nav bar or the header do nothing. The wheel moves the
cursor like `↑`/`↓`, stopping at the top of the list (it never reaches the
nav tabs; nothing while the player is expanded). When NOW PLAYING is wide
enough (the expanded player, for one), every control takes one row:

```
[◀◀]  [ ❚❚ PAUSE ]  [▶▶]  [↻ OFF]  VOL [−] ▮▮▮▮▮▮▮▮ [+] 90%  [⤢]
```

with the meter as wide as fits; narrower, the volume row goes under the
transport row. Narrow layouts shorten `PLAY`/`PAUSE` to its glyph, tighten
the gaps and leave out buttons that do not fit; the tiny layout has none.
The compact layout puts the volume row beside the transport buttons when it
fits (the buttons as glyphs), and leaves it out otherwise. In the compact
layout, expanding hides the list under the player.

`LOOP` shows the mode asked for at once and keeps it until the player
reports it (for at most 3 seconds); a refused change is reported on the status line and the button
shows the player's mode again. A mode changed elsewhere (the Music app)
shows with the next state.

The `VOL` readout shows Nu11Signal's own volume, independent of the system
volume; `SYS` in its place means it shows (and changes) the system output
volume instead, the fallback. It is read at startup (`VOL --` until then, or
when the output device has no settable volume; a refused change is reported
on the status line) and again when the mode changes. Changes show at once;
rapid presses are coalesced, so only the latest level is sent once the
previous change answers.

### App volume

MusicKit plays the helper's audio in a separate macOS process
(`RemotePlayerService`), so the helper cannot scale its own output. Instead
it creates a Core Audio process tap (macOS 14.2+) on that process, muting
its direct output, and plays the tapped audio through a private aggregate
device on the default output device, scaled by the app volume (a square-law
curve, ramped over 15 ms so changes do not click). This adds about 60 ms of
output latency. The level persists in the helper's preferences.

- **Permission.** A tap needs the *audio capture* permission
  (`NSAudioCaptureUsageDescription`: "Nu11Signal adjusts its own playback
  volume…"). macOS asks on the first playback; the music plays at the system
  volume (`SYS`) until it is granted, and stays there if it is denied (grant
  it later in System Settings › Privacy & Security › Screen & System Audio
  Recording, then restart the app). Nothing is muted while the prompt waits.
- **Launch.** macOS attributes that permission to the *responsible* process,
  which for a program started from a terminal is the terminal. The helper
  therefore re-executes itself once at startup, in place (same process, same
  pipes), with the private `responsibility_spawnattrs_setdisclaim` spawn
  attribute (as LLDB and Chromium do), so the prompt names Nu11Signal. When
  the private function is missing, the helper stays on the system volume.
- **Which process.** Only the `RemotePlayerService` copy macOS holds the
  helper responsible for is tapped. Other apps' copies are never touched
  (a muted tap on one would silence that app); if the helper's copy cannot
  be identified, the system volume is used.
- **Resources.** The tap is built when playback starts; its audio thread stops
  while paused and everything is released when playback stops, when a play
  fails, or when the app quits. A new default output device or player
  process rebuilds it: the new tap is built first and the old one is
  released only once the new one plays.
- **Failures.** A failed rebuild (common in the middle of an output device
  change) is retried three times over about five seconds. The old muted
  tap is kept meanwhile, so after an output device change the music is
  silent rather than jumping to the system volume. When no tap mutes the
  music (the player moved to a new process, or no tap could be built yet)
  and the app volume is below full, the music is paused during the retries
  and resumes when one succeeds; at full app volume it keeps playing, as
  the system volume is then no louder. If it keeps failing, the rest of the
  session uses the system volume and the status line says `APP VOLUME OFF`.
  The system volume is never changed for you: if the music was playing
  quieter than the system volume, it is paused instead (or stays paused),
  and plays at the system volume (`SYS`) when you resume.
- **Opting out.** `NU11SIGNAL_VOLUME_MODE=system` in the environment keeps the
  system volume (no relaunch, no tap, no prompt).

### Spectrum

In app volume mode (macOS 15+), the tap's audio thread also copies each
buffer, before the app volume is applied (so the bars do not change with
the volume) and mixed to mono, into a lock-free ring buffer. A timer on a
utility queue, running only while that thread runs (playing in app mode),
reads the newest 2048 samples 15 times a second, applies a Hann window and
a vDSP FFT, sums the bins into 24 log-spaced bands from 40 Hz to 16 kHz,
maps each band's power from -50 dBFS (empty) to -10 dBFS (full), smooths
it (fast attack, a fall of about a second) and emits
`{"event":"levels","bands":[0-100, ...],"wave":[-100-100, ...]}`: `wave` is
the same 2048 samples decimated to 64 points, each keeping its stretch's
largest swing (so a transient survives), in hundredths of full scale, for
the rain's overall intensity (see [Rain](#rain)); a helper that omits it
still works. The TUI resamples the 24 bands to
the bars the panel has room for on its own 10 fps playing frame. When a
song starts or resumes in app mode the bars hold where they are (usually
flat) until the first reading arrives, since the tap takes a moment to
attach; a reading from before a pause, or an empty one, never counts. If
no reading arrives within 1.5 s of playing, or none arrived in the last
500 ms, the bars turn decorative, easing from their heights; when readings
come back the bars glide onto them over four frames. When paused the bars
fall to zero. Nothing is measured in system volume mode.

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
| Event | `{"event":"ready"}`, `{"event":"state","state":{...}}`, `{"event":"error","message":"<msg>"}`, `{"event":"levels","bands":[...],"wave":[...]}` (app volume mode only; see [Spectrum](#spectrum)) |

| Command | Args | Result |
|---------|------|--------|
| `authorize` | none | `{"status":...}`: `authorized`, `denied`, `restricted`, or `notDetermined` |
| `searchCatalog` | `term` (not blank), `limit` (clamped to 1-25 per type; suggestions to 10, top results to 6) | `{"suggestions":[...],"top":[{"kind":"artist","artist":{...}},...],"artists":[...],"albums":[...],"songs":[...],"playlists":[...]}`; `kind` is `artist`, `album`, `song`, or `playlist` (other top result kinds are left out) |
| `artist` | `artistId` | `{"artist":{...},"topSongs":[...],"essentialAlbums":[...],"albums":[...],"singles":[...],"compilations":[...],"playlists":[...],"about":{"notes","genre","origin","formed"}}` |
| `album` | `albumId` | `{"album":{...},"tracks":[...],"genre","releaseDate","recordLabel","copyright","notes"}` |
| `songAlbum` | `songId` | Same as `album`, for the album holding the song |
| `catalogPlaylist` | `playlistId` | `{"playlist":{...},"tracks":[...],"notes"}` |
| `playlists` | none | `{"playlists":[{"id","name","editable"}]}`: alphabetical, with Apple Music API library ids (`p.…`); `editable` is false for playlists followed from the catalog |
| `libraryPlaylist` | `playlistId` (an API library id, `p.…`) | `{"playlist":{"id","name"},"tracks":[...],"notes"}`; songs only, with their catalog ids; a song not in the catalog keeps its library id (`i.…`) and carries `"libraryOnly":true` |
| `playSongs` | `ids`, `startIndex` | `{}`, or any of `{"missing":[...],"skipped":[...],"startedAlone":true}` (see [Queue preparation](#queue-preparation)) |
| `playPlaylist` | `playlistId`, optional `startIndex` (an index into the `libraryPlaylist` tracks) | Same as `playSongs`; the playlist's catalog songs are queued (library-only songs are skipped; starting at one is an error). The UI plays library playlists with `playSongs` from the tracks it loaded instead |
| `pause`, `resume`, `next`, `previous`, `stop` | none | `{}` |
| `setRepeat` | `mode`: `off`, `all` (the queue), or `one` (the current song) | `{}`; `state` events report the mode as `repeat` (`off` when the player has none; the UI reads any mode it does not know as `off`) |
| `seek` | `seconds` (>= 0) | `{}`, followed by a `state` event |
| `volume` | none | `{"level":...,"mode":"app"\|"system"}`: the app volume (`app`) or, as a fallback, the system output volume (`system`), 0 to 1 (runs concurrently); `state` events carry the mode as `volumeMode`; see [App volume](#app-volume) |
| `setVolume` | `level` (clamped to 0-1) | `{"level":...,"mode":...}` as `volume` reports after the change; in `app` mode the system volume is never touched; an error when the system output device's volume cannot be changed (runs concurrently) |
| `createPlaylist` | `name` (not blank), optional `description`, optional `songIds` (in order) | `{"id","name"}`: the new playlist, with its Apple Music API library id (`p.…`) |
| `addToPlaylist` | `playlistId` (an API library id, `p.…`), `songIds` (not empty) | `{}`, or an error such as `playlist is not editable` |
| `favorite` | `songId` | `{"favorite":true\|false}`: whether the song is loved (no rating is `false`; an unreadable ratings answer fails the command) |
| `favorites` | `songIds` (possibly empty) | `{"favorites":{"<id>":true\|false,...}}`: every requested id, loved or not; one ratings read per 100 ids of each kind (runs concurrently); an unreadable ratings answer fails the command |
| `setFavorite` | `songId`, `on` (a boolean) | `{}`: `true` loves the song, `false` removes its rating (a song without one included) |

Playback commands (`setRepeat` included) run one at a time in arrival order, each bounded by 10 s
(a hung one is answered with a timeout error); `authorize`, `playlists`, and
the catalog commands run concurrently. Catalog commands stay within their own
time budget (`CatalogBudget` in `helper/Sources/Nu11SignalProtocol/Catalog.swift`),
below the Go client's deadline; an artist page section that fails or hangs is
left empty instead of failing the page. The library edits (`createPlaylist`,
`addToPlaylist`, `favorite`, `setFavorite`) and the `favorites` read run
concurrently too, one Apple Music API request each (`/v1/me/library/playlists`,
`/v1/me/ratings/...`; `favorites` sends one `GET /v1/me/ratings/songs?ids=...`
or `.../library-songs?ids=...` per batch) through
MusicKit's `MusicDataRequest`, as MusicKit's own library editing is unavailable on
macOS. `playlists` and `libraryPlaylist` read the library through the same API
(`GET /v1/me/library/playlists` and `.../{id}/tracks`, following pages up to 500
playlists or 1000 songs), so every id they return is one the edits accept: song
ids are catalog ids or API library ids (`i.…`), playlist ids are `p.…`. Ids are
checked (letters, digits, and dots only) before they go into a request path. A
`createPlaylist` or `addToPlaylist` that times out may still be applied: its
error says the outcome is unknown, and it is not retried. At stdin EOF the helper finishes in-flight work
(up to 3 s), stops playback, and exits.

### Queue preparation

On macOS, `ApplicationMusicPlayer` refuses a queue whose first song comes from
the catalog when the queue also holds a song of this Mac's local library
(`MPMusicPlayerControllerErrorDomain` code 6, "Failed to prepare to play" or
"Prepare queue failed with unexpected start item"), although each song prepares
on its own. So `playSongs` and `playPlaylist` look the queued songs up in the
local library first (`MusicLibraryRequest`, up to 1.5 s): when the chosen song
is in it, every local song is queued as its library copy; otherwise the local
songs are left out and listed as `"skipped"`. If the player still cannot prepare
the queue, the chosen song is queued on its own (`"startedAlone":true`) and,
once it plays, the songs after it are appended to the queue, so the list
still plays on (a failed append is only logged: the song plays alone); if
even the song alone fails, the error names the song. These steps are logged
to the helper's stderr. `state` events name a song queued as its library
copy by the catalog id asked for, so the UI's `▶` and favorite marks match.

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
