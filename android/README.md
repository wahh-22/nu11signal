# Android TV app shell

This APK runs the real Go/Bubble Tea TUI on a Go-owned PTY. A local xterm.js
WebView covers a music.apple.com WebView, which owns MusicKit playback and
its Apple ID child sign-in window. It is a platform-only Java app: no Gradle,
AndroidX, or npm step during APK builds.

## Build

From the repository root:

```sh
make android                               # dev version, debug signing key
DEBUG=1 android/build.sh                   # debuggable, WebView inspection
make android-release VERSION=0.5.0 DRY_RUN=1 # real release key; verify, no dist
make android-release VERSION=0.5.0         # verified APK + checksum in dist/v0.5.0
```

Requires Go, JDK 11 or newer, Android SDK platform 34, build-tools 35.0.0,
NDK 28.2.13676358, and zip. Both armeabi-v7a and arm64-v8a executables are
built with CGO and API 28 clang. Version stamping matches the release scripts:
`main.version` and `main.versionStamp=nu11signal-version:<version>;`.

| Environment | Default / purpose |
| --- | --- |
| `VERSION` | `dev`; APK name, binary version and Android versionCode |
| `DEBUG` | `0`; `1` enables debuggable/WebView inspection (never for releases) |
| `ANDROID_HOME` | `~/Library/Android/sdk` |
| `ANDROID_NDK` | `$ANDROID_HOME/ndk/28.2.13676358` |
| `BUILD_TOOLS` | `35.0.0` |
| `ANDROID_PLATFORM` | `android-34` (manifest remains target 34) |
| `NU11SIGNAL_TV_KEYSTORE` | Custom signing keystore path |
| `NU11SIGNAL_TV_KEY_ALIAS` | Custom key alias |
| `NU11SIGNAL_TV_KEYSTORE_PASS` | Custom keystore password |
| `NU11SIGNAL_TV_KEY_PASS` | Optional key password, defaults to store password |
| `ANDROID_SERIAL` | Required only for optional install |

`make android` always uses version `dev` and clears custom signing variables.
For direct `android/build.sh` builds, set all three required signing variables together. Otherwise the build
creates and reuses `android/build/debug.keystore`. Passwords are passed to
apksigner through environment variables, not command-line values. A debug
signing key does not make the application debuggable: WebView inspection is
only enabled when Android's application debuggable flag is present.

