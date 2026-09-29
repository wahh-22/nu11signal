#!/usr/bin/env bash
# Builds nu11signal-helper and packages it as a signed, windowless
# Nu11SignalHelper.app (by default build/Nu11SignalHelper.app at the repository
# root).
#
# Configuration (environment):
#   NU11SIGNAL_SIGN_MODE      development | release  (default: development)
#   NU11SIGNAL_BUNDLE_ID      bundle identifier      (default: dev.wahh.soulking.player)
#   NU11SIGNAL_TEAM_ID        Apple Developer team   (default: W6GZP998GQ)
#   NU11SIGNAL_PROFILE        provisioning profile   (development default: signing/Nu11Signal_Dev.provisionprofile;
#                                                     release default: signing/Nu11Signal_DeveloperID.provisionprofile)
#   NU11SIGNAL_SIGN_IDENTITY  codesign identity      (development default: Apple Development;
#                                                     release default: Developer ID Application)
#   NU11SIGNAL_BUILD_DIR      output directory       (default: build)
#   NU11SIGNAL_VERSION        stamps CFBundleShortVersionString/CFBundleVersion (optional)
#
# Release mode builds a universal (arm64 + x86_64) binary and signs with the
# hardened runtime and a secure timestamp, as notarization requires.
# NU11SIGNAL_SIGN_IDENTITY=- signs ad hoc (dry runs only): the profile becomes
# optional and the result cannot use MusicKit.
#
# Relative paths are resolved from the repository root.
set -euo pipefail

HELPER_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HELPER_DIR/.." && pwd)"
cd "$ROOT"

MODE="${NU11SIGNAL_SIGN_MODE:-development}"
BUNDLE_ID="${NU11SIGNAL_BUNDLE_ID:-dev.wahh.soulking.player}"
TEAM_ID="${NU11SIGNAL_TEAM_ID:-W6GZP998GQ}"
BUILD_DIR="${NU11SIGNAL_BUILD_DIR:-build}"

case "$MODE" in
  development)
    DEFAULT_PROFILE="signing/Nu11Signal_Dev.provisionprofile"
    DEFAULT_IDENTITY="Apple Development"
    PROFILE_KIND="macOS App Development"
    BUILD_ARGS=(-c release)
    ;;
  release)
    DEFAULT_PROFILE="signing/Nu11Signal_DeveloperID.provisionprofile"
    DEFAULT_IDENTITY="Developer ID Application"
    PROFILE_KIND="Developer ID"
    BUILD_ARGS=(-c release --arch arm64 --arch x86_64)
    ;;
  *)
    echo "error: NU11SIGNAL_SIGN_MODE must be development or release, got: $MODE" >&2
    exit 1
    ;;
esac

PROFILE="${NU11SIGNAL_PROFILE:-$DEFAULT_PROFILE}"
IDENTITY="${NU11SIGNAL_SIGN_IDENTITY:-$DEFAULT_IDENTITY}"
if [[ ! -f "$PROFILE" ]]; then
  if [[ "$IDENTITY" != "-" ]]; then
    echo "error: provisioning profile not found: $PROFILE" >&2
    echo "       Download the $PROFILE_KIND profile for $TEAM_ID.$BUNDLE_ID from the" >&2
    echo "       Apple Developer portal and save it as $DEFAULT_PROFILE (gitignored)," >&2
    echo "       or point NU11SIGNAL_PROFILE at it. MusicKit playback is killed without it." >&2
    exit 1
  fi
  echo "warning: ad-hoc signing without a provisioning profile ($PROFILE missing); MusicKit will not work" >&2
  PROFILE=""
fi

mkdir -p "$BUILD_DIR"
APP="$(cd "$BUILD_DIR" && pwd)/Nu11SignalHelper.app"

if [[ "$MODE" == development ]]; then
  echo "==> Building nu11signal-helper (release)"
else
  echo "==> Building nu11signal-helper (release, universal)"
fi
swift build "${BUILD_ARGS[@]}" --package-path "$HELPER_DIR"
BIN_DIR="$(swift build "${BUILD_ARGS[@]}" --package-path "$HELPER_DIR" --show-bin-path)"

echo "==> Assembling $APP"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
cp "$BIN_DIR/nu11signal-helper" "$APP/Contents/MacOS/nu11signal-helper"
if [[ -n "$PROFILE" ]]; then
  cp "$PROFILE" "$APP/Contents/embedded.provisionprofile"
fi
sed "s/__BUNDLE_ID__/$BUNDLE_ID/g" "$HELPER_DIR/Resources/Info.plist.in" > "$APP/Contents/Info.plist"
if [[ -n "${NU11SIGNAL_VERSION:-}" ]]; then
  BUNDLE_VERSION="${NU11SIGNAL_VERSION%%-*}" # bundle versions are numeric: drop any pre-release suffix
  plutil -replace CFBundleShortVersionString -string "$BUNDLE_VERSION" "$APP/Contents/Info.plist"
  plutil -replace CFBundleVersion -string "$BUNDLE_VERSION" "$APP/Contents/Info.plist"
fi

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/nu11signal-helper.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT
ENTITLEMENTS="$WORK_DIR/Entitlements.plist"
sed -e "s/__BUNDLE_ID__/$BUNDLE_ID/g" -e "s/__TEAM_ID__/$TEAM_ID/g" \
  "$HELPER_DIR/Resources/Entitlements.plist.in" > "$ENTITLEMENTS"
plutil -lint "$APP/Contents/Info.plist" "$ENTITLEMENTS" >/dev/null

echo "==> Signing with \"$IDENTITY\""
if [[ "$MODE" == development ]]; then
  codesign --force --sign "$IDENTITY" --entitlements "$ENTITLEMENTS" "$APP"
  codesign --verify --strict --verbose "$APP"
else
  TIMESTAMP=(--timestamp)
  [[ "$IDENTITY" == "-" ]] && TIMESTAMP=(--timestamp=none) # ad hoc cannot be timestamped
  codesign --force --options runtime "${TIMESTAMP[@]}" --sign "$IDENTITY" \
    --entitlements "$ENTITLEMENTS" "$APP"
  codesign --verify --strict --deep --verbose=2 "$APP"
fi
echo "==> Built $APP"
