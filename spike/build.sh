#!/usr/bin/env bash
# Builds and signs the spike helper as a windowless .app bundle.
set -euo pipefail
cd "$(dirname "$0")"

APP=build/SoulKingHelper.app
IDENTITY="${SOULKING_SIGN_IDENTITY:-Apple Development}"
PROFILE="${SOULKING_PROFILE:-SoulKing_Player.provisionprofile}"

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
cp Info.plist "$APP/Contents/Info.plist"
cp "$PROFILE" "$APP/Contents/embedded.provisionprofile"
swiftc -O -target arm64-apple-macos14 Spike.swift -o "$APP/Contents/MacOS/soulking-helper"
codesign --force --sign "$IDENTITY" --entitlements Entitlements.plist "$APP"
codesign --verify --verbose "$APP"