Release builds use the persistent keystore/keychain setup documented in
[Releasing — Android TV APK](../docs/releasing.md#android-tv-apk). They override
inherited `DEBUG=1`, verify the signer and reject debuggable APKs before staging.
CI can inject the signing password through its secret environment; do not put
passwords in command lines, logs, or repository files.

For semver versions, `versionCode = MAJOR*10000 + MINOR*100 + PATCH`;
minor/patch must be 0–99, and the result must be 1–2100000000. `dev` uses 1.
Prerelease/build metadata does not change the code: use a higher core version
for each published update (including prereleases).

Output: `android/build/nu11signal-tv-<version>.apk`. Build outputs, caches,
verified vendor downloads, and signing material are ignored. Each build
uses a fresh Java packaging directory to avoid stale class files.

Optional installation (never part of the default build):

```sh
ANDROID_SERIAL=<serial> android/build.sh install
```

## Assets and terminal

`android/vendor-xterm.sh` downloads pinned xterm 6.0.0, addon-fit 0.11.0,
and compatible addon-webgl 0.19.0 archives and verifies their SHA-512 digests
before copying UMD assets and
MIT licenses. The exact URLs and integrity values are in
`assets/xterm/README.md`. Commit these assets; subsequent builds use local
files only (Go dependencies must already be cached for fully offline builds).

`android/vendor-font.sh` downloads Kode Mono from a pinned Google Fonts
commit and verifies its SHA-256 before copying it with the SIL OFL 1.1 license.
Source and checksum details are in `assets/fonts/README.md`. The build fails
if the font or license is missing. Run both vendor scripts to refresh assets.

`android/brand.sh` renders the app's logo images from the Blueshift vector
logo (`docs/assets/brand/logo/nu11signal-blueshift.svg`, the default theme)
with `rsvg-convert`: the adaptive launcher icon foreground (the head, on the
Blueshift ink background from `res/values/colors.xml`), the 320x180dp home
screen banner (full logo), the update-dialog mark and the white notification
icon, at every density. The PNGs are committed, so builds need no SVG tools;
rerun the script after the logo changes. The playback notification and the
Google TV now-playing card show the logo because the web player provides no
album art.

The terminal has a black background and truecolor support. It loads local
Kode Mono (the desktop's designed font) with `document.fonts.load` before
measuring or opening xterm. WebGL renders custom block, braille, and box
glyphs instead of relying on the font, with line height 1 and letter spacing 0.
If WebGL is unavailable or its context is lost, the addon is disposed and
xterm falls back to its DOM renderer (which may look different).

`TARGET_COLUMNS = 161` and `TARGET_ROWS = 44` in `assets/terminal.html`
match the desktop grid; `?cols=N&rows=N` independently override them with
positive integers. After loading Kode Mono, a `W` at 100px measures cell
width and natural height per font pixel (xterm line height multiplier 1).
`terminalFontSize()` takes the smaller of viewport width / (columns * cell
width) and viewport height / (rows * cell height), without a 12px floor.
Resizes recompute the size; FitAddon reports the actual grid, with a small
size correction if scrollbar allowance or pixel rounding undershoots the
target. One dimension may have extra cells. The full TUI needs at least 60x16.
stdout is collected for up to 16 ms or 32 KiB, then passed as base64-decoded
bytes to xterm. Only one terminal JavaScript evaluation may be posted or
in flight; until its ValueCallback completes, further output coalesces in
a pending buffer capped at 256 KiB. While the Activity is started, producers wait at the cap rather than
dropping bytes or queuing more evaluations. While stopped, output is drained
and discarded instead (see Background playback below). Raw
Android keys never reach xterm's own input handler. xterm's `onData` still
forwards terminal query replies to Go through the local-only JS interface.

## Real spectrum and audio permission

The rain can follow the TV's actual audio with Android's output-mix
`Visualizer` (session 0). Android calls the required runtime permission
`RECORD_AUDIO`: this feature reads the TV's output mix, **not a microphone**.
`MODIFY_AUDIO_SETTINGS` is also declared for the effect. The permission
prompt appears only when playback first requests spectrum levels, never
at launch. Declining leaves playback, sign-in, and the terminal working
normally, with decorative rain; it will not ask again during that run.
A permanent denial stays decorative silently.

To revoke access, open **Settings > Apps > nu11signal > Permissions** and
disable the audio/microphone permission. You can also grant it there later.
Unsupported devices or an effect held by another app fall back to decorative
rain without stopping playback. Capture is released while paused/stopped,
when the Activity pauses, and on exit; it resumes only while playing.

Waveform capture runs at the highest supported rate up to 20 Hz; readings
are sent at most about 15 times per second. Pure Java DSP converts unsigned
8-bit PCM to floats, applies a Hann window, and runs a radix-2 FFT. Its
one-sided power compensates the window's energy gain, then sums bin centres
into 24 logarithmic bands from 40 Hz to 16 kHz. Bands narrower than a bin
interpolate the two neighbouring bin powers at the band's geometric centre,
so low columns are not structurally empty (frequencies beyond Nyquist still
have no data). Power maps from -50 to -10 dBFS with fast attack and about a
one-second fall, plus 64 signed waveform points retaining each stretch's
largest swing.

`SCALING_MODE_NORMALIZED` is intentional: unlike `AS_PLAYED`, levels do not
follow the TV volume setting. Android's automatic normalization means dBFS
is relative to the captured PCM, not calibrated pre-volume source amplitude;
8-bit quantization and device capture limits still differ from macOS. This
is output-mix audio, so other TV sounds can affect the rain too.

## Remote and keyboard

| Key | TUI input |
| --- | --- |
| D-pad arrows | ANSI arrow sequences |
| D-pad center / Enter | Open keyboard when text input is available and hidden; otherwise carriage return |
| Long D-pad center | Toggle on-screen keyboard |
| Short Back | Escape; hides keyboard first if open |
| Long Back | Escape in SEARCH or NEW PLAYLIST name; quit confirmation elsewhere (non-printable Ctrl+Q); hides keyboard first if open |
| Play/pause, play, pause | Space |
| Next / fast-forward | `n` |
| Previous / rewind | `p` |
| Menu | Open/resume SEARCH at its input and open keyboard (non-printable Ctrl+_); no-op if keyboard already open |
| Delete/backspace | DEL |
| Hardware letters, digits, printable symbols | Unicode characters as UTF-8 |

Selecting the SEARCH input row or the NEW PLAYLIST name input makes text input
available without opening the keyboard. SEARCH result rows retain normal OK
selection. Menu on those rows focuses the input without inserting `/`, then
opens the keyboard after Go acknowledges input availability. Menu also returns
parked detail pages to SEARCH; desktop `/` and Tab keep their resume behavior.
Desktop `/` on a SEARCH result row also refocuses the input; on the input itself
it is text. Press OK / Enter to open it; that press does not submit the input. The keyboard hides when the input loses focus or that
mode ends (including blocking overlays). Long OK remains a manual toggle. While open, D-pad navigation belongs to the IME. Back only
hides the keyboard, leaving the TUI in place; a second Back sends Escape.
TUI input availability never opens the keyboard over Apple Music sign-in.

The invisible EditText forwards every edit immediately, including IME
composition, while retaining suggestions. Each edit keeps the common Unicode
prefix, sends DEL for each removed code point, then sends the new suffix as
UTF-8. Keyboard Enter/Send sends carriage return and silently clears local
text; hiding the keyboard or ending typing mode also resets it without DEL.
Every keyboard opening clears the local EditText and diff baseline without
sending DEL; the existing TUI search term is kept for appending.
Backspace on an empty field still sends DEL. Volume keys retain system behavior.

Each time sign-in starts, a full-screen native guide covers the music page
with large, high-contrast text explaining why Apple Music sign-in is needed
and how to use the pointer, Apple ID fields, and verification code. Press
OK / Enter to dismiss the guide and start; Back skips sign-in for now,
returns to the TUI, and notifies Go. While the guide is visible, arrows and
media keys are ignored and the pointer is hidden.

After the guide, a high-contrast ring pointer starts at screen center over
the music page (or its Apple ID child window). A hint bar shows the login
controls; both overlays disappear when returning to the TUI. When Go
confirms authorization by closing active sign-in, a centered "Signed in to
Apple Music" banner appears over the terminal for four seconds. Leaving
with Back does not show this confirmation.

After first sign-in, Go hides the sign-in view and navigates the music
WebView once to `https://music.apple.com/`. Apple then selects the account's
storefront instead of keeping the anonymous storefront (for example, `/us`
becomes `/co`). Go owns this handoff so authorization cannot reach the
composite player before the new document is ready. An old-document marker
also prevents an authorized status polled just before navigation from
prematurely completing the handoff. An already-signed-in launch does not
need this extra navigation.

The TUI remains usable during the refresh. Navigation failures such as
`page navigated` and MusicKit's `ready:false` keep Apple Music pending,
without transient library errors or a fixed readiness deadline; on a Google
TV Streamer this can take about 40 seconds. A bridge disconnect or canceled
poll still propagates normally. Once the new document is ready and
signed in, the composite emits a library-change notification. On every
platform, the radio then reloads playlists and retries failed favorite
reads without an app restart. Returning with Back does not itself trigger
navigation; if the account finishes signing in later, the same Go-owned
handoff runs when authorization is observed.

| Key during sign-in | Action |
| --- | --- |
| D-pad arrows | Move pointer; holding accelerates from 8 to 48 dp per repeat |
| Up/down at the screen edge | Scroll the page under the pointer |
| D-pad center / Enter | Tap at pointer; text fields open the TV keyboard |
| Long D-pad center / Enter | Ignored |
| Back | Go back in child window history, then close it; otherwise return to TUI and notify Go |
| Media keys | Ignored while the TUI is hidden |

With the soft keyboard visible, arrows and OK use normal IME/WebView
handling instead of moving or tapping the pointer. Back can dismiss the
keyboard normally. Visibility uses IME WindowInsets on API 30+ and a
visible-window-height fallback on API 28/29.

Taps dispatch touchscreen MotionEvents directly to the topmost WebView:
DOWN and UP share a downTime, with UP posted 50 ms later. Pointer positions
are converted from root coordinates to WebView-local coordinates using
their screen locations, not CSS pixels. Edge scrolling dispatches DOWN,
four MOVE events 30 ms apart, then UP, with finger motion opposite the
scroll direction. The drag starts inset from the vertical edge in the
pointer's column and travels 48 dp plus the current movement step (bounded
by the viewport). Native touch drags, rather than `scrollBy`, allow inner
DOM scroll containers to handle scrolling. Gestures do not overlap;
repeats during a gesture are skipped. Closing login or its child window
cancels any pending touch. No JavaScript clicks are used.

## Process and bridge

`src/dev/wahh/nu11signal/tv/MainActivity.java` starts the extracted executable
from `applicationInfo.nativeLibraryDir/libnu11signal.so` with
`--android-tv <filesDir>/run/bridge.sock`. Native extraction is mandatory;
the `.so` files are executable Go programs, not JNI libraries. The app
creates the run directory with mode 0700. Go creates its socket with mode
0600, listens once, and owns the PTY. The app retries socket connection for
up to 20 seconds. HOME and XDG_CONFIG_HOME use private `home` and `config`
directories; TMPDIR is the cache directory, TERM is xterm-256color, and
COLORTERM is truecolor. stderr is read separately under logcat tag
`nu11signal`. Startup failures are displayed as plain terminal text. While
Apple Music loads, Go draws the TUI's boot splash (emblem and BOOTING line)
from the first terminal frame and holds it until the web player is ready,
then hands the same Bubble Tea program over to the normal UI
(`radio.Startup`).

The filesystem LocalSocket carries UTF-8 JSON lines, not terminal data:

- Go sends `evaluate {id, expr}`, `login {show}`, `levels {on}`, `keyboard {show}`,
  or `nowplaying {title, artist, album, playing, position_ms, duration_ms}`.
  Levels enable output-mix capture only while playing; repeated states do not
  repeat the control.
  Keyboard `show:true` marks TUI text input available; `show:false` clears
  availability and dismisses an open TUI keyboard.
- The app evaluates expressions verbatim in an async wrapper in the music
  WebView. `NU11.settle(token, id, json)` checks a random per-launch token
  and returns `result {id, ok, value}` or `result {id, ok:false, error}`.
  Undefined results become JSON null; IDs preserve concurrent correlation.
- The app sends `resize {cols, rows}`, `login {closed:true}`, and
  `levels {bands, wave}` (integer percentages). Go bounds readings to 64 bands
  and 128 wave points, clamps their values, and keeps only the latest reading.
- Socket writes and stdin writes each use their own single-thread executor.
  The initial terminal size is retained until the socket connects.

Terminal navigation and resource requests are restricted to local Android
assets, with network loads blocked. Music WebViews keep the spike's JS,
DOM storage, third-party cookie, media autoplay, and real child-window
settings. Only protected media ID permission is granted. Behind the opaque
terminal, the music WebView stays attached and VISIBLE but uses a 1x1-pixel
layout at the top-left to avoid compositing Apple's animated full-screen
page without marking MusicKit hidden. Login mode, the sign-in guide, or an
open child window restores its full-screen layout; returning to the TUI
shrinks it again. Neither WebView pause nor pauseTimers is issued. The
terminal's layer type is unchanged to preserve its WebGL rendering path.
Requests for Apple's animated album art (silent HLS streams on
`mvod.itunes.apple.com` whose file name contains `_Anull_`, see
`MotionArtwork.java`) get an empty 403: on a cold start the home page
otherwise decoded about a dozen of them in the hidden page, keeping a
hardware decoder and the CPU busy for many seconds. Songs and anything with
an audio track are unaffected. `scripts/test-android-motion-artwork.sh` runs
its host-JVM tests.

