# App volume and spectrum

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

How the helper gives nu11signal its own volume through a Core Audio process tap, and how the same tap feeds the rain's real spectrum.

## App volume

MusicKit plays the helper's audio in a separate macOS process
(`RemotePlayerService`), so the helper cannot scale its own output. Instead
it creates a Core Audio process tap (macOS 14.2+) on that process, muting
its direct output, and plays the tapped audio through a private aggregate
device on the default output device, scaled by the app volume (a square-law
curve, ramped over 15 ms so changes do not click). This adds about 60 ms of
output latency. The level persists in the helper's preferences.

- **Permission.** A tap needs the *audio capture* permission
  (`NSAudioCaptureUsageDescription`: "Nu11Signal adjusts its own playback
  volume…"). macOS asks on the first playback; the music plays at the system
  volume (`SYS`) until it is granted, and stays there if it is denied (grant
  it later in System Settings › Privacy & Security › Screen & System Audio
  Recording, then restart the app). Nothing is muted while the prompt waits.
- **Launch.** macOS attributes that permission to the *responsible* process,
  which for a program started from a terminal is the terminal. The helper
  therefore re-executes itself once at startup, in place (same process, same
  pipes), with the private `responsibility_spawnattrs_setdisclaim` spawn
  attribute (as LLDB and Chromium do), so the prompt names Nu11Signal. When
  the private function is missing, the helper stays on the system volume.
- **Which process.** Only the `RemotePlayerService` copy macOS holds the
  helper responsible for is tapped. Other apps' copies are never touched
  (a muted tap on one would silence that app); if the helper's copy cannot
  be identified, the system volume is used.
- **Resources.** The tap is built when playback starts; its audio thread stops
  while paused and everything is released when playback stops, when a play
  fails, or when the app quits. A new default output device or player
  process rebuilds it: the new tap is built first and the old one is
  released only once the new one plays.
- **Failures.** A failed rebuild (common in the middle of an output device
  change) is retried three times over about five seconds. The old muted
  tap is kept meanwhile, so after an output device change the music is
  silent rather than jumping to the system volume. When no tap mutes the
  music (the player moved to a new process, or no tap could be built yet)
  and the app volume is below full, the music is paused during the retries
  and resumes when one succeeds; at full app volume it keeps playing, as
  the system volume is then no louder. If it keeps failing, the rest of the
  session uses the system volume and the status line says `APP VOLUME OFF`.
  The system volume is never changed for you: if the music was playing
  quieter than the system volume, it is paused instead (or stays paused),
  and plays at the system volume (`SYS`) when you resume.
- **Opting out.** `NU11SIGNAL_VOLUME_MODE=system` in the environment keeps the
  system volume (no relaunch, no tap, no prompt).

## Spectrum

In app volume mode (macOS 15+), the tap's audio thread also copies each
buffer, before the app volume is applied (so the bars do not change with
the volume) and mixed to mono, into a lock-free ring buffer. A timer on a
utility queue, running only while that thread runs (playing in app mode),
reads the newest 2048 samples 15 times a second, applies a Hann window and
a vDSP FFT, sums the bins into 24 log-spaced bands from 40 Hz to 16 kHz,
maps each band's power from -50 dBFS (empty) to -10 dBFS (full), smooths
it (fast attack, a fall of about a second) and emits
`{"event":"levels","bands":[0-100, ...],"wave":[-100-100, ...]}`: `wave` is
the same 2048 samples decimated to 64 points, each keeping its stretch's
largest swing (so a transient survives), in hundredths of full scale, for
the rain's overall intensity (see [Rain](effects.md#rain)); a helper that omits it
still works. The TUI resamples the 24 bands to
the bars the panel has room for on its own 10 fps playing frame. When a
song starts or resumes in app mode the bars hold where they are (usually
flat) until the first reading arrives, since the tap takes a moment to
attach; a reading from before a pause, or an empty one, never counts. If
no reading arrives within 1.5 s of playing, or none arrived in the last
500 ms, the bars turn decorative, easing from their heights; when readings
come back the bars glide onto them over four frames. When paused the bars
fall to zero. Nothing is measured in system volume mode.
