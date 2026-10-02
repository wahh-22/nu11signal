package local

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"time"
)

// mp3Duration estimates an mp3's length from its first frame, without the
// full frame scan go-mp3 does when it opens a file: the frame count of a
// Xing/Info or VBRI header when there is one (VBR files and LAME's CBR
// files carry it), the size over the bitrate otherwise. Only MPEG layer III
// is understood, as only it is decoded.
func mp3Duration(path string) (time.Duration, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()
	var start int64
	head := make([]byte, 10)
	if _, err := io.ReadFull(f, head); err != nil {
		return 0, err
	}
	if string(head[:3]) == "ID3" {
		start = 10 + (int64(head[6]&0x7f)<<21 | int64(head[7]&0x7f)<<14 | int64(head[8]&0x7f)<<7 | int64(head[9]&0x7f))
		if head[5]&0x10 != 0 {
			start += 10 // footer
		}
	}
	buf := make([]byte, 64<<10)
	n, err := f.ReadAt(buf, start)
	if n == 0 {
		return 0, err
	}
	buf = buf[:n]
	for i := 0; i+4 <= len(buf); i++ {
		h, ok := parseMP3Header(buf[i:])
		if !ok {
			continue
		}
		frame := buf[i:]
		side := 17 // side information after the 4-byte header
		switch {
		case h.mpeg1 && !h.mono:
			side = 32
		case !h.mpeg1 && h.mono:
			side = 9
		}
		if x := 4 + side; len(frame) >= x+12 {
			if tag := string(frame[x : x+4]); (tag == "Xing" || tag == "Info") && frame[x+7]&1 != 0 {
				return h.duration(int64(binary.BigEndian.Uint32(frame[x+8:]))), nil
			}
		}
		if len(frame) >= 36+18 && string(frame[36:40]) == "VBRI" {
			return h.duration(int64(binary.BigEndian.Uint32(frame[36+14:]))), nil
		}
		audio := size - start - int64(i)
		tail := make([]byte, 3)
		if _, err := f.ReadAt(tail, size-128); err == nil && string(tail) == "TAG" {
			audio -= 128
		}
		return time.Duration(audio * 8 * int64(time.Second) / int64(h.bitrate)), nil
	}
	return 0, errors.New("local: no mp3 frame found")
}

// mp3Header is what mp3Duration needs from a frame header.
type mp3Header struct {
	mpeg1   bool
	mono    bool
	bitrate int // bits per second
	rate    int // Hz
}

// duration is the length of frames frames.
func (h mp3Header) duration(frames int64) time.Duration {
	spf := int64(576)
	if h.mpeg1 {
		spf = 1152
	}
	return time.Duration(frames * spf * int64(time.Second) / int64(h.rate))
}

// parseMP3Header reads a layer III frame header at b; false when b holds
// no valid one.
func parseMP3Header(b []byte) (mp3Header, bool) {
	if b[0] != 0xff || b[1]&0xe0 != 0xe0 {
		return mp3Header{}, false
	}
	version := b[1] >> 3 & 3 // 3 MPEG-1, 2 MPEG-2, 0 MPEG-2.5
	layer := b[1] >> 1 & 3   // 1 layer III
	bri, sri := int(b[2]>>4), int(b[2]>>2&3)
	if version == 1 || layer != 1 || bri == 0 || bri == 15 || sri == 3 {
		return mp3Header{}, false
	}
	v1 := [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	v2 := [...]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	rates := [...]int{44100, 48000, 32000}
	h := mp3Header{mpeg1: version == 3, mono: b[3]>>6 == 3, rate: rates[sri]}
	switch version {
	case 3:
		h.bitrate = v1[bri] * 1000
	case 2:
		h.bitrate, h.rate = v2[bri]*1000, h.rate/2
	default:
		h.bitrate, h.rate = v2[bri]*1000, h.rate/4
	}
	return h, true
}
