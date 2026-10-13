#!/usr/bin/env bash
# Builds and verifies a release-key-signed, non-debuggable Android TV APK.
# Usage: make android-release VERSION=x.y.z [DRY_RUN=1] [FORCE=1]
# Dry runs build/verify in android/build only; releases add an APK and checksum
# to dist/vVERSION without replacing desktop artifacts. Never trace passwords.
set +x
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
die() { echo "error: $*" >&2; exit 1; }
VERSION="${VERSION:-}"
[[ -n "$VERSION" ]] || die 'usage: make android-release VERSION=x.y.z [DRY_RUN=1] [FORCE=1]'
# SemVer 2.0: core numbers and numeric prerelease identifiers have no leading zero.
NUMBER='(0|[1-9][0-9]*)'
IDENT='[0-9A-Za-z-]+'
[[ "$VERSION" =~ ^$NUMBER\.$NUMBER\.$NUMBER(-$IDENT(\.$IDENT)*)?(\+$IDENT(\.$IDENT)*)?$ ]] ||
  die 'VERSION must be semver (x.y.z, optionally with prerelease/build metadata)'
PRE="${VERSION#*-}"
if [[ "$PRE" != "$VERSION" && "${VERSION%%+*}" == *-* ]]; then
  PRE="${PRE%%+*}"
  IFS=. read -r -a IDS <<<"$PRE"
  for id in "${IDS[@]}"; do
    [[ ! "$id" =~ ^0[0-9]+$ ]] || die 'VERSION must be semver (numeric prerelease has a leading zero)'
  done
fi
NAME="nu11signal-tv-$VERSION.apk"
APK="$ROOT/android/build/$NAME"
DEST="$ROOT/dist/v$VERSION"
refuse_overwrite() {
  if [[ "${FORCE:-0}" != 1 && ( -e "$DEST/$NAME" || -L "$DEST/$NAME" || -e "$DEST/$NAME.sha256" || -L "$DEST/$NAME.sha256" ) ]]; then
    die "$DEST/$NAME already exists; use FORCE=1 to replace the Android artifacts"
  fi
}
[[ "${DRY_RUN:-0}" == 1 ]] || refuse_overwrite
export NU11SIGNAL_TV_KEYSTORE="${NU11SIGNAL_TV_KEYSTORE:-$HOME/Library/Application Support/nu11signal/android/nu11signal-tv-release.keystore}"
export NU11SIGNAL_TV_KEY_ALIAS="${NU11SIGNAL_TV_KEY_ALIAS:-nu11signal-tv}"
[[ -f "$NU11SIGNAL_TV_KEYSTORE" ]] || die "release keystore missing: $NU11SIGNAL_TV_KEYSTORE; restore it from your backup, or set NU11SIGNAL_TV_KEYSTORE (see docs/releasing.md)"
if [[ -z "${NU11SIGNAL_TV_KEYSTORE_PASS:-}" ]]; then
  command -v security >/dev/null 2>&1 || die 'macOS security tool unavailable; supply NU11SIGNAL_TV_KEYSTORE_PASS through the CI secret environment'
  NU11SIGNAL_TV_KEYSTORE_PASS="$(security find-generic-password -a nu11signal-tv -s nu11signal-tv-keystore -w 2>/dev/null)" ||
    die 'keychain item missing or inaccessible: account nu11signal-tv, service nu11signal-tv-keystore; restore its password using Keychain Access (see docs/releasing.md)'
  [[ -n "$NU11SIGNAL_TV_KEYSTORE_PASS" ]] || die 'keychain item nu11signal-tv-keystore has an empty password; restore it using Keychain Access'
