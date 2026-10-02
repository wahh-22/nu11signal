# Troubleshooting

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

Common failures and their fixes.

| Symptom | Cause | Fix |
|---------|-------|-----|
| `developerTokenRequestFailed` | Missing entitlements or provisioning profile | Rebuild with a valid profile for your bundle ID and team (`make helper`) |
| Helper exits with status 137 | AMFI killed it: entitlements without an embedded profile | Check `NU11SIGNAL_PROFILE`; run `codesign -d --entitlements - build/Nu11SignalHelper.app` |
| Authorization denied | Access was refused once | System Settings > Privacy & Security > Media & Apple Music, enable the helper |
| `nu11signal-helper not found` | Helper not built or not next to the binary | `make build`, or set `NU11SIGNAL_HELPER` to an absolute path |
| `NO AUDIO OUTPUT // IS PULSEAUDIO OR PIPEWIRE RUNNING?` (Linux, local files) | The sound output did not open within 5 seconds, or took no audio for 5 seconds of a playing song: no PulseAudio or PipeWire server was reached, and ALSA has no sound card | Start PulseAudio or PipeWire (`pipewire-pulse`), or point `PULSE_SERVER` at its socket (see [Install › Linux](install.md#linux)), then play again |
