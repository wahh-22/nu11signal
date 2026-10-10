# Android TV spike

## Objective
Find out whether Apple Music can play on Android TV from our own APK without a
developer token (user goal 2026-10-10: listen to Apple Music on the TV).

## Design
- The official MusicKit for Android SDK needs a developer token and deeplinks
  into the Apple Music app, which has no Android TV Play Store build, so the
  spike mirrors the Linux web player instead: a WebView loads music.apple.com
  and the page's own MusicKit is driven from outside.
- `spike/android-tv-webview/`: one plain Java Activity (no AndroidX, no
  Gradle) built with the SDK tools directly (aapt2, javac, d8, zipalign,
  apksigner with a local debug key). Leanback launcher entry, D-pad friendly.
- WebView: JavaScript, DOM storage, cookies (third-party too), media playback
  without a user gesture, `RESOURCE_PROTECTED_MEDIA_ID` granted for Widevine,
  remote debugging on, so the Mac drives it over CDP
  (`adb forward tcp:9222 localabstract:webview_devtools_remote_<pid>`).
- Device: Google TV Streamer `sala-de-estar` over Tailscale
  (`adb connect 100.117.131.97:5555`), Android 14 (API 34), armeabi-v7a,
  WebView 153.
- Sign-in is done by the user (on the TV or through chrome://inspect); the
  agent never handles the Apple ID password.
- Verification is functional on the device (no deterministic test applies to
  a feasibility spike).

## Tasks
- [x] S1 — Minimal Android TV WebView APK + no-Gradle build script.
  Route: delegated writer (6 new files). `./build.sh` passed (platform 34,
  build-tools 35.0.0); `apksigner verify` ok; badging shows launchable and
  leanback-launchable activity, min 28 / target 34. Installed on the TV
  (`adb install -r`: Success), launched, WebView devtools socket present.
  Parent added `cdp.mjs` (one-expression CDP evaluator, `--reload`).
- [ ] S2 — Install on the TV; verify launch, page load, and Widevine EME
  (`requestMediaKeySystemAccess('com.widevine.alpha')`) over CDP.
- [ ] S3 — User signs in; verify page MusicKit authorization and play one
  song (`playbackState` playing, audio heard on the TV).
- [ ] S4 — Record the go/no-go findings and the next step.

## Progress
- 2026-10-10: first load failed with `ERR_NAME_NOT_RESOLVED`; cause was Doze
  (`blocked=DOZE` for uid 10052, TV asleep), not DNS. Added the app to the
  Doze whitelist (`dumpsys deviceidle whitelist +dev.wahh.nu11signal.tvspike`;
  remove with `-` after the spike). With network back, the page stays
  `readyState=loading`, `visibilityState=hidden` while `mWakefulness=Asleep`:
  the TV must be awake to continue.
- Created 2026-10-10 on branch `spike/android-tv-webview` from main 8e99dd3.
