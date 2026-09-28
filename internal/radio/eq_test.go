package radio

import "testing"

func TestEQAnimatesWhilePlaying(t *testing.T) {
	var e eq
	for frame := uint64(0); frame < 5; frame++ {
		e = e.step(true, 7, frame)
	}
	if e.flat() {
		t.Fatal("eq is flat after playing frames, want movement")
	}
}

func TestEQIsDeterministic(t *testing.T) {
	var a, b eq
	for frame := uint64(0); frame < 10; frame++ {
		a = a.step(true, 42, frame)
		b = b.step(true, 42, frame)
	}
	if a != b {
		t.Fatalf("same seed and frames produced different bars:\n%v\n%v", a, b)
	}
}

func TestEQDecaysToFlatWhenNotPlaying(t *testing.T) {
	var e eq
	for frame := uint64(0); frame < 10; frame++ {
		e = e.step(true, 1, frame)
	}
	peak := e.total()
	e = e.step(false, 1, 10)
	if e.total() >= peak {
		t.Fatalf("eq did not decay: %v -> %v", peak, e.total())
	}
	for frame := uint64(11); frame < 40; frame++ {
		e = e.step(false, 1, frame)
	}
	if !e.flat() {
		t.Fatalf("eq not flat after paused frames: %v", e)
	}
}

func TestEQRenderShape(t *testing.T) {
	var e eq
	for frame := uint64(0); frame < 5; frame++ {
		e = e.step(true, 3, frame)
	}
	rows := e.render(10, 3)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	for i, r := range rows {
		if n := len([]rune(r)); n != 10 {
			t.Errorf("row %d has %d cells, want 10: %q", i, n, r)
		}
	}
	if rows := e.render(0, 0); len(rows) != 0 {
		t.Errorf("zero size rendered %d rows", len(rows))
	}
	if rows := e.render(-1, -1); len(rows) != 0 {
		t.Errorf("negative size rendered %d rows", len(rows))
	}
}
