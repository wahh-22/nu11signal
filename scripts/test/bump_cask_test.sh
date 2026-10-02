# Tests for scripts/bump-cask.sh. Run through scripts/test/run.sh.

test_bump_rejects_non_semver_version() {
  bump 0.2
  assert_failed
  assert_output_contains "VERSION must be semver"
  [[ "$(stub_calls brew)" == 0 ]] || fail "brew was called before validation"
}

test_bump_requires_the_versioned_checksum() {
  setup_tap
  bump 0.2.1
  assert_failed
  assert_output_contains "checksum not found: $T/root/dist/v0.2.1/nu11signal-0.2.1-macos-universal.tar.gz.sha256"
  [[ ! -e "$TAP" ]] || fail "the tap was cloned without a checksum"
}

test_bump_rejects_a_checksum_for_another_archive() {
  setup_tap
  mkdir -p "$T/root/dist/v0.2.1"
  printf '%s  nu11signal-0.2.0-macos-universal.tar.gz\n' "$SHA_B" \
    >"$T/root/dist/v0.2.1/nu11signal-0.2.1-macos-universal.tar.gz.sha256"
  bump 0.2.1
  assert_failed
  assert_output_contains "not nu11signal-0.2.1-macos-universal.tar.gz"
}

test_bump_renders_and_audits_without_committing() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1
  assert_status 0
  assert_output_contains "Not pushed"
  diff <(render_cask 0.2.1 "$SHA_B") "$TAP/Casks/nu11signal.rb" || fail "unexpected render"
  diff <(render_formula 0.2.1 "$SHA_B" "$SHA_C" "$SHA_D") "$TAP/Formula/nu11signal.rb" || fail "unexpected formula render"
  [[ "$(stub_calls 'brew audit --cask')" == 1 ]] || fail "brew audit --cask was not run once"
  [[ "$(stub_calls 'brew audit --formula')" == 1 ]] || fail "brew audit --formula was not run once"
  [[ "$(git -C "$TAP" rev-list --count HEAD)" == 1 ]] || fail "a commit was created"
  [[ "$(origin_commits)" == 1 ]] || fail "origin changed"
  [[ -z "$(ls "$STUB_BREW_REPO/Library/Taps")" ]] || fail "the temporary audit tap was left behind"
}

test_bump_refuses_a_tap_with_an_untracked_file_elsewhere() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf 'x\n' >"$TAP/notes.txt"
  bump 0.2.1
  assert_failed
  assert_output_contains "uncommitted changes"
  assert_output_contains "notes.txt"
}

test_bump_refuses_an_untracked_file_next_to_the_cask() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf 'x\n' >"$TAP/Casks/nu11signal.rb.orig"
  bump 0.2.1
  assert_failed
  assert_output_contains "Casks/nu11signal.rb.orig"
}

test_bump_refuses_a_modified_tracked_file() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf 'changed\n' >>"$TAP/README.md"
  bump 0.2.1
  assert_failed
  assert_output_contains "README.md"
}

test_bump_allows_a_cask_left_by_an_earlier_run() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf '# stale render\n' >>"$TAP/Casks/nu11signal.rb"
  bump 0.2.1
  assert_status 0
  assert_output_contains "Not pushed"
  diff <(render_cask 0.2.1 "$SHA_B") "$TAP/Casks/nu11signal.rb" || fail "unexpected render"
}

test_bump_push_commits_and_pushes_once() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1 --push
  assert_status 0
  assert_output_contains "Pushed"
  [[ "$(origin_commits)" == 2 ]] || fail "expected one new commit on origin"
  [[ "$(git --git-dir="$ORIGIN" log -1 --format=%s main)" == "chore: bump nu11signal to 0.2.1" ]] ||
    fail "unexpected commit subject"
  diff <(render_cask 0.2.1 "$SHA_B") <(git --git-dir="$ORIGIN" show main:Casks/nu11signal.rb) ||
    fail "origin cask differs from the render"
  diff <(render_formula 0.2.1 "$SHA_B" "$SHA_C" "$SHA_D") <(git --git-dir="$ORIGIN" show main:Formula/nu11signal.rb) ||
    fail "origin formula differs from the render"
  [[ "$(git --git-dir="$ORIGIN" show --format= --name-only main | LC_ALL=C sort | tr '\n' ' ')" == \
    "Casks/nu11signal.rb Formula/nu11signal.rb " ]] || fail "the bump commit does not hold exactly the cask and the formula"
}