fi
export NU11SIGNAL_TV_KEYSTORE_PASS
SDK="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
TOOLS="$SDK/build-tools/${BUILD_TOOLS:-35.0.0}"
[[ -x "$TOOLS/apksigner" && -x "$TOOLS/aapt2" ]] || die "missing apksigner/aapt2 in $TOOLS"
EXPECTED="${NU11SIGNAL_TV_CERT_SHA256:-44:81:48:92:5A:1E:CA:D2:F5:1F:3F:50:07:34:35:35:D2:2D:41:71:6B:4F:0B:9C:4D:88:DA:AE:D0:49:43:20}"
normalize() { printf '%s' "$1" | tr -d ':' | tr '[:upper:]' '[:lower:]'; }
EXPECTED="$(normalize "$EXPECTED")"
[[ "$EXPECTED" =~ ^[0-9a-f]{64}$ ]] || die 'NU11SIGNAL_TV_CERT_SHA256 must be a SHA-256 fingerprint (hex, optionally colon-separated)'
# Override any inherited DEBUG, even in a dry run: these builds use the release key.
VERSION="$VERSION" DEBUG=0 bash "$ROOT/android/build.sh"
# Snapshot the build output into a private directory and verify only the
# snapshot: a later build of the same VERSION can replace android/build at any
# time, and what is published must be exactly the bytes that were verified.
if [[ "${DRY_RUN:-0}" == 1 ]]; then
  STAGE="$(mktemp -d "${TMPDIR:-/tmp}/nu11signal-android-release.XXXXXXXX")"
  trap 'rm -rf "$STAGE"' EXIT
else
  refuse_overwrite
  [[ -d "$ROOT/dist" ]] && NEW_DIST=0 || NEW_DIST=1
  [[ -d "$DEST" ]] && NEW_DEST=0 || NEW_DEST=1
  mkdir -p "$DEST"
  # Staged on the destination filesystem so publication is a rename. A
  # rejected or failed run removes its stage and any directory it created.
  STAGE="$(mktemp -d "$DEST/.staging-android.XXXXXXXX")"
  cleanup() {
    rm -rf "$STAGE"
    [[ "$NEW_DEST" == 0 ]] || rmdir "$DEST" 2>/dev/null || true
    [[ "$NEW_DIST" == 0 ]] || rmdir "$ROOT/dist" 2>/dev/null || true
  }
  trap cleanup EXIT
fi
cp "$APK" "$STAGE/$NAME"
APK="$STAGE/$NAME"
CERTS="$("$TOOLS/apksigner" verify --print-certs "$APK")" || die 'APK signature verification failed'
SIGNERS="$(printf '%s\n' "$CERTS" | awk '/^Signer #[0-9]+ certificate SHA-256 digest:/ {print $NF}')"
[[ -n "$SIGNERS" && "$SIGNERS" != *$'\n'* && "$(normalize "$SIGNERS")" == "$EXPECTED" ]] ||
  die 'APK signer SHA-256 fingerprint mismatch (expected exactly one release signer)'
echo "ok: signer fingerprint check passed ($EXPECTED)"
BADGING="$("$TOOLS/aapt2" dump badging "$APK")" || die 'APK badging verification failed'
[[ "$BADGING" != *application-debuggable* ]] || die 'APK is debuggable; refusing release'
[[ "$BADGING" == *"versionCode='"* ]] || die 'APK badging is missing versionCode'
# The manifest carries placeholder versions; the release must carry VERSION.
[[ "$BADGING" == *"versionName='$VERSION'"* && "$BADGING" != *"versionCode='1' versionName='dev'"* ]] ||
  die "APK version is not $VERSION (stale manifest version?)"
echo 'ok: APK is not debuggable'
printf '%s\n' "$BADGING" | awk '/^package:/ {print}'
if [[ "${DRY_RUN:-0}" == 1 ]]; then
  echo "Dry run verified: $NAME (dist untouched)"
  exit 0
fi
(cd "$STAGE" && shasum -a 256 "$NAME" >"$NAME.sha256" && shasum -a 256 -c "$NAME.sha256")
mv -f "$STAGE/$NAME" "$DEST/$NAME"
mv -f "$STAGE/$NAME.sha256" "$DEST/$NAME.sha256"
echo "Android release ready: $DEST/$NAME (+ .sha256)"
