# Tests for scripts/release.sh. Run through scripts/test/run.sh.

# assert_release_layout DIR VERSION: DIR is a complete promoted release.
assert_release_layout() {
  local dir="$1" v="$2"
  local archive="nu11signal-$v-macos-universal.tar.gz"
  [[ -x "$dir/nu11signal-$v/bin/nu11signal" ]] || fail "missing $dir/nu11signal-$v/bin/nu11signal"
  [[ -x "$dir/nu11signal-$v/libexec/Nu11SignalHelper.app/Contents/MacOS/nu11signal-helper" ]] ||
    fail "missing the helper in $dir"
  [[ -f "$dir/nu11signal-$v/LICENSE" && -f "$dir/nu11signal-$v/README.md" ]] || fail "missing LICENSE/README.md"
  [[ -f "$dir/$archive" ]] || fail "missing $dir/$archive"
  (cd "$dir" && shasum -a 256 -c "$archive.sha256" >/dev/null) || fail "checksum does not verify"
  [[ "$(ls -A "$dir" | LC_ALL=C sort | tr '\n' ' ')" == "nu11signal-$v $archive $archive.sha256 " ]] ||
    fail "unexpected entries in $dir: $(ls -A "$dir")"
  [[ "$(stat -f %Lp "$dir")" == 755 ]] || fail "$dir mode is $(stat -f %Lp "$dir"), want 755"
}

test_release_rejects_non_semver_version() {
  setup_release
  release 0.2
  assert_failed
  assert_output_contains "VERSION must be semver"
  [[ ! -e "$DIST" ]] || fail "dist/ was created"
}

test_release_promotes_with_a_single_rename() {
  setup_release
  release 0.2.1
  assert_status 0
  assert_release_layout "$DIST/v0.2.1" 0.2.1
  [[ "$(ls -A "$DIST")" == v0.2.1 ]] || fail "unexpected entries in dist/: $(ls -A "$DIST")"
  [[ "$(stub_calls mv)" == 1 ]] || fail "expected exactly one mv, got: $(grep '^mv ' "$STUB_LOG")"
  grep -qE "^mv .*/dist/\.staging-v0\.2\.1\.[^ /]+ $DIST/v0\.2\.1$" "$STUB_LOG" ||
    fail "the one mv is not the staging dir renamed to dist/v0.2.1: $(grep '^mv ' "$STUB_LOG")"
  tar -tzf "$DIST/v0.2.1/nu11signal-0.2.1-macos-universal.tar.gz" | grep -qx 'nu11signal-0.2.1/bin/nu11signal' ||
    fail "archive does not contain nu11signal-0.2.1/bin/nu11signal"
}

test_release_keeps_other_versions_untouched() {
  setup_release
  mkdir -p "$DIST/v0.2.0/nu11signal-0.2.0"
  printf 'published\n' >"$DIST/v0.2.0/nu11signal-0.2.0/marker"
  printf 'legacy\n' >"$DIST/nu11signal-0.2.0-macos-universal.tar.gz"
  release 0.2.1
  assert_status 0
  assert_file_contains "$DIST/v0.2.0/nu11signal-0.2.0/marker" published
  assert_file_contains "$DIST/nu11signal-0.2.0-macos-universal.tar.gz" legacy
  assert_release_layout "$DIST/v0.2.1" 0.2.1
}

test_release_refuses_to_overwrite_without_force() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  local before
  before="$(tree_hash "$DIST")"
  release 0.2.1
  assert_failed
  assert_output_contains "$DIST/v0.2.1 already exists"
  assert_output_contains "--force"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ "$(stub_calls go)" == 0 ]] || fail "built before refusing"
}

