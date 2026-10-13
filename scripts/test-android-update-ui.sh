#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$ROOT/android/build"
WORK="$(mktemp -d "$ROOT/android/build/update-ui-test.XXXXXXXX")"
javac --release 11 -d "$WORK" \
  "$ROOT/android/src/dev/wahh/nu11signal/tv/TvUpdateState.java" \
  "$ROOT/android/test/update-ui/TvUpdateStateTest.java"
java -cp "$WORK" dev.wahh.nu11signal.tv.TvUpdateStateTest
