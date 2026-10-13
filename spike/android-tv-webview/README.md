# Android TV WebView feasibility spike

A platform-only, full-screen WebView for testing Apple Music / Widevine playback on a Google TV Streamer (Android 14 / API 34, armeabi-v7a, WebView 153). Playback feasibility is unverified until tested on the TV. The Mac can drive the page through Chrome DevTools Protocol (CDP).

## Build

Requires JDK 17, Android SDK platform 34, build-tools 35.0.0, and `zip`; no Gradle or dependencies.

```bash
cd spike/android-tv-webview
./build.sh
```

Output: `build/nu11signal-tv-spike.apk`. Override the SDK path with `ANDROID_HOME` (default `~/Library/Android/sdk`) or the build-tools version with `BUILD_TOOLS` (for example, `34.0.0`). The script generates a local debug keystore in the ignored `build/` directory on first run. This is a debug-signed testing app, not a production build.

## Install and launch

Enable network ADB on the TV first. With SDK `platform-tools` on your PATH:

```bash
adb connect <tv>:5555
ANDROID_SERIAL=<tv>:5555 ./build.sh install
adb -s <tv>:5555 shell am start -n dev.wahh.nu11signal.tvspike/.MainActivity
```

Optional launch extras: `--es ua '<user agent>'` and `--es url 'https://music.apple.com/'`. Otherwise the WebView uses its default user agent and loads `https://music.apple.com/`. Cleartext traffic and mixed content are disabled.

## Remote debugging

Select the TV for the commands below (or add `-s <tv>:5555`):

```bash
export ANDROID_SERIAL=<tv>:5555
adb shell cat /proc/net/unix | grep webview_devtools_remote
adb forward tcp:9222 localabstract:webview_devtools_remote_<pid>
curl localhost:9222/json
```

Replace `<pid>` with the suffix from the socket listing. Open `chrome://inspect` on the Mac, configure `localhost:9222` if needed, and inspect the page; CDP clients can use the WebSocket URL from `/json`. Debugging is always enabled in this spike: use only on a trusted development device/network.

Only protected-media-ID permission requests are granted; camera, microphone, and other requested resources are denied. Console messages and main-frame errors use logcat tag `Nu11TvSpike`. D-pad focus and Back navigation are enabled. The screen stays on; the app deliberately does not pause its WebView when backgrounded, but Android may still suspend or terminate it.
