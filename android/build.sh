#!/usr/bin/env bash
# Builds and signs the platform-only Android TV app and bundled Go executables.
# Output: build/nu11signal-tv-<version>.apk. Optional argument: install.
# DEBUG=1 marks the app debuggable, which also turns on WebView remote
# debugging (chrome://inspect); never ship such a build.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
SDK="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
NDK="${ANDROID_NDK:-$SDK/ndk/28.2.13676358}"
TOOLS="$SDK/build-tools/${BUILD_TOOLS:-35.0.0}"
ANDROID_JAR="$SDK/platforms/${ANDROID_PLATFORM:-android-34}/android.jar"
VERSION="${VERSION:-dev}"
DEBUG_FLAGS=()
[[ "${DEBUG:-0}" == 1 ]] && DEBUG_FLAGS=(--debug-mode)
[[ "$VERSION" =~ ^[A-Za-z0-9][A-Za-z0-9._+-]*$ ]] || { echo 'Invalid VERSION' >&2; exit 1; }
[[ $# -le 1 && ( $# -eq 0 || "$1" == install ) ]] || { echo "Usage: $0 [install]" >&2; exit 1; }
[[ "${1:-}" != install || -n "${ANDROID_SERIAL:-}" ]] || { echo 'Set ANDROID_SERIAL before install.' >&2; exit 1; }
case "$(uname -s)" in
  Darwin) HOST=darwin-x86_64 ;;
  Linux) HOST=linux-x86_64 ;;
  *) echo 'Unsupported NDK host' >&2; exit 1 ;;
esac
CLANG="$NDK/toolchains/llvm/prebuilt/$HOST/bin"
[[ -f "$ANDROID_JAR" && -x "$TOOLS/aapt2" && -x "$CLANG/clang" ]] || { echo 'Missing Android SDK/NDK tools' >&2; exit 1; }
[[ -f "$ROOT/assets/xterm/xterm.js" && -f "$ROOT/assets/xterm/addon-fit.js" && -f "$ROOT/assets/xterm/addon-webgl.js" ]] || { echo 'Missing xterm assets: run android/vendor-xterm.sh first.' >&2; exit 1; }
[[ -f "$ROOT/assets/fonts/KodeMono.ttf" && -f "$ROOT/assets/fonts/OFL.txt" ]] || { echo 'Missing Kode Mono font/license: run android/vendor-font.sh first.' >&2; exit 1; }
OUT="$ROOT/build"
APK="$OUT/nu11signal-tv-$VERSION.apk"
mkdir -p "$OUT/lib/armeabi-v7a" "$OUT/lib/arm64-v8a"
# Isolate each Java build from stale classes without deleting previous outputs.
WORK="$(mktemp -d "$OUT/apk.XXXXXXXX")"
mkdir -p "$WORK/generated" "$WORK/classes" "$WORK/dex"
export GOCACHE="$OUT/go-cache" GOTMPDIR="$OUT/go-tmp"
mkdir -p "$GOCACHE" "$GOTMPDIR"
cd "$REPO"
for arch in arm arm64; do
  if [[ "$arch" == arm ]]; then TARGET=armv7a-linux-androideabi28; ABI=armeabi-v7a; else TARGET=aarch64-linux-android28; ABI=arm64-v8a; fi
  echo "Building Go: android/$arch ($ABI)"
  CGO_ENABLED=1 GOOS=android GOARCH="$arch" GOARM=7 CC="$CLANG/$TARGET-clang" CXX="$CLANG/$TARGET-clang++" \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION -X main.versionStamp=nu11signal-version:$VERSION;" \
    -o "$OUT/lib/$ABI/libnu11signal.so" ./cmd/nu11signal
 done
"$TOOLS/aapt2" compile --dir "$ROOT/res" -o "$WORK/resources.zip"
"$TOOLS/aapt2" link -o "$WORK/resources.apk" --manifest "$ROOT/AndroidManifest.xml" -I "$ANDROID_JAR" \
  --min-sdk-version 28 --target-sdk-version 34 --version-name "$VERSION" ${DEBUG_FLAGS[@]+"${DEBUG_FLAGS[@]}"} \
  --java "$WORK/generated" "$WORK/resources.zip"
javac --release 11 -classpath "$ANDROID_JAR" -d "$WORK/classes" \
  "$ROOT/src/dev/wahh/nu11signal/tv/MainActivity.java" "$WORK/generated/dev/wahh/nu11signal/tv/R.java"
CLASSES=()
while IFS= read -r file; do CLASSES+=("$file"); done < <(find "$WORK/classes" -name '*.class' -type f)
"$TOOLS/d8" --lib "$ANDROID_JAR" --min-api 28 --output "$WORK/dex" "${CLASSES[@]}"
cp "$WORK/resources.apk" "$WORK/unsigned.apk"
(cd "$WORK/dex" && zip -q "$WORK/unsigned.apk" classes.dex)
(cd "$OUT" && zip -q -0 -r "$WORK/unsigned.apk" lib)
(cd "$ROOT" && zip -q -r "$WORK/unsigned.apk" assets)
"$TOOLS/zipalign" -f -p 4 "$WORK/unsigned.apk" "$WORK/aligned.apk"
if [[ -n "${NU11SIGNAL_TV_KEYSTORE:-}${NU11SIGNAL_TV_KEY_ALIAS:-}${NU11SIGNAL_TV_KEYSTORE_PASS:-}${NU11SIGNAL_TV_KEY_PASS:-}" ]]; then
  : "${NU11SIGNAL_TV_KEYSTORE:?Set keystore}" "${NU11SIGNAL_TV_KEY_ALIAS:?Set alias}" "${NU11SIGNAL_TV_KEYSTORE_PASS:?Set password}"
  export NU11SIGNAL_TV_KEY_PASS="${NU11SIGNAL_TV_KEY_PASS:-$NU11SIGNAL_TV_KEYSTORE_PASS}"
  "$TOOLS/apksigner" sign --ks "$NU11SIGNAL_TV_KEYSTORE" --ks-key-alias "$NU11SIGNAL_TV_KEY_ALIAS" \
    --ks-pass env:NU11SIGNAL_TV_KEYSTORE_PASS --key-pass env:NU11SIGNAL_TV_KEY_PASS --out "$APK" "$WORK/aligned.apk"
else
  if [[ ! -f "$OUT/debug.keystore" ]]; then
    keytool -genkeypair -keystore "$OUT/debug.keystore" -storepass android -keypass android \
      -alias androiddebugkey -keyalg RSA -keysize 2048 -validity 10000 -dname 'CN=Android Debug,O=Android,C=US' -noprompt
  fi
  "$TOOLS/apksigner" sign --ks "$OUT/debug.keystore" --ks-key-alias androiddebugkey \
    --ks-pass pass:android --key-pass pass:android --out "$APK" "$WORK/aligned.apk"
fi
"$TOOLS/apksigner" verify "$APK"
if [[ "${1:-}" == install ]]; then "$SDK/platform-tools/adb" -s "$ANDROID_SERIAL" install -r "$APK"; fi
printf '%s\n' "$APK"
