package local

import (
	"errors"
	"io"
	"math"
	"path/filepath"
	"testing"
)

// readAll decodes the whole stream, stereo interleaved.
func readAll(t *testing.T, r interface {
	read([]float32) (int, error)
}) []float32 {
	t.Helper()
	var out []float32
	buf := make([]float32, 2*300)
	for {
		n, err := r.read(buf)
		out = append(out, buf[:2*n]...)
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("read returned no frames and no error")
		}
	}
}

func TestWAVFormats(t *testing.T) {
	gen := func(f, ch int) float64 { return []float64{0.5, -0.25}[ch] * float64(f%2*2-1) }
	for name, format := range map[string]wavFormat{"pcm8": wavPCM8, "pcm16": wavPCM16, "pcm24": wavPCM24, "float32": wavFloat32} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "x.wav")
			writeWAV(t, path, 11025, 2, 100, format, gen)
			d, err := openDecoder(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.close()
			if d.rate() != 11025 || d.length() != 100 {
				t.Fatalf("rate %d length %d, want 11025 and 100", d.rate(), d.length())
			}
			s := readAll(t, d)
			if len(s) != 200 {
				t.Fatalf("got %d samples, want 200", len(s))
			}
			tol := 1e-3
			if format == wavPCM8 {
				tol = 1e-2
			}
			for f := range 100 {
				for c := range 2 {
					if math.Abs(float64(s[2*f+c])-gen(f, c)) > tol {
						t.Fatalf("frame %d ch %d = %v, want %v", f, c, s[2*f+c], gen(f, c))
					}
				}
			}
		})
	}
}

func TestWAVMonoIsCopiedToBothChannelsAndSeeks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.wav")
	writeWAV(t, path, 8000, 1, 1000, wavPCM16, func(f, _ int) float64 { return float64(f) / 2000 })
	d, err := openDecoder(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	if err := d.seek(600); err != nil {
		t.Fatal(err)
	}
	s := readAll(t, d)
	if len(s) != 800 {
		t.Fatalf("after seek got %d frames, want 400", len(s)/2)
	}
	if math.Abs(float64(s[0])-0.3) > 1e-3 || s[0] != s[1] {
		t.Errorf("first frame after seek = %v,%v, want 0.3 on both channels", s[0], s[1])
	}
}

func TestResampleSine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.wav")
	const inRate, outRate, frames = 22050, 48000, 22050
	writeWAV(t, path, inRate, 1, frames, wavPCM16, sine(440, 0.5, inRate))
	d, err := openDecoder(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	r := newResampler(d, outRate)
	s := readAll(t, r)
	if got, want := len(s)/2, frames*outRate/inRate; got < want-2 || got > want+2 {
		t.Fatalf("got %d frames, want %d", got, want)
	}
	var worst float64
	// Frames past the last input frame hold it; they are not interpolated.
	for j := 0; float64(j)*inRate/outRate <= frames-1; j++ {
		want := 0.5 * math.Sin(2*math.Pi*440*float64(j)/outRate)
		worst = max(worst, math.Abs(float64(s[2*j])-want))
		if s[2*j] != s[2*j+1] {
			t.Fatalf("frame %d: channels differ", j)
		}
	}
	if worst > 0.005 {
		t.Errorf("worst error %v, want under 0.005", worst)
	}
}

func TestResampleDownAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.wav")
	writeWAV(t, path, 48000, 1, 4800, wavPCM16, func(f, _ int) float64 { return float64(f) / 9600 })
	d, err := openDecoder(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	r := newResampler(d, 8000)
	s := readAll(t, r)
	if len(s)/2 < 799 || len(s)/2 > 801 {
		t.Fatalf("got %d frames, want 800", len(s)/2)
	}
	// A ramp resamples to a ramp: frame j is input frame 6j.
	if math.Abs(float64(s[2*400])-float64(6*400)/9600) > 1e-3 {
		t.Errorf("frame 400 = %v", s[800])
	}
	if err := d.seek(2400); err != nil {
		t.Fatal(err)
	}
	r.reset()
	s = readAll(t, r)
	if len(s)/2 < 399 || len(s)/2 > 401 || math.Abs(float64(s[0])-0.25) > 1e-3 {
		t.Errorf("after reset: %d frames from %v, want 400 from 0.25", len(s)/2, s[0])
	}
}

// zeroCrossings counts sign changes of the left channel.
func zeroCrossings(s []float32) int {
	n := 0
	for i := 2; i < len(s); i += 2 {
		if (s[i-2] < 0) != (s[i] < 0) {
			n++
		}
	}
	return n
}

func TestCompressedFixturesDecodeAndSeek(t *testing.T) {
	for _, name := range []string{"sine.mp3", "sine.flac", "sine.ogg"} {
		t.Run(name, func(t *testing.T) {
			d, err := openDecoder(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			defer d.close()
			if d.rate() != 22050 {
				t.Fatalf("rate = %d, want 22050", d.rate())
			}
			s := readAll(t, d)
			frames := len(s) / 2
			if frames < 11025 || frames > 13500 {
				t.Fatalf("decoded %d frames, want about 11025", frames)
			}
			if l := d.length(); l != 0 && (l < 11025 || l > 13500) {
				t.Errorf("length = %d", l)
			}
			// 0.2 s from the middle: a 440 Hz tone crosses zero 176 times.
			mid := s[2*4410 : 2*(4410+4410)]
			if z := zeroCrossings(mid); z < 170 || z > 182 {
				t.Errorf("%d zero crossings, want about 176", z)
			}
			if p := peak(mid); p < 0.4 || p > 0.6 {
				t.Errorf("peak %v, want about 0.5", p)
			}
			if err := d.seek(5512); err != nil {
				t.Fatal(err)
			}
			rest := readAll(t, d)
			if want := frames - 5512; len(rest)/2 < want-1200 || len(rest)/2 > want+1200 {
				t.Errorf("after seek %d frames, want about %d", len(rest)/2, want)
			}
		})
	}
}

func TestOpenDecoderRejectsUnknownAndBroken(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.txt"), []byte("text"))
	writeFile(t, filepath.Join(dir, "x.wav"), []byte("RIFF....WAVEjunk"))
	for _, n := range []string{"x.txt", "x.wav", "missing.flac"} {
		if d, err := openDecoder(filepath.Join(dir, n)); err == nil {
			d.close()
			t.Errorf("%s: want an error", n)
		}
	}
}
