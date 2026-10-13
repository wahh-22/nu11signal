package local

import (
	"math"
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