test_bump_push_with_an_unchanged_render_creates_no_commit() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1 --push
  assert_status 0
  bump 0.2.1 --push
  assert_status 0
  assert_output_contains "nothing to commit"
  [[ "$(origin_commits)" == 2 ]] || fail "a duplicate commit reached origin"
  [[ "$(git -C "$TAP" rev-list --count HEAD)" == 2 ]] || fail "a duplicate local commit was created"
}

# leave_unpushed_bump VERSION SHA: the state after `--push` committed but the
# push itself failed.
leave_unpushed_bump() {
  bump "$1"
  assert_status 0
  git -C "$TAP" commit --quiet -am "chore: bump nu11signal to $1"
}

test_bump_reports_an_unpushed_bump_without_push() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  bump 0.2.1
  assert_status 0
  assert_output_contains "unpushed"
  assert_output_contains "chore: bump nu11signal to 0.2.1"
  assert_output_contains "--push"
  [[ "$(origin_commits)" == 1 ]] || fail "pushed without --push"
}

test_bump_push_resumes_an_unpushed_bump() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  local bump_commit
  bump_commit="$(git -C "$TAP" rev-parse HEAD)"
  bump 0.2.1 --push
  assert_status 0
  assert_output_contains "Pushed"
  [[ "$(git --git-dir="$ORIGIN" rev-parse main)" == "$bump_commit" ]] ||
    fail "origin is not at the earlier bump commit (duplicate or missing push)"
  [[ "$(git -C "$TAP" rev-list --count HEAD)" == 2 ]] || fail "a duplicate local commit was created"
  [[ "$(stub_calls 'brew audit --cask')" == 2 ]] || fail "the resumed push was not audited (cask)"
  [[ "$(stub_calls 'brew audit --formula')" == 2 ]] || fail "the resumed push was not audited (formula)"
}

test_bump_push_refuses_an_unpushed_bump_that_differs_from_the_render() {
  setup_tap
  write_checksum 0.2.1 "$SHA_A"
  leave_unpushed_bump 0.2.1
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "does not match"
  [[ "$(origin_commits)" == 1 ]] || fail "a mismatched bump was pushed"
}

test_bump_refuses_unrelated_unpushed_commits() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf 'local\n' >>"$TAP/README.md"
  git -C "$TAP" commit --quiet -am "docs: local change"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "docs: local change"
  [[ "$(origin_commits)" == 1 ]] || fail "unrelated commits were pushed"
}

test_bump_warns_about_an_unpushed_bump_that_differs_from_the_render_without_push() {
  setup_tap
  write_checksum 0.2.1 "$SHA_A"
  leave_unpushed_bump 0.2.1
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1
  assert_status 0
  assert_output_contains "does not match the render"
  assert_output_contains "reset --hard origin/main"
  [[ "$(origin_commits)" == 1 ]] || fail "pushed without --push"
}

test_bump_refuses_an_unpushed_bump_for_another_version() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  write_checksum 0.2.2 "$SHA_B"
  bump 0.2.2 --push
  assert_failed
  assert_output_contains "chore: bump nu11signal to 0.2.1"
  assert_output_contains "not bump commits for 0.2.2"
  [[ "$(origin_commits)" == 1 ]] || fail "unrelated commits were pushed"
}

test_bump_refuses_an_unpushed_bump_that_touches_other_files() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1
  assert_status 0
  printf 'local\n' >>"$TAP/README.md"
  git -C "$TAP" commit --quiet -am "chore: bump nu11signal to 0.2.1"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "change files other than Casks/nu11signal.rb"
  assert_output_contains "README.md"
  assert_output_lacks "      Casks/nu11signal.rb"
  [[ "$(origin_commits)" == 1 ]] || fail "a bump touching other files was pushed"
}

test_bump_explains_unpushed_bumps_with_no_net_change() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  render_cask 0.2.0 "$SHA_A" >"$TAP/Casks/nu11signal.rb"
  git -C "$TAP" rm --quiet Formula/nu11signal.rb
  git -C "$TAP" commit --quiet -am "chore: bump nu11signal to 0.2.1"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "no net change"
  assert_output_contains "reset --hard origin/main"
  [[ "$(origin_commits)" == 1 ]] || fail "empty bump commits were pushed"
}

# advance_origin: a new upstream commit (README.md only) the tap does not have.
advance_origin() {
  printf 'upstream\n' >>"$T/seed/README.md"
  git -C "$T/seed" commit --quiet -am "docs: upstream change"
  git -C "$T/seed" push --quiet origin HEAD:main 2>/dev/null
}

