# Android TV startup
Show the boot splash for the whole Apple Music load and remove the sluggish first seconds after a cold start.
Branch feat/tv-startup from main 81e95c4; delivery as a PR and a later TV release.

## Specs
1. S1 — User: "esta parte de espera, debe mostrar el booting de cuando esta iniciando la app, no esta parte" (screenshot: only "nu11signal: loading Apple Music, this can take a minute on a TV..." on a black terminal).
2. S2 — User: "cuando se inicia la app luego de cerrarla completamente, dura varios segundos muy lenta que no responde, luego ya es bastante fluida".
3. S3 — User chose to accept the Android installer's final "Se instaló la app" screen with the generic icon ("Aceptarla tal cual"); no change to the installer mechanism.

## Tasks
- [ ] T1 — S1: From the first terminal frame until the web player is ready, show the TUI boot splash (emblem + BOOTING NU11SIGNAL...) instead of the Java loading line; then hand off to the normal UI. Route: inline, test-first. Commit: pending.
- [ ] T2 — S2: Stop the hidden Apple Music page from decoding video and flooding the log during startup, without affecting MusicKit audio; measure the cold start before/after on the TV. Route: inline + device measurement. Commit: pending.
- [ ] T3 — S1–S2: Independent verification, PR, CI, merge; TV release after user confirmation. Route: verifier + parent delivery. Commit: pending.

## Log
- L1: User request with two screenshots: installer final screen with the generic icon; the loading line instead of the boot animation; slow first seconds after a cold start.
- L2: Installer: the confirmation dialog for the 0.0.7 APK shows the logo (TV capture, cancelled, nothing installed). Only Android's final screen shows the generic icon. User decision S3: accept it.
- L3: Startup map (explorer): MainActivity writes the loading line after starting Go (MainActivity.java:550); Go's play() opens the web player and waits for MusicKit (webplayer readiness, up to 2 minutes) before radio.New (cmd/nu11signal/main.go:205-219, internal/androidtv/runtime.go:285-294). The boot splash (internal/radio/boot.go) lasts 1.5 s from the first size and needs a live Model; no radio Model exists during the wait. radio.View uses the alternate screen.
- L4: Cold start measured on the TV (0.0.7, force-stop, session kept): loading line 0–5 s, boot splash ~6 s, UI ~8 s; 4-core CPU nearly saturated until ~26 s (6–32 % idle), then ~50 % idle. At ~22 s Chromium creates a video/hevc decoder (c2.mtk.hevc.decoder, 320x240) in the hidden music.apple.com WebView with no music playing; 138 "eventQueue overflow" MusicKit console warnings in 40 s, each forwarded to logcat. The slowness is after the UI appears.
