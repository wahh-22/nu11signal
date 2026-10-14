#!/usr/bin/env bash
# Host-JVM tests for the animated-artwork request filter of the Android TV
# music WebView (pure Java, no Android SDK needed).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$ROOT/android/build"
WORK="$(mktemp -d "$ROOT/android/build/motion-artwork-test.XXXXXXXX")"
javac --release 11 -d "$WORK" \
  "$ROOT/android/src/dev/wahh/nu11signal/tv/MotionArtwork.java" \
  "$ROOT/android/test/motion-artwork/MotionArtworkTest.java"
java -cp "$WORK" dev.wahh.nu11signal.tv.MotionArtworkTest