test_bump_reports_an_up_to_date_tap() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1
  assert_status 0
  assert_output_contains "up to date with origin/main"
}

test_bump_fast_forwards_a_tap_behind_its_upstream() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  advance_origin
  bump 0.2.1
  assert_status 0
  assert_output_contains "behind origin/main by 1 commit"
  [[ "$(git -C "$TAP" rev-parse HEAD)" == "$(git --git-dir="$ORIGIN" rev-parse main)" ]] ||
    fail "the tap was not fast-forwarded"
}

test_bump_refuses_a_diverged_tap_before_pulling() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  advance_origin
  local head
  head="$(git -C "$TAP" rev-parse HEAD)"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "has diverged from origin/main"
  assert_output_contains "the checkout has 1 commit not on origin/main"
  assert_output_contains "chore: bump nu11signal to 0.2.1"
  assert_output_contains "pull --rebase"
  assert_output_lacks "could not fast-forward"
  [[ "$(git -C "$TAP" rev-parse HEAD)" == "$head" ]] || fail "the diverged tap was changed"
  [[ "$(origin_commits)" == 2 ]] || fail "origin changed"
}

test_bump_refuses_a_tap_branch_without_upstream() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  git -C "$TAP" checkout --quiet -b local-only
  bump 0.2.1
  assert_failed
  assert_output_contains "has no upstream"
}

# --- Formula -----------------------------------------------------------------------

test_bump_requires_the_linux_checksums() {
  setup_tap
  write_sha_file 0.2.1 nu11signal-0.2.1-macos-universal.tar.gz "$SHA_B"
  write_sha_file 0.2.1 nu11signal-0.2.1-linux-arm64.tar.gz "$SHA_D"
  bump 0.2.1
  assert_failed
  assert_output_contains "checksum not found: $T/root/dist/v0.2.1/nu11signal-0.2.1-linux-amd64.tar.gz.sha256"
  assert_output_contains "make release VERSION=0.2.1"
  [[ ! -e "$TAP" ]] || fail "the tap was cloned without the Linux checksums"
}

test_bump_rejects_a_linux_checksum_for_another_archive() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  write_sha_file 0.2.1 nu11signal-0.2.1-linux-arm64.tar.gz "$SHA_D"
  printf '%s  nu11signal-0.2.1-linux-amd64.tar.gz\n' "$SHA_D" \
    >"$T/root/dist/v0.2.1/nu11signal-0.2.1-linux-arm64.tar.gz.sha256"
  bump 0.2.1
  assert_failed
  assert_output_contains "not nu11signal-0.2.1-linux-arm64.tar.gz"
}

test_bump_renders_the_formula_with_all_three_checksums() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B" "$SHA_C" "$SHA_D"
  bump 0.2.1
  assert_status 0
  local f="$TAP/Formula/nu11signal.rb"
  diff <(render_formula 0.2.1 "$SHA_B" "$SHA_C" "$SHA_D") "$f" || fail "unexpected formula render"
  assert_file_contains "$f" "class Nu11signal < Formula"
  assert_file_contains "$f" 'url "https://github.com/wahh-22/nu11signal/releases/download/v0.2.1/nu11signal-0.2.1-macos-universal.tar.gz"'
  grep -A1 -F 'macos-universal.tar.gz"' "$f" | grep -qF "sha256 \"$SHA_B\"" || fail "macOS url is not followed by its sha256"
  # The cask and the formula carry the same macOS sha256.
  assert_file_contains "$TAP/Casks/nu11signal.rb" "sha256 \"$SHA_B\""
  assert_file_contains "$f" 'prefix.install "bin", "libexec"'
  ! grep -qF "depends_on :linux" "$f" || fail "the formula is still Linux-only"
  assert_file_contains "$f" 'url "https://github.com/wahh-22/nu11signal/releases/download/v0.2.1/nu11signal-0.2.1-linux-amd64.tar.gz"'
  assert_file_contains "$f" 'url "https://github.com/wahh-22/nu11signal/releases/download/v0.2.1/nu11signal-0.2.1-linux-arm64.tar.gz"'
  # The amd64 sha256 follows the amd64 url, the arm64 sha256 the arm64 url.
  grep -A1 -F 'linux-amd64.tar.gz"' "$f" | grep -qF "sha256 \"$SHA_C\"" || fail "amd64 url is not followed by its sha256"
  grep -A1 -F 'linux-arm64.tar.gz"' "$f" | grep -qF "sha256 \"$SHA_D\"" || fail "arm64 url is not followed by its sha256"
  ! grep -q '^#' "$f" || fail "the template comment was rendered"
  grep -qE "^ruby -c $TAP/Formula/nu11signal.rb$" "$STUB_LOG" || fail "ruby -c was not run on the formula"
  assert_output_contains "+class Nu11signal < Formula"
}

