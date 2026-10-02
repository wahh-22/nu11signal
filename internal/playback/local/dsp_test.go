package local

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestAmplitudeIsSquareLaw(t *testing.T) {
	for _, c := range []struct{ level, want float64 }{{0, 0}, {0.5, 0.25}, {1, 1}, {2, 1}, {-1, 0}, {math.NaN(), 0}} {
		if got := amplitude(c.level); math.Abs(float64(got)-c.want) > 1e-6 {
			t.Errorf("amplitude(%v) = %v, want %v", c.level, got, c.want)
		}
	}
}

func TestRampMovesLinearlyToTarget(t *testing.T) {
	r := ramp{cur: 1, target: 0, step: rampStep(48000, 0.015)}
	var last float32 = 1
	for i := range 720 {
		g := r.next()
		if g > last {
			t.Fatalf("frame %d: gain rose from %v to %v", i, last, g)
		}
		if i == 359 && math.Abs(float64(g)-0.5) > 0.01 {
			t.Errorf("halfway gain = %v, want 0.5", g)
		}
		last = g
	}
	if r.cur != 0 || !r.done() {
		t.Errorf("after 15 ms gain = %v, want 0", r.cur)
	}
	r.target = 0.25
	for range 1000 {
		r.next()
	}
	if r.cur != 0.25 {
		t.Errorf("gain = %v, want 0.25 exactly", r.cur)
	}
	if s := rampStep(0, 1); s != 1 {
		t.Errorf("rampStep without a rate = %v, want a jump", s)
	}
}

func TestFFTMatchesDFT(t *testing.T) {
	const n = 16
	x := make([]complex128, n)
	for i := range x {
		x[i] = complex(math.Sin(float64(i)*0.7)+0.3*float64(i%3), 0)
	}
	want := make([]complex128, n)
	for k := range want {
		for i, v := range x {
			want[k] += v * cmplx.Exp(complex(0, -2*math.Pi*float64(k*i)/n))
		}
	}
	fft(x)
	for k := range x {
		if cmplx.Abs(x[k]-want[k]) > 1e-9 {
			t.Fatalf("bin %d = %v, want %v", k, x[k], want[k])
		}
	}
}

func TestBandRangesMatchHelper(t *testing.T) {
	r := bandRanges(48000, 2048, 24, 40, 16000)
	if len(r) != 24 {
		t.Fatalf("%d bands", len(r))
	}
	for i, b := range r {
		if b.lo < 1 || b.hi > 1024 || b.lo >= b.hi {
			t.Errorf("band %d = %+v", i, b)
		}
		if i > 0 && b.lo < r[i-1].lo {
			t.Errorf("band %d starts before band %d", i, i-1)
		}
	}
	// 40 Hz is under bin 2 (23.4 Hz bins): the lowest band reads one bin.
	if r[0].hi-r[0].lo != 1 {
		t.Errorf("lowest band = %+v, want a single bin", r[0])
	}
	if got := bandOf(1000, 24, 40, 16000); got != 12 {
		t.Errorf("1 kHz is band %d, want 12", got)
	}
}

func TestLevelScale(t *testing.T) {
	for _, c := range []struct{ power, want float64 }{{0, 0}, {1e-5, 0}, {1e-3, 0.5}, {0.1, 1}, {1, 1}, {math.Inf(1), 0}} {
		if got := normalizedLevel(c.power); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("normalizedLevel(%v) = %v, want %v", c.power, got, c.want)
		}
	}
}

func TestSmootherAttacksFastReleasesSlow(t *testing.T) {
	s := newSmoother(1)
	if got := s.update([]float64{1})[0]; math.Abs(got-0.7) > 1e-9 {
		t.Errorf("attack = %v, want 0.7", got)
	}
	if got := s.update([]float64{0})[0]; math.Abs(got-0.56) > 1e-9 {
		t.Errorf("release = %v, want 0.56", got)
	}
	s.levels[0] = 0.006
	if got := s.update([]float64{0})[0]; got != 0 {
		t.Errorf("tiny level = %v, want 0", got)
	}
}

func TestDecimateKeepsLargestSwing(t *testing.T) {
	s := make([]float32, 128)
	s[3] = -0.9
	s[5] = 0.5
	s[100] = 2
	w := decimate(s, 64)
	if len(w) != 64 || w[1] != float64(float32(-0.9)) || w[2] != 0.5 || w[50] != 1 {
		t.Errorf("wave = %v", w)
	}
	if w := decimate(s[:10], 64); len(w) != 64 || w[0] != 0 {
		t.Errorf("short input = %v, want 64 zeros", w)
	}
}

func TestAnalyzerPutsAToneInItsBand(t *testing.T) {
	const rate = 48000
	a := newAnalyzer(rate)
	window := make([]float32, fftSize)
	for i := range window {
		window[i] = float32(0.5 * math.Sin(2*math.Pi*1000*float64(i)/rate))
	}
	bands := a.levels(window)
	if len(bands) != numBands {
		t.Fatalf("%d bands", len(bands))
	}
	loudest := 0
	for i, b := range bands {
		if b > bands[loudest] {
			loudest = i
		}
	}
	if loudest != bandOf(1000, numBands, minHz, maxHz) {
		t.Errorf("loudest band %d, want %d: %v", loudest, bandOf(1000, numBands, minHz, maxHz), bands)
	}
	// A half-scale sine is -6 dBFS: above the ceiling, a full bar.
	if bands[loudest] != 1 {
		t.Errorf("tone band = %v, want 1", bands[loudest])
	}
	if bands[0] > 0.2 || bands[numBands-1] > 0.2 {
		t.Errorf("far bands leak: %v", bands)
	}
}

func TestRingKeepsTheNewestWindow(t *testing.T) {
	r := newRing(8)
	if _, ok := r.latest(4); ok {
		t.Fatal("an empty ring has no window")
	}
	r.writeStereo([]float32{1, 3, 2, 4, 3, 5}) // mono 2, 3, 4
	if _, ok := r.latest(4); ok {
		t.Fatal("3 samples are not a window of 4")
	}
	r.writeStereo([]float32{0, 10, 0, 12, 0, 14, 0, 16, 0, 18, 0, 20, 0, 22})
	w, ok := r.latest(4)
	if !ok || w[0] != 8 || w[3] != 11 {
		t.Errorf("latest = %v %v, want 8...11", w, ok)
	}
	r.reset()
	if _, ok := r.latest(1); ok {
		t.Error("reset ring still has samples")
	}
}
