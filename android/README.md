# Android TV app shell

This APK runs the real Go/Bubble Tea TUI on a Go-owned PTY. A local xterm.js
WebView covers a music.apple.com WebView, which owns MusicKit playback and
its Apple ID child sign-in window. It is a platform-only Java app: no Gradle,
AndroidX, or npm step during APK builds.

## Build

From the repository root:

```sh
android/build.sh
VERSION=0.5.0 android/build.sh
```

Requires Go, JDK 11 or newer, Android SDK platform 34, build-tools 35.0.0,
NDK 28.2.13676358, and zip. Both armeabi-v7a and arm64-v8a executables are
built with CGO and API 28 clang. Version stamping matches the release scripts:
`main.version` and `main.versionStamp=nu11signal-version:<version>;`.

| Environment | Default / purpose |
| --- | --- |
| `VERSION` | `dev`; APK name and binary version |
| `ANDROID_HOME` | `~/Library/Android/sdk` |
| `ANDROID_NDK` | `$ANDROID_HOME/ndk/28.2.13676358` |
| `BUILD_TOOLS` | `35.0.0` |
| `ANDROID_PLATFORM` | `android-34` (manifest remains target 34) |
| `NU11SIGNAL_TV_KEYSTORE` | Custom signing keystore path |
| `NU11SIGNAL_TV_KEY_ALIAS` | Custom key alias |
| `NU11SIGNAL_TV_KEYSTORE_PASS` | Custom keystore password |
| `NU11SIGNAL_TV_KEY_PASS` | Optional key password, defaults to store password |
| `ANDROID_SERIAL` | Required only for optional install |

Set all three required signing variables together. Otherwise the build
creates and reuses `android/build/debug.keystore`. Passwords are passed to
apksigner through environment variables, not command-line values. A debug
signing key does not make the application debuggable: WebView inspection is
only enabled when Android's application debuggable flag is present.

Output: `android/build/nu11signal-tv-<version>.apk`. Build outputs, caches,
verified vendor downloads, and signing material are ignored. Each build
uses a fresh Java packaging directory to avoid stale class files.

Optional installation (never part of the default build):

```sh
ANDROID_SERIAL=<serial> android/build.sh install
```

## Assets and terminal

`android/vendor-xterm.sh` downloads pinned xterm 6.0.0 and addon-fit 0.11.0
archives and verifies their SHA-512 digests before copying UMD assets and
MIT licenses. The exact URLs and integrity values are in
`assets/xterm/README.md`. Commit these assets; subsequent builds use local
files only (Go dependencies must already be cached for fully offline builds).

The terminal has a black background and truecolor support. Its proportional
monospace font is about 26 CSS px at 1920 wide, with line height 1.2, aiming
for roughly 120x34 cells at 1080p. Density-scaled viewports use the same
proportion, with a 12px floor. FitAddon reports the actual cell count on load
and window resize; the full TUI needs at least 60x16. stdout is batched for
up to 16 ms or 32 KiB, then passed as base64-decoded bytes to xterm. Raw
Android keys never reach xterm's own input handler. xterm's `onData` still
forwards terminal query replies to Go through the local-only JS interface.

## Remote and keyboard

| Key | TUI input |
| --- | --- |
| D-pad arrows | ANSI arrow sequences |
| D-pad center / Enter | Carriage return |
| Long D-pad center | Toggle on-screen keyboard |
| Short Back | Escape; hides keyboard first if open |
| Long Back | `q` (TUI quit confirmation); hides keyboard first if open |
| Play/pause, play, pause | Space |
| Next / fast-forward | `n` |
| Previous / rewind | `p` |
| Menu | `/` (search) |
| Delete/backspace | DEL |
| Hardware letters, digits, printable symbols | Unicode characters as UTF-8 |

The invisible EditText receives IME commits; each Unicode code point is
serialized to Go stdin. Keyboard Enter/Send sends carriage return. Volume
keys retain system behavior.

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
`nu11signal`. Startup failures are displayed as plain terminal text.

The filesystem LocalSocket carries UTF-8 JSON lines, not terminal data:

- Go sends `evaluate {id, expr}` or `login {show}`.
- The app evaluates expressions verbatim in an async wrapper in the music
  WebView. `NU11.settle(token, id, json)` checks a random per-launch token
  and returns `result {id, ok, value}` or `result {id, ok:false, error}`.
  Undefined results become JSON null; IDs preserve concurrent correlation.
- The app sends `resize {cols, rows}` and `login {closed:true}`.
- Socket writes and stdin writes each use their own single-thread executor.
  The initial terminal size is retained until the socket connects.

Terminal navigation and resource requests are restricted to local Android
assets, with network loads blocked. Music WebViews keep the spike's JS,
DOM storage, third-party cookie, media autoplay, and real child-window
settings. Only protected media ID permission is granted. Music continues
running behind the opaque terminal; no WebView pause is issued.

A normal Go exit finishes the Activity. Nonzero exits remain visible with
an error. Destruction closes the socket/pipes and terminates Go. Foreground
service and MediaSession lifecycle support are deferred to A4; background
playback is not guaranteed yet. Device verification and the end-user guide
are separate tasks, not part of this shell.
