#!/usr/bin/env bash
# Builds the spike and signs it as its own app bundle with the helper's
# development identity, bundle id and provisioning profile (MusicKit needs
# the App ID's entitlement). Output: $1 (default: ./build)/LocalQueueSpike.app
set -euo pipefail
SPIKE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SPIKE/../.." && pwd)"
OUT="${1:-$SPIKE/build}"
BUNDLE_ID="${NU11SIGNAL_BUNDLE_ID:-dev.wahh.soulking.player}"
TEAM_ID="${NU11SIGNAL_TEAM_ID:-W6GZP998GQ}"
swift build -c release --package-path "$SPIKE"
BIN="$(swift build -c release --package-path "$SPIKE" --show-bin-path)/LocalQueueSpike"
APP="$OUT/LocalQueueSpike.app"
rm -rf "$APP"; mkdir -p "$APP/Contents/MacOS"
cp "$BIN" "$APP/Contents/MacOS/nu11signal-helper"
cp "$ROOT/signing/Nu11Signal_Dev.provisionprofile" "$APP/Contents/embedded.provisionprofile"
sed "s/__BUNDLE_ID__/$BUNDLE_ID/g" "$ROOT/helper/Resources/Info.plist.in" > "$APP/Contents/Info.plist"
ENT="$(mktemp -d)/Entitlements.plist"
sed -e "s/__BUNDLE_ID__/$BUNDLE_ID/g" -e "s/__TEAM_ID__/$TEAM_ID/g" "$ROOT/helper/Resources/Entitlements.plist.in" > "$ENT"
codesign --force --sign "Apple Development" --entitlements "$ENT" "$APP"
codesign --verify --strict "$APP"
echo "$APP"
