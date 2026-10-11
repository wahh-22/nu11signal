#!/usr/bin/env bash
# Builds and debug-signs the platform-only Android TV Visualizer spike.
# Output: ./build/nu11signal-viz-spike.apk. Optional argument: install.
set -euo pipefail
SPIKE="$(cd "$(dirname "$0")" && pwd)"
SDK="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
VERSION="${BUILD_TOOLS:-35.0.0}"
TOOLS="$SDK/build-tools/$VERSION"
ANDROID_JAR="$SDK/platforms/android-34/android.jar"
OUT="$SPIKE/build"
APK="$OUT/nu11signal-viz-spike.apk"

if [[ $# -gt 1 || ( $# -eq 1 && "$1" != install ) ]]; then
  echo "Usage: $0 [install]" >&2
  exit 1
fi
if [[ "${1:-}" == install && -z "${ANDROID_SERIAL:-}" ]]; then
  echo 'Set ANDROID_SERIAL before using install.' >&2
  exit 1
fi
if [[ ! -f "$ANDROID_JAR" || ! -x "$TOOLS/aapt2" ]]; then
  echo "Missing SDK platform or build tools: $ANDROID_JAR, $TOOLS" >&2
  exit 1
fi
mkdir -p "$OUT/generated" "$OUT/classes" "$OUT/dex"

"$TOOLS/aapt2" link -o "$OUT/resources.apk" \
  --manifest "$SPIKE/AndroidManifest.xml" -I "$ANDROID_JAR" \
  --min-sdk-version 28 --target-sdk-version 34 \
  --java "$OUT/generated"
javac --release 8 -classpath "$ANDROID_JAR" -d "$OUT/classes" \
  "$SPIKE/src/dev/wahh/nu11signal/vizspike/MainActivity.java" \
  
CLASSES=()
while IFS= read -r file; do
  CLASSES+=("$file")
done < <(find "$OUT/classes" -name '*.class' -type f)
"$TOOLS/d8" --lib "$ANDROID_JAR" --min-api 28 --output "$OUT/dex" "${CLASSES[@]}"
cp "$OUT/resources.apk" "$OUT/unsigned.apk"
(cd "$OUT/dex" && zip -q "$OUT/unsigned.apk" classes.dex)
"$TOOLS/zipalign" -f -p 4 "$OUT/unsigned.apk" "$OUT/aligned.apk"
if [[ ! -f "$OUT/debug.keystore" ]]; then
  keytool -genkeypair -keystore "$OUT/debug.keystore" -storepass android \
    -keypass android -alias androiddebugkey -keyalg RSA -keysize 2048 \
    -validity 10000 -dname 'CN=Android Debug,O=Android,C=US' -noprompt
fi
"$TOOLS/apksigner" sign --ks "$OUT/debug.keystore" --ks-key-alias androiddebugkey \
  --ks-pass pass:android --key-pass pass:android --out "$APK" "$OUT/aligned.apk"
"$TOOLS/apksigner" verify "$APK"
if [[ "${1:-}" == install ]]; then
  "$SDK/platform-tools/adb" install -r "$APK"
fi
printf '%s\n' "$APK"
