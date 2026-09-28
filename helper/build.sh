#!/usr/bin/env bash
# Builds soulking-helper and packages it as a signed, windowless
# build/SoulKingHelper.app at the repository root.
#
# Configuration (environment):
#   SOULKING_BUNDLE_ID      bundle identifier      (default: dev.wahh.soulking.player)
#   SOULKING_TEAM_ID        Apple Developer team   (default: W6GZP998GQ)
#   SOULKING_PROFILE        provisioning profile   (default: spike/SoulKing_Player.provisionprofile)
#   SOULKING_SIGN_IDENTITY  codesign identity      (default: Apple Development)
#
# Relative paths are resolved from the repository root.
set -euo pipefail

HELPER_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HELPER_DIR/.." && pwd)"
cd "$ROOT"

BUNDLE_ID="${SOULKING_BUNDLE_ID:-dev.wahh.soulking.player}"
TEAM_ID="${SOULKING_TEAM_ID:-W6GZP998GQ}"
PROFILE="${SOULKING_PROFILE:-spike/SoulKing_Player.provisionprofile}"
IDENTITY="${SOULKING_SIGN_IDENTITY:-Apple Development}"
APP="$ROOT/build/SoulKingHelper.app"

if [[ ! -f "$PROFILE" ]]; then
  echo "error: provisioning profile not found: $PROFILE" >&2
  echo "       Download the profile for $TEAM_ID.$BUNDLE_ID from the Apple Developer portal" >&2
  echo "       and point SOULKING_PROFILE at it. MusicKit playback is killed without it." >&2
  exit 1
fi

echo "==> Building soulking-helper (release)"
swift build -c release --package-path "$HELPER_DIR"
BIN_DIR="$(swift build -c release --package-path "$HELPER_DIR" --show-bin-path)"

echo "==> Assembling $APP"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
cp "$BIN_DIR/soulking-helper" "$APP/Contents/MacOS/soulking-helper"
cp "$PROFILE" "$APP/Contents/embedded.provisionprofile"
sed "s/__BUNDLE_ID__/$BUNDLE_ID/g" "$HELPER_DIR/Resources/Info.plist.in" > "$APP/Contents/Info.plist"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/soulking-helper.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT
ENTITLEMENTS="$WORK_DIR/Entitlements.plist"
sed -e "s/__BUNDLE_ID__/$BUNDLE_ID/g" -e "s/__TEAM_ID__/$TEAM_ID/g" \
  "$HELPER_DIR/Resources/Entitlements.plist.in" > "$ENTITLEMENTS"
plutil -lint "$APP/Contents/Info.plist" "$ENTITLEMENTS" >/dev/null

echo "==> Signing with \"$IDENTITY\""
codesign --force --sign "$IDENTITY" --entitlements "$ENTITLEMENTS" "$APP"
codesign --verify --strict --verbose "$APP"
echo "==> Built $APP"
