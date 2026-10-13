#!/usr/bin/env bash
# Offline JVM tests; keep compiler/test stdout, stderr and exit status intact.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d "$ROOT/android/test/update-client/.runs/run.XXXXXXXX")"
javac --release 11 -d "$WORK" \
  "$ROOT/android/src/dev/wahh/nu11signal/tv/TvUpdateRelease.java" \
  "$ROOT/android/src/dev/wahh/nu11signal/tv/TvUpdateClient.java" \
  "$ROOT/android/test/update-client/TvUpdateClientTest.java"
java -Dtest.work="$WORK" -cp "$WORK" dev.wahh.nu11signal.tv.TvUpdateClientTest
