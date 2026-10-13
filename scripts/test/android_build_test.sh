# Build-script signing transport and versionCode tests with a fake toolchain.
setup_android_build() {
  unset NU11SIGNAL_TV_KEYSTORE NU11SIGNAL_TV_KEY_ALIAS NU11SIGNAL_TV_KEYSTORE_PASS NU11SIGNAL_TV_KEY_PASS VERSION DEBUG
  mkdir -p "$T/root/android/assets/xterm" "$T/root/android/assets/fonts" "$T/sdk/platforms/android-34" \
    "$T/sdk/build-tools/35.0.0" "$T/clang"
  cp "$REPO/android/build.sh" "$T/root/android/"
  touch "$T/root/android/assets/xterm/"{xterm,addon-fit,addon-webgl}.js \
    "$T/root/android/assets/fonts/"{KodeMono.ttf,OFL.txt} "$T/sdk/platforms/android-34/android.jar"
  export ANDROID_HOME="$T/sdk" ANDROID_NDK="$T/ndk"
  local host
  case "$(uname -s)" in Darwin) host=darwin-x86_64 ;; Linux) host=linux-x86_64 ;; esac
  mkdir -p "$ANDROID_NDK/toolchains/llvm/prebuilt/$host/bin"
  ln -s /usr/bin/true "$ANDROID_NDK/toolchains/llvm/prebuilt/$host/bin/clang"
  cat >"$T/bin/go" <<'EOF'
#!/usr/bin/env bash
while (($#)); do if [[ "$1" == -o ]]; then printf 'fake Go\n' >"$2"; exit 0; fi; shift; done
exit 1
EOF
  cat >"$T/sdk/build-tools/35.0.0/aapt2" <<'EOF'
#!/usr/bin/env bash
echo "aapt2 $*" >>"$STUB_LOG"
while (($#)); do if [[ "$1" == -o ]]; then printf 'fake resources\n' >"$2"; exit 0; fi; shift; done
exit 1
EOF
  cat >"$T/sdk/build-tools/35.0.0/d8" <<'EOF'
#!/usr/bin/env bash
out=""
classes=0
while (($#)); do
  case "$1" in
    --output) out="$2"; shift 2 ;;
    *.class) [[ -f "$1" ]] || exit 1; classes=$((classes + 1)); shift ;;
    *) shift ;;
  esac
done
[[ -n "$out" && "$classes" -gt 0 ]] || exit 1
touch "$out/classes.dex"
EOF
  cat >"$T/sdk/build-tools/35.0.0/zipalign" <<'EOF'
#!/usr/bin/env bash
cp "$4" "$5"
EOF
  cat >"$T/sdk/build-tools/35.0.0/apksigner" <<'EOF'
#!/usr/bin/env bash
[[ "$1" != verify ]] || exit 0
echo "apksigner $*" >>"$STUB_LOG"
while (($#)); do
  case "$1" in
    --ks-pass|--key-pass)
      [[ "$2" == env:* ]] || exit 1
      var="${2#env:}"
      [[ -n "${!var:-}" ]] || exit 1
      shift 2 ;;
    --out) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
printf 'signed APK\n' >"$out"
EOF
  cat >"$T/bin/keytool" <<'EOF'
#!/usr/bin/env bash
while (($#)); do if [[ "$1" == -keystore ]]; then touch "$2"; exit 0; fi; shift; done
exit 1
EOF
  cat >"$T/bin/javac" <<'EOF'
#!/usr/bin/env bash
# Model javac's output so d8 receives classes on every supported Bash version.
while (($#)); do
  if [[ "$1" == -d ]]; then
    mkdir -p "$2/dev/wahh/nu11signal/tv"
    touch "$2/dev/wahh/nu11signal/tv/MainActivity.class"
    exit 0
  fi
  shift
done
exit 1
EOF
  ln -s /usr/bin/true "$T/bin/zip"
  chmod +x "$T/bin/go" "$T/bin/keytool" "$T/bin/javac" "$T/sdk/build-tools/35.0.0/"{aapt2,d8,zipalign,apksigner}
}
android_build() { run bash "$T/root/android/build.sh"; }

test_android_build_debug_default_version_code() {
  setup_android_build
  DEBUG=1 android_build
  assert_status 0
  assert_file_contains "$STUB_LOG" "--version-name dev --version-code 1 --replace-version --debug-mode"
  assert_file_contains "$STUB_LOG" '--ks-pass env:NU11SIGNAL_TV_DEBUG_PASS'
  [[ -f "$T/root/android/build/nu11signal-tv-dev.apk" ]] || fail 'no debug APK'
}

test_android_build_semver_code_and_environment_signing() {
  setup_android_build
  NU11SIGNAL_TV_KEYSTORE="$T/fake.keystore" NU11SIGNAL_TV_KEY_ALIAS=release \
    NU11SIGNAL_TV_KEYSTORE_PASS=synthetic-test-credential VERSION=12.34.56-rc.1 android_build
  assert_status 0
  assert_file_contains "$STUB_LOG" '--version-name 12.34.56-rc.1 --version-code 123456 --replace-version'
  assert_file_contains "$STUB_LOG" '--ks-pass env:NU11SIGNAL_TV_KEYSTORE_PASS --key-pass env:NU11SIGNAL_TV_KEY_PASS'
  ! grep -qF 'synthetic-test-credential' "$STUB_LOG" "$T/run.log" || fail 'credential leaked'
  ! grep -qF -- '--debug-mode' "$STUB_LOG" || fail 'release build debuggable'
}

test_android_build_rejects_version_code_collision_and_overflow() {
  setup_android_build
  local v
  for v in 1.100.1 1.1.100 999999999999.1.1; do
    VERSION="$v" android_build
    assert_failed
    assert_output_contains 'component limits'
  done
  VERSION=0.0.0 android_build
  assert_failed
  assert_output_contains 'versionCode must be between'
}
