#!/usr/bin/env bash
# Test-only instrumentation; never installs the target APK or publishes artifacts.
set +x
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
[[ $# == 1 ]] || { echo 'usage: bash scripts/build-android-update-e2e.sh <signed-0.0.4.apk>' >&2; exit 2; }
SDK="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
TOOLS="$SDK/build-tools/${BUILD_TOOLS:-35.0.0}"
JAR="$SDK/platforms/android-34/android.jar"
PIN=448148925a1ecad2f51f3f5007343535d22d41716b4f0b9c4d88daaed0494320
mkdir -p "$ROOT/android/build"
WORK="$(mktemp -d "$ROOT/android/build/update-e2e.XXXXXXXX")"
mkdir -p "$WORK/assets" "$WORK/classes" "$WORK/dex"
cp "$1" "$WORK/assets/update.apk"
verify_pin() {
  local certs signer
  certs="$("$TOOLS/apksigner" verify --print-certs "$1")"
  signer="$(printf '%s\n' "$certs" | awk '/^Signer #[0-9]+ certificate SHA-256 digest:/ {print $NF}')"
  [[ "$signer" == "$PIN" ]] || { echo 'error: expected exactly the official release signer' >&2; exit 1; }
}
verify_pin "$WORK/assets/update.apk"
BADGING="$("$TOOLS/aapt2" dump badging "$WORK/assets/update.apk")"
[[ "$BADGING" == *"name='dev.wahh.nu11signal.tv' versionCode='4' versionName='0.0.4'"* && "$BADGING" != *application-debuggable* ]] || {
  echo 'error: fixture must be the non-debuggable signed 0.0.4 APK' >&2; exit 1;
}
"$TOOLS/aapt2" link -o "$WORK/test-res.apk" --manifest "$ROOT/android/test/update-e2e/AndroidManifest.xml" \
  -I "$JAR" -A "$WORK/assets" --min-sdk-version 28 --target-sdk-version 34
javac --release 11 -classpath "$JAR" -d "$WORK/classes" "$ROOT/android/test/update-e2e/UpdateE2eInstrumentation.java"
CLASSES=()
while IFS= read -r file; do CLASSES+=("$file"); done < <(find "$WORK/classes" -name '*.class' -type f)
"$TOOLS/d8" --lib "$JAR" --min-api 28 --output "$WORK/dex" "${CLASSES[@]}"
cp "$WORK/test-res.apk" "$WORK/unsigned.apk"
(cd "$WORK/dex" && zip -q "$WORK/unsigned.apk" classes.dex)
"$TOOLS/zipalign" -f -p 4 "$WORK/unsigned.apk" "$WORK/aligned.apk"
export NU11SIGNAL_TV_KEYSTORE="${NU11SIGNAL_TV_KEYSTORE:-$HOME/Library/Application Support/nu11signal/android/nu11signal-tv-release.keystore}"
export NU11SIGNAL_TV_KEY_ALIAS="${NU11SIGNAL_TV_KEY_ALIAS:-nu11signal-tv}"
if [[ -z "${NU11SIGNAL_TV_KEYSTORE_PASS:-}" ]]; then
  NU11SIGNAL_TV_KEYSTORE_PASS="$(security find-generic-password -a nu11signal-tv -s nu11signal-tv-keystore -w 2>/dev/null)" || {
    echo 'error: release password unavailable in Keychain' >&2; exit 1;
  }
fi
export NU11SIGNAL_TV_KEYSTORE_PASS
export NU11SIGNAL_TV_KEY_PASS="${NU11SIGNAL_TV_KEY_PASS:-$NU11SIGNAL_TV_KEYSTORE_PASS}"
"$TOOLS/apksigner" sign --ks "$NU11SIGNAL_TV_KEYSTORE" --ks-key-alias "$NU11SIGNAL_TV_KEY_ALIAS" \
  --ks-pass env:NU11SIGNAL_TV_KEYSTORE_PASS --key-pass env:NU11SIGNAL_TV_KEY_PASS \
  --out "$WORK/nu11signal-update-test.apk" "$WORK/aligned.apk"
verify_pin "$WORK/nu11signal-update-test.apk"
printf 'Verified test APK (do not publish): %s\n' "$WORK/nu11signal-update-test.apk"
