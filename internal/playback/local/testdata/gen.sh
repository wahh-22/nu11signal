#!/bin/sh
# Regenerates the decoder fixtures: 0.5 s of a 440 Hz sine at half scale,
# mono, 22050 Hz, tagged. Made with ffmpeg 8 (libmp3lame, flac, libvorbis);
# other encoder versions produce different bytes but equivalent audio.
# The mp3 is 64 kb/s: go-mp3 v0.3.4 glitches on this tone at 32 kb/s
# (MPEG-2 at 22050 Hz), which ffmpeg decodes cleanly.
set -e
cd "$(dirname "$0")"
src="aevalsrc=0.5*sin(2*PI*440*t):s=22050:d=0.5"
tags="-metadata title=Sine -metadata artist=Fixture -metadata album=Tones -metadata track=3"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "$src" -ac 1 $tags -c:a libmp3lame -b:a 64k sine.mp3
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "$src" -ac 1 $tags -c:a flac -sample_fmt s16 sine.flac
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "$src" -ac 1 $tags -c:a libvorbis -q:a 0 sine.ogg
