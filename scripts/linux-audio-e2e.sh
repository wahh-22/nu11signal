#!/usr/bin/env bash
# Check pure-Go monitor capture and shared DSP with isolated Linux test tones.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
docker="${DOCKER:-/usr/local/bin/docker}"
if [[ ! -x "$docker" ]]; then docker=docker; fi
arch="$($docker info --format '{{.Architecture}}')"
case "$arch" in aarch64|arm64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) echo "FAIL: unsupported Docker architecture $arch" >&2; exit 1 ;; esac
# Keep the generated binary inside the probe's edit/build surface.
mkdir -p "$root/tools/pulseprobe/.e2e"
(cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o tools/pulseprobe/.e2e/pulseprobe ./tools/pulseprobe)
"$docker" run --rm -i --platform "linux/$arch" \
  -v "$root/tools/pulseprobe/.e2e/pulseprobe:/usr/local/bin/pulseprobe:ro" debian:stable-slim bash -s <<'LINUX'
set -euo pipefail
trap 'echo "FAIL: Linux audio end-to-end check failed (line $LINENO)" >&2' ERR
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq pulseaudio pulseaudio-utils sox >/dev/null
# Run a per-user daemon, not PulseAudio system mode, even in this container.
mkdir -p /home/probe
useradd -d /home/probe probe
chown probe:probe /home/probe
runuser -u probe -- bash -s <<'PROBE'
set -euo pipefail
export HOME=/home/probe
export XDG_RUNTIME_DIR=/home/probe/runtime
mkdir -m 700 -p "$XDG_RUNTIME_DIR"
export PULSE_SERVER="unix:$XDG_RUNTIME_DIR/pulse/native"
pulseaudio -D --exit-idle-time=-1 --system=false
pactl load-module module-null-sink sink_name=probe rate=48000 >/dev/null
pactl set-default-sink probe
pactl set-sink-volume probe 100%
for hz in 1000 100; do
  sox -n -r 48000 -c 1 -b 16 -t wav - synth 7 sine "$hz" vol 0.25 | paplay &
  tone=$!
  sleep 1
  pulseprobe -seconds 3 > "/home/probe/$hz.log"
  wait "$tone"
done
sleep 1
pulseprobe -seconds 3 > /home/probe/silence.log
awk '
/^summary/ {
  print FILENAME ": " $0
  for (i=1; i<=NF; i++) {
    split($i, kv, "=")
    if (kv[1]=="max_band") band=kv[2]+0
    if (kv[1]=="max") peak=kv[2]+0
    if (kv[1]=="bands") split(kv[2], levels, ",")
  }
  if (FILENAME ~ /1000.log$/) { high=band; highPeak=peak; for(i=1;i<=24;i++) hi[i]=levels[i] }
  else if (FILENAME ~ /100.log$/) { low=band; lowPeak=peak; for(i=1;i<=24;i++) lo[i]=levels[i] }
  else { silent=peak; silenceSeen=1 }
  count++
}
END {
  if(count!=3 || !silenceSeen || highPeak<0.2 || lowPeak<0.2 || high<=low || hi[high+1]<=hi[low+1] || lo[low+1]<=lo[high+1] || silent>0.02) {
    print "FAIL: tone bands must move down from 1 kHz to 100 Hz, and silence must be <=0.02" > "/dev/stderr"; exit 1
  }
  print "PASS: 1 kHz band=" high ", 100 Hz band=" low ", silence max=" silent
}' /home/probe/1000.log /home/probe/100.log /home/probe/silence.log
PROBE
LINUX
