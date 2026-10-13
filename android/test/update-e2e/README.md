# Isolated TV updater experiment

Test-only instrumentation for the exact **0.0.3 → 0.0.4** transition. It refuses
other installed versions. Do not downgrade or clear app data to rerun it.

The helper is signed with the release certificate and is privileged: **never
publish its APK**. It packages only test bytecode and the signed update fixture;
production classes are loaded from the target app, not duplicated in test dex.

## Run with device-owner approval

1. Build the signed 0.0.4 APK with `make android-release VERSION=0.0.4`.
2. Build the helper:
   ```sh
   bash scripts/build-android-update-e2e.sh dist/v0.0.4/nu11signal-tv-0.0.4.apk
   ```
   The script prints its unique output path under ignored `android/build/`.
   It verifies the fixture signature, pinned certificate, package and version
   before embedding it; passwords are read from the existing environment or
   macOS Keychain and passed to apksigner through environment references.
3. Install **only the helper** at that printed path, never the target update:
   ```sh
   adb -s "$TV" install -r <printed-helper-apk>
   adb -s "$TV" shell am instrument -r \
     dev.wahh.nu11signal.tv.updatetest/dev.wahh.nu11signal.tv.updatetest.UpdateE2eInstrumentation
   ```
4. Check `Nu11UpdateE2E` logcat for `READY` and `DISCOVERY`. The helper replaces
   only the native client's transport before Activity resume. It supplies
   release metadata and embedded APK bytes; unexpected requests fail. Apple
   Music can still use the network normally.
5. Using the remote, choose **Download and install**, allow installations
   from nu11signal if Android requests it, return and choose the update again.
   Confirm installation in Android's own installer. No test code presses
   confirmation or calls the production installer directly.
6. Independently inspect the installed version/code with `dumpsys package`,
   reopen the app and confirm the session remains. Process termination alone
   is not successful-install evidence.
7. With the owner's approval, uninstall only
   `dev.wahh.nu11signal.tv.updatetest`. Keep helper APKs private.

## Observed evidence

On Google TV Streamer (Android 14), 2026-10-13:

- Helper syntax check, SDK34 compilation and independent safety review passed.
- Release 0.0.4 and helper signature checks passed against the pinned release key.
- Instrumentation logged `READY` and `DISCOVERY` against unchanged release 0.0.3.
- User completed the remote flow. Package inspection then showed 0.0.4/code4;
  `firstInstallTime` stayed unchanged. The target 0.0.4 was **not installed by ADB**.
- User confirmed the Apple Music session remained. Helper uninstall succeeded;
  package listing retained only `dev.wahh.nu11signal.tv` at version 0.0.4.

This proves the isolated metadata/download, validation and real Android
installation path on this device. It does **not** prove GitHub production HTTPS
or release publication. No release was published during this experiment.
