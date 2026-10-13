#!/usr/bin/env bash
# Pure JVM policy tests first; then compile only this platform boundary (no peer/UI files).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEST="$ROOT/android/test/update-installer"
OUT="$(mktemp -d "$TEST/build.XXXXXXXX")"
SRC="$ROOT/android/src/dev/wahh/nu11signal/tv"
javac --release 11 -d "$OUT" "$SRC/TvUpdateGuard.java" "$TEST/InstallerGuardTest.java" "$TEST/SnapshotLifecycleTest.java"
java -cp "$OUT" dev.wahh.nu11signal.tv.InstallerGuardTest "$OUT/updates"
java -cp "$OUT" dev.wahh.nu11signal.tv.SnapshotLifecycleTest "$OUT/lifecycle"
SDK="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
JAR="$SDK/platforms/${ANDROID_PLATFORM:-android-34}/android.jar"
[[ -f "$JAR" ]] || { echo "Missing Android platform jar: $JAR" >&2; exit 1; }
javac --release 11 -classpath "$JAR" -d "$OUT" \
  "$SRC/TvUpdateGuard.java" "$SRC/TvUpdateApkProvider.java" "$SRC/TvUpdateInstaller.java"
printf 'PASS: platform-only installer boundary compiled against %s\n' "$JAR"
