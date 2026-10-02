package local

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// wavFormat selects the sample encoding writeWAV uses.
type wavFormat int

const (
	wavPCM16 wavFormat = iota
	wavPCM24
	wavFloat32
	wavPCM8
)

// writeWAV writes frames frames of channels channels at rate to path; gen
// returns the sample of a frame and channel, -1...1.
func writeWAV(t *testing.T, path string, rate, channels, frames int, format wavFormat, gen func(frame, ch int) float64) {
	t.Helper()
	var data bytes.Buffer
	bits, code := 16, uint16(1)
	switch format {
	case wavPCM24:
		bits = 24
	case wavFloat32:
		bits, code = 32, 3
	case wavPCM8:
		bits = 8
	}
	for f := range frames {
		for c := range channels {
			v := max(min(gen(f, c), 1), -1)
			switch format {
			case wavPCM16:
				_ = binary.Write(&data, binary.LittleEndian, int16(math.Round(v*32767)))
			case wavPCM24:
				s := int32(math.Round(v * 8388607))
				data.Write([]byte{byte(s), byte(s >> 8), byte(s >> 16)})
			case wavFloat32:
				_ = binary.Write(&data, binary.LittleEndian, float32(v))
			case wavPCM8:
				data.WriteByte(byte(math.Round(v*127) + 128))
			}
		}
	}
	block := channels * bits / 8
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(4+8+16+8+data.Len()))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), code, uint16(channels), uint32(rate), uint32(rate * block), uint16(block), uint16(bits)} {
		_ = binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(data.Len()))
	b.Write(data.Bytes())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sine is a generator of a tone at hz and amplitude amp at rate.
func sine(hz, amp float64, rate int) func(frame, ch int) float64 {
	return func(f, _ int) float64 { return amp * math.Sin(2*math.Pi*hz*float64(f)/float64(rate)) }
}

// retagMP3 replaces the ID3v2 tag of an mp3 with one holding only a title
// and a track number (ID3v2.3, text frames in ISO-8859-1).
func retagMP3(t *testing.T, mp3 []byte, title string, track string) []byte {
	t.Helper()
	if !bytes.HasPrefix(mp3, []byte("ID3")) {
		t.Fatal("fixture has no ID3v2 tag")
	}
	size := int(mp3[6])<<21 | int(mp3[7])<<14 | int(mp3[8])<<7 | int(mp3[9])
	audio := mp3[10+size:]
	var frames bytes.Buffer
	for _, f := range [][2]string{{"TIT2", title}, {"TRCK", track}} {
		frames.WriteString(f[0])
		_ = binary.Write(&frames, binary.BigEndian, uint32(1+len(f[1])))
		frames.Write([]byte{0, 0, 0})
		frames.WriteString(f[1])
	}
	n := frames.Len()
	out := []byte{'I', 'D', '3', 3, 0, 0, byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
	out = append(out, frames.Bytes()...)
	return append(out, audio...)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeSink is a sink without a device: the test pulls the rendered audio.
type fakeSink struct {
	mu      sync.Mutex
	src     func([]float32)
	rate    int
	starts  int
	closes  int
	failure error
}

func (f *fakeSink) start(rate int, src func([]float32)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failure != nil {
		return f.failure
	}
	f.starts++
	f.rate, f.src = rate, src
	return nil
}

func (f *fakeSink) close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	return nil
}

// pull renders frames stereo frames, as the device would ask for them, in
// blocks of 256; nothing renders before start.
func (f *fakeSink) pull(frames int) []float32 {
	f.mu.Lock()
	src := f.src
	f.mu.Unlock()
	out := make([]float32, 0, 2*frames)
	for frames > 0 {
		n := min(frames, 256)
		buf := make([]float32, 2*n)
		if src != nil {
			src(buf)
		}
		out = append(out, buf...)
		frames -= n
	}
	return out
}

func (f *fakeSink) counts() (starts, closes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts, f.closes
}

// peak is the largest absolute sample of interleaved audio.
func peak(s []float32) float64 {
	var p float64
	for _, v := range s {
		p = max(p, math.Abs(float64(v)))
	}
	return p
}
