#!/usr/bin/env bash
# Runs the hermetic tests for scripts/release.sh and scripts/bump-cask.sh.
#
# Usage: scripts/test/run.sh [TEST_FILE...] [-k PATTERN]
#
# Each test_* function in scripts/test/*_test.sh runs in its own bash process
# with a fresh sandbox (see lib.sh): stubbed tools first on PATH, temp dirs
# for dist/ and the tap, and a local bare repository as the tap's origin.
# Nothing outside the sandbox is read or written except the scripts under
# test, which are copied in. KEEP_TEST_TMP=1 keeps the sandboxes.
set -uo pipefail

TEST_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$TEST_DIR/../.." && pwd)"
export TEST_DIR REPO

files=()
pattern=""
while (($# > 0)); do
  case "$1" in
    -k)
      pattern="${2:?-k needs a pattern}"
      shift 2
      ;;
    *)
      files+=("$1")
      shift
      ;;
  esac
done
if ((${#files[@]} == 0)); then
  files=("$TEST_DIR"/*_test.sh)
fi

pass=0
failed=()
for file in "${files[@]}"; do
  for name in $(grep -oE '^test_[A-Za-z0-9_]+' "$file"); do
    [[ -z "$pattern" || "$name" == *"$pattern"* ]] || continue
    if out="$(bash -euo pipefail -c 'source "$TEST_DIR/lib.sh"; source "$1"; "$2"' _ "$file" "$name" 2>&1)"; then
      echo "ok   $name"
      pass=$((pass + 1))
    else
      echo "FAIL $name"
      printf '%s\n' "$out" | sed 's/^/     /'
      failed+=("$name")
    fi
  done
done

echo
echo "$pass passed, ${#failed[@]} failed"
((${#failed[@]} == 0))