## Background playback and system media controls

The first `nowplaying` message with `playing:true` starts `PlaybackService`,
a platform foreground mediaPlayback service. Its low-importance `playback`
channel has a MediaStyle notification with previous, play/pause and next
controls. A platform MediaSession publishes title, artist, album, duration and
position to the TV's now-playing surfaces. Media-session notifications are
exempt from the Android notification permission prompt; POST_NOTIFICATIONS is
declared but the app does not request it.

Home, another app, and ambient/screensaver mode do not pause MusicKit: the
music WebView remains attached, VISIBLE and 1x1 behind the terminal, and neither
WebView receives onPause or pauseTimers. While the Activity is stopped, stdout
continues to drain but terminal bytes are dropped; no new terminal JavaScript
work is posted. onStart sends a resize to redraw the TUI. Spectrum capture still
pauses when the Activity pauses, without affecting audio.

Go sends deduplicated `nowplaying {title, artist, album, playing, position_ms,
duration_ms}` anchors at most once per second. Ordinary advancing position ticks
are suppressed; song/status/duration changes and seeks update the anchor.
Strings are limited to 512 Unicode code points. MusicKit poll jitter up to
500 ms is ignored when detecting seeks.

The service uses the Activity's existing serialized stdin writer in the same
process (`space`, `n`, `p`), not another player. Explicit play/pause callbacks
check the reported playing state before toggling. While the Activity is resumed,
it consumes physical remote media keys and the session ignores media-button
events, avoiding double delivery; direct session/notification controls still
work. Sign-in ignores service input just like foreground remote input.

After five minutes paused the service removes its foreground notification,
but keeps the session available until exit; playback restores foreground mode.
The static bridge retains the Activity only for its lifetime and is cleared on
Activity destruction. The service is not sticky: it cannot recreate Go or the
WebViews after process death. A foreground service improves background survival,
but Android/device resource policies can still kill the process.

To stop playback and the service, return to nu11signal and press `q`, or long
Back outside a text input, then confirm quitting. A normal Go exit finishes the
Activity. Nonzero exits remain visible with an error. Destruction stops the
service, releases the MediaSession, closes socket/pipes and terminates Go.
Device verification and the end-user guide remain separate tasks.
