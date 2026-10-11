# Android TV

## Objective
Run nu11signal itself (the same Bubble Tea TUI) on Android TV as an APK and
play Apple Music there, with every step the user takes documented (user
choice 2026-10-10, after the go outcome of `odd/tasks/android-tv-spike.md`).

## Design
- One APK, two parts:
  - **Go:** the real `nu11signal` binary, cross-compiled with the NDK for
    `android/arm` (armeabi-v7a, the Google TV Streamer's userspace) and
    `android/arm64`, shipped as `lib/<abi>/libnu11signal.so` (an executable
    in the native library dir, the only place an app may exec from) and
    started by the app with `--android-tv <socket>`.
  - **Java (no Gradle, no AndroidX, as the spike):** one Activity holding a
    full-screen terminal WebView (vendored xterm.js, local asset, no network)
    over a hidden music WebView on music.apple.com.
- **Terminal:** in `--android-tv` mode Go opens a PTY itself (`/dev/ptmx`,
  proven on the TV from adb), runs the TUI on the slave, and relays the
  master to its stdin/stdout, which the app pipes to xterm.js. Env set by the
  app: `HOME` and `XDG_CONFIG_HOME` in app-private storage, `TERM=xterm-256color`,
  `COLORTERM=truecolor`. Resize goes through the bridge (TIOCSWINSZ).
- **Bridge:** a Unix socket Go listens on in the app's private dir (0700),
  JSON lines. Go -> app: `evaluate {id, expr}`, `login {show}`; app -> Go:
  `result {id, ok, value|error}`, `resize {cols, rows}`. The app runs each
  expression in the music WebView inside an async wrapper and reports the
  settled promise back through a `@JavascriptInterface` (correlated by id,
  concurrent, errors short and sanitized like `ScriptError`).
- **Go reuse:** `webplayer.New(page Evaluator, ...)` already takes the one
  method we need (`Evaluate(ctx, expr) (json.RawMessage, error)`), so the
  Player, page script, catalog and library are reused unchanged. New: an
  Evaluator over the bridge, an android branch in `openPlayer` before the
  unsupported-OS fallback, and an Android login path (show the music
  WebView, wait for `isAuthorized`).
- **Sign-in on the TV:** the page's Sign In needs a real user gesture, so in
  login mode the app shows the music WebView with a D-pad-driven pointer
  (arrows move it, OK taps with real MotionEvents); text fields open the TV
  keyboard. Back leaves login mode.
- **Remote keys:** arrows -> arrow keys, OK -> enter, Back -> esc (long Back
  -> q, the quit confirmation), play/pause -> space, next/previous -> n/p,
  menu -> `/` (SEARCH). Typing uses the TV keyboard (or the Google TV phone
  app's keyboard), forwarded as text.
- **Background:** a foreground media service with a MediaSession keeps
  playback alive with the app in the background and takes the remote's media
  keys.
- **Non-goals (v1):** local files on the TV (oto/Oboe compiles but scoped
  storage needs a separate design), Play Store distribution, a native TV UI.
- **Docs:** `docs/android-tv.md` is the user guide: requirements, install
  (sideload), first launch, sign-in step by step, remote key map, typing,
  background playback, updating, troubleshooting, uninstalling. Building the
  APK goes in `docs/releasing.md` / developer docs.

## Tasks
- [x] A1 — Go: `--android-tv` mode (PTY relay, bridge socket, Evaluator over
  the bridge, android branch in `openPlayer`, Android login) + tests.
  Route: delegated writer. `webplayer` bridge Evaluator (JSON lines,
  correlated concurrent ids, ctx cancel, sanitized ScriptError,
  ErrBrowserGone on close, bounded lines); `internal/androidtv` (private
  socket, single timed accept, PTY + relay + resize/SIGWINCH, sign-in
  coordination, Android hint wrapper); `--android-tv` flag (Android only,
  exit 2 elsewhere), TUI on the PTY slave; plain Android stays local-only;
  `--apple-music-login` on Android points to the app. RED observed
  (`TestAndroidTVFlag`, `TestAndroidTVBackendSelection`,
  `TestAndroidLoginPointsToApp`, compile failures for new APIs). Verifier:
  `go build`, `go vet` ok; `go test -race ./...` 1511 passed in 14 packages;
  NDK builds android/arm and android/arm64 ok. PTY tests skip on macOS.
  Commit `0ddea38`. Review `review-4df5c9c081dee75c` (high, 4 lenses,
  cumulative base-diff from main) APPROVED, acknowledged. Advisories (not
  scheduled yet): the stale-socket liveness probe in runtime.go:83-86 dials
  the socket (consumes a live listener's accept); socket tests on Windows.
- [x] A2 — App shell: Activity with xterm.js terminal and hidden music
  WebView, process launch with env, bridge client, resize, remote key map;
  build script producing the APK with the Go binaries.
  Route: delegated writer (android/), then device-driven fixes. Vendored
  xterm 6.0.0 + addon-fit 0.11.0 (sha512 verified). `DEBUG=1 build.sh` makes
  a debuggable APK (WebView remote debugging). Device findings fixed:
  (1) evaluations pending across a music page navigation never settled ->
  failed on `onPageStarted`; (2) MusicKit ready ~41 s after launch on the
  Streamer (page JS saturated, answers 1-20 s) -> `BridgeLoadTimeout` 2 min
  for Android readiness/startup (RED: TestBridgeReadinessBound,
  TestAndroidTVBackendSelection) + loading line in the terminal; (3) raw
  Android player showed AUTH // ERROR -> composite-wrapped like Linux with a
  private empty `$HOME/Music`, sign-in keeps its own player reference and
  bounds polls to 5 s (RED: TestAndroidTVBackendSelection,
  TestSignInPollTimeoutStillShowsLogin). Verified on the TV: Go stays up,
  the TUI draws (read through CDP: header, PLAYLISTS, NOW PLAYING), and when
  unauthorized the music WebView is VISIBLE+focused with the terminal GONE
  (`dumpsys activity top`). `go test -race ./...` 1517 passed. Note:
  `adb screencap` shows this app black; inspect through CDP instead.
  Commits `3c8ecef` (vendored xterm, third-party minified: not reviewed) and
  `f61e536` (app). The cumulative branch review hit
  `lens_context_budget_exceeded` (vendored xterm), so the app commit was
  reviewed against `3c8ecef`: `review-5d726ca856fdd7a3` (high, 4 lenses,
  1006 lines) APPROVED, acknowledged. Advisories (later): a fatal Go exit
  can stay hidden behind the login view (MainActivity.java:286), IME
  composition text can be lost (108-121), navigation-failed evaluations are
  not retried by the app (166; Go re-polls).
- [ ] A3 — Sign-in mode: D-pad pointer over the music WebView, driven by the
  bridge `login` message.
- [ ] A2b — Desktop fidelity (user feedback 2026-10-10: plays and reads
  well, but): Kode Mono font bundled (OFL), xterm WebGL renderer so the
  rain's block/braille glyphs match a desktop terminal, and a terminal grid
  sized like the user's desktop terminal so NOW PLAYING gets the same share
  (list panel caps at 72 columns, `listPanelWidthFor`; 120 TV columns leave
  the player 47).
- [x] A8 — Spike: real spectrum for the rain on Android (desktop has it only
  on macOS through the Core Audio tap; Linux is decorative). Try
  `android.media.audiofx.Visualizer` on the output mix with Widevine audio;
  needs RECORD_AUDIO. Go/no-go before any build-out.
  Outcome GO (`spike/android-tv-visualizer`, a separate APK reading
  `Visualizer(0)`, RECORD_AUDIO granted with `pm grant`): control with the
  unencrypted 30 s preview in an `<audio>` element gave swing 5-21 / FFT
  peak 7-35; the Widevine track ("Sharks", full track, playing) gave
  non-zero, varying swing 1-5 / FFT peak 1-11 (quiet outro, AS_PLAYED
  scaling). Paused: all zeros. Build-out (A9) should use
  SCALING_MODE_NORMALIZED and needs the user's RECORD_AUDIO consent (it
  hears the whole TV output mix).
- [ ] A9 — Real rain spectrum on Android: Visualizer FFT in the app -> bridge
  `levels` messages -> a Go LevelSource (bands + wave like docs/audio.md) ->
  composite -> radio.
- [ ] A3c — Open the TV keyboard automatically when the TUI enters a text
  input (SEARCH, NEW PLAYLIST name) and hide it when it leaves; today only a
  long OK opens it (user could not find it).
- [ ] A4 — Foreground media service + MediaSession (background playback,
  media keys).
- [ ] A5 — Build and release tooling: `make android`, signing with the
  user's own key, artifacts.
- [ ] A6 — User guide `docs/android-tv.md` + README/install links +
  developer build docs.
- [ ] A7 — End-to-end verification on the TV (install, sign in with the
  remote, search, play, background, relaunch keeps the session).

## Progress
- 2026-10-11: A3c auto keyboard `b87d024` (review `review-faf7d69608cc4315`
  approved; verified `mInputShown` on SEARCH). User feedback: open the
  keyboard on OK over the input, not on entering SEARCH (A3d); play with
  nothing queued animates the rain while MusicKit reports state 0 and an
  empty queue (bug, A3d); user approved RECORD_AUDIO for A9 (decline keeps
  the decorative rain).
- 2026-10-10: A3b guide `96c7b66` (review `review-dfe0f1a4305c8bd7` approved).
  User signed in with the remote only; verified `isAuthorized: true` and
  persistent `media-user-token`; session survives an app restart (6 library
  playlists, storefront `co`). First-session defects (empty library, page on
  storefront `us`) fixed in `17a975d` (RED: TestLateAuthorizationReloadsLibrary,
  TestLibraryChangeRetriesFailedFavorites,
  TestLateAuthorizationWaitsForAccountDocument; 1524 tests pass; review
  `review-9573a2b326cfbfe3` approved). Device check of that fix needs a
  fresh sign-in (pending the user's go-ahead).
- 2026-10-10: first remote sign-in attempt went to the spike app (both
  apps were installed and both showed in chrome://inspect); the real app's
  cookie DB held only `geo`. Spike uninstalled from the TV and removed from
  the Doze whitelist (user approved). Follow-up A3b: a clear on-screen
  sign-in guide and a signed-in confirmation; the user guide documents only
  the remote-control path.
- Created 2026-10-10 on branch `feat/android-tv` from `spike/android-tv-webview` b595d9e.
- Probe: `nu11signal` builds for `android/arm` with the NDK 28.2 clang
  (`CGO_ENABLED=1`, 17.7 MB) and `--version` runs on the TV; a Go probe
  opened `/dev/ptmx` + `/dev/pts/0` there (as the adb shell user).