test_bump_audits_and_styles_the_formula() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1
  assert_status 0
  grep -qE "^brew audit --formula --strict nu11signal-bump-[0-9]+/local/nu11signal$" "$STUB_LOG" ||
    fail "brew audit --formula --strict was not run: $(grep '^brew ' "$STUB_LOG")"
  grep -qE "^brew style nu11signal-bump-[0-9]+/local/nu11signal$" "$STUB_LOG" ||
    fail "brew style was not run on the formula: $(grep '^brew ' "$STUB_LOG")"
  [[ "$(stub_calls 'brew audit --cask')" == 1 ]] || fail "the cask was not audited"
  [[ -z "$(ls "$STUB_BREW_REPO/Library/Taps")" ]] || fail "the temporary audit tap was left behind"
}

test_bump_push_stops_when_the_formula_check_fails() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  STUB_BREW_STYLE_EXIT=1 bump 0.2.1 --push
  assert_failed
  [[ "$(origin_commits)" == 1 ]] || fail "pushed after a failed formula check"
  [[ -z "$(ls "$STUB_BREW_REPO/Library/Taps")" ]] || fail "the temporary audit tap was left behind"
  STUB_BREW_FORMULA_AUDIT_EXIT=1 bump 0.2.1 --push
  assert_failed
  [[ "$(origin_commits)" == 1 ]] || fail "pushed after a failed formula audit"
}

test_bump_allows_a_formula_left_by_an_earlier_run() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  mkdir -p "$TAP/Formula"
  printf '# stale render\n' >"$TAP/Formula/nu11signal.rb"
  bump 0.2.1
  assert_status 0
  assert_output_contains "Not pushed"
  diff <(render_formula 0.2.1 "$SHA_B" "$SHA_C" "$SHA_D") "$TAP/Formula/nu11signal.rb" || fail "unexpected formula render"
}

test_bump_refuses_an_untracked_file_next_to_the_formula() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  mkdir -p "$TAP/Formula"
  printf 'x\n' >"$TAP/Formula/nu11signal.rb.orig"
  bump 0.2.1
  assert_failed
  assert_output_contains "Formula/nu11signal.rb.orig"
}

test_bump_push_commits_a_changed_formula_alone() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1 --push
  assert_status 0
  write_checksum 0.2.1 "$SHA_B" "$SHA_A" "$SHA_D"
  bump 0.2.1 --push
  assert_status 0
  assert_output_contains "Pushed"
  [[ "$(origin_commits)" == 3 ]] || fail "expected a second bump commit on origin"
  [[ "$(git --git-dir="$ORIGIN" show --format= --name-only main)" == "Formula/nu11signal.rb" ]] ||
    fail "the second bump commit does not hold only the formula"
  diff <(render_formula 0.2.1 "$SHA_B" "$SHA_A" "$SHA_D") <(git --git-dir="$ORIGIN" show main:Formula/nu11signal.rb) ||
    fail "origin formula differs from the render"
}

test_bump_push_resumes_an_unpushed_bump_with_the_formula() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  [[ "$(git -C "$TAP" show --format= --name-only HEAD | LC_ALL=C sort | tr '\n' ' ')" == \
    "Casks/nu11signal.rb Formula/nu11signal.rb " ]] || fail "the unpushed bump does not hold both files"
  bump 0.2.1 --push
  assert_status 0
  assert_output_contains "Pushed"
  diff <(render_formula 0.2.1 "$SHA_B" "$SHA_C" "$SHA_D") <(git --git-dir="$ORIGIN" show main:Formula/nu11signal.rb) ||
    fail "origin formula differs from the render"
}

test_bump_push_refuses_an_unpushed_bump_whose_formula_differs_from_the_render() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B" "$SHA_C" "$SHA_D"
  leave_unpushed_bump 0.2.1
  write_checksum 0.2.1 "$SHA_B" "$SHA_A" "$SHA_D"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "does not match"
  [[ "$(origin_commits)" == 1 ]] || fail "a mismatched bump was pushed"
}