test_release_force_replaces_and_keeps_a_backup() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  release --force 0.2.1
  assert_status 0
  assert_release_layout "$DIST/v0.2.1" 0.2.1
  local backups
  backups="$(find "$DIST" -maxdepth 1 -name 'v0.2.1.replaced-*')"
  [[ -n "$backups" && "$(printf '%s\n' "$backups" | wc -l | tr -d ' ')" == 1 ]] ||
    fail "expected one backup, got: $backups"
  assert_file_contains "$backups/marker" published
  assert_output_contains "$backups"
  [[ "$(stub_calls mv)" == 2 ]] || fail "expected two renames, got: $(grep '^mv ' "$STUB_LOG")"
}

test_release_failure_after_staging_leaves_dist_untouched() {
  setup_release
  mkdir -p "$DIST/v0.2.0"
  printf 'published\n' >"$DIST/v0.2.0/marker"
  local before
  before="$(tree_hash "$DIST")"
  STUB_SPCTL_FAIL=1 release 0.2.1
  assert_failed
  [[ "$(stub_calls spctl)" == 1 ]] || fail "did not fail at the final spctl check"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ -z "$(staging_left "$DIST")" ]] || fail "staging left behind: $(staging_left "$DIST")"
  [[ "$(stub_calls mv)" == 0 ]] || fail "something was promoted"
}

test_release_rejected_notarization_leaves_dist_untouched() {
  setup_release
  mkdir -p "$DIST"
  local before
  before="$(tree_hash "$DIST")"
  STUB_NOTARY_STATUS=Invalid release 0.2.1
  assert_failed
  assert_output_contains "notarization status: Invalid"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ -z "$(staging_left "$DIST")" ]] || fail "staging left behind"
}

test_release_dry_run_writes_only_the_dry_run_dir() {
  setup_release
  release --dry-run 0.2.1
  assert_status 0
  local out="$T/root/build/release-dry-run/v0.2.1/nu11signal-0.2.1"
  [[ -x "$out/bin/nu11signal" ]] || fail "missing $out/bin/nu11signal"
  [[ ! -e "$DIST" ]] || fail "a dry run created dist/"
  [[ "$(stub_calls 'xcrun notarytool submit')" == 0 ]] || fail "a dry run notarized"
  release --dry-run 0.2.1
  assert_status 0
  [[ -z "$(staging_left "$T/root/build/release-dry-run")" ]] || fail "staging left behind"
}

test_release_force_restores_the_previous_release_when_promotion_fails() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  local before
  before="$(tree_hash "$DIST")"
  STUB_MV_FAIL_STAGING=1 release --force 0.2.1
  assert_failed
  assert_output_contains "could not rename"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ was not restored: $(ls -A "$DIST")"
}

test_release_force_restores_the_previous_release_when_interrupted_between_renames() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  local before
  before="$(tree_hash "$DIST")"
  STUB_MV_TERM_AFTER_BACKUP=1 release --force 0.2.1
  assert_failed
  assert_output_contains "restored the previous $DIST/v0.2.1"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ was not restored: $(ls -A "$DIST")"
}

test_release_force_reports_a_leftover_backup() {
  setup_release
  mkdir -p "$DIST/v0.2.1" "$DIST/v0.2.1.replaced-20260101T000000"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  release --force 0.2.1
  assert_status 0
  assert_output_contains "earlier backup"
  assert_output_contains "$DIST/v0.2.1.replaced-20260101T000000"
  [[ -d "$DIST/v0.2.1.replaced-20260101T000000" ]] || fail "the earlier backup was removed"
  assert_release_layout "$DIST/v0.2.1" 0.2.1
}

test_release_refuses_when_only_a_backup_is_left() {
  setup_release
  mkdir -p "$DIST/v0.2.1.replaced-20260101T000000"
  printf 'published\n' >"$DIST/v0.2.1.replaced-20260101T000000/marker"
  local before
  before="$(tree_hash "$DIST")"
  release --force 0.2.1
  assert_failed
  assert_output_contains "$DIST/v0.2.1 is missing"
  assert_output_contains "mv $DIST/v0.2.1.replaced-20260101T000000 $DIST/v0.2.1"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ "$(stub_calls go)" == 0 ]] || fail "built before refusing"
}
