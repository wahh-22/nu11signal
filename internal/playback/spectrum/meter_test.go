package spectrum

import (
	"math"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

func tone(rate, frames int, hz float64) []float32 {
	s := make([]float32, frames)
	for i := range s {
		s[i] = float32(0.5 * math.Sin(2*math.Pi*hz*float64(i)/float64(rate)))
	}
	return s
}

// The reference is the local player's original ring/analyzer/smoother loop.
// Compare complete readings, including waveform and rounding, across resets,
// repeated ticks, low sample rates and ring wraparound.
func TestMeterMatchesLocalPipeline(t *testing.T) {
	for _, rate := range []int{8000, 44100, 48000} {
		t.Run(strconv.Itoa(rate), func(t *testing.T) {
			m := NewMeter(rate)
			r := newRing(2 * FFTSize)
			a, sm := newAnalyzer(rate), newSmoother(BandCount)
			for _, hz := range []float64{100, 1000, 3000} {
				m.Reset()
				r.reset()
				sm.reset()
				mono := tone(rate, 5*FFTSize+37, hz)
				stereo := make([]float32, 2*len(mono))
				for i, v := range mono {
					stereo[2*i], stereo[2*i+1] = v*0.5, v*1.5
				}
				// Multiple writes exercise chunk boundaries as well as wraparound.
				for start := 0; start < len(stereo); start += 314 {
					chunk := stereo[start:min(start+314, len(stereo))]
					m.WriteStereo(chunk)
					r.writeStereo(chunk)
				}
				for range 4 {
					window, _ := r.latest(FFTSize)
					want := playback.Spectrum{Bands: quantize(sm.update(a.levels(window))), Wave: quantize(decimate(window, WavePoints))}
					got, ok := m.Reading()
					if !ok || !reflect.DeepEqual(got, want) {
						t.Fatalf("%v Hz: reading = %+v, %v; want %+v", hz, got, ok, want)
					}
				}
			}
		})
	}
}

func TestMeterWaitsForCompleteWindowAndResets(t *testing.T) {
	m := NewMeter(48000)
	if _, ok := m.Reading(); ok {
		t.Fatal("empty meter has a reading")
	}
	samples := tone(48000, FFTSize, 1000)
	m.WriteMono(samples[:FFTSize-1])
	if _, ok := m.Reading(); ok {
		t.Fatal("partial window has a reading")
	}
	m.WriteMono(samples[FFTSize-1:])
	first, ok := m.Reading()
	if !ok || len(first.Bands) != 24 || len(first.Wave) != 64 || first.Bands[12] != 0.7 {
		t.Fatalf("first reading = %+v, %v", first, ok)
	}
	second, _ := m.Reading()
	if second.Bands[12] != 0.91 {
		t.Fatalf("second attack = %v, want 0.91", second.Bands[12])
	}
	m.WriteMono(make([]float32, FFTSize))
	quiet, _ := m.Reading()
	if quiet.Bands[12] != 0.73 {
		t.Fatalf("release = %v, want 0.73", quiet.Bands[12])
	}
	m.Reset()
	if _, ok := m.Reading(); ok {
		t.Fatal("reset meter retained its window")
	}
	m.WriteMono(samples)
	restarted, _ := m.Reading()
	if !reflect.DeepEqual(restarted, first) {
		t.Fatalf("reset retained smoothing: %+v, want %+v", restarted, first)
	}
	// Reading buffers belong to the caller, not the smoother.
	first.Bands[12], first.Wave[0] = 99, 99
	if next, _ := m.Reading(); next.Bands[12] != 0.91 || next.Wave[0] == 99 {
		t.Fatalf("caller mutated meter: %+v", next)
	}
}

func TestMeterStereoMeanMatchesMono(t *testing.T) {
	mono, stereo := NewMeter(48000), NewMeter(48000)
	samples := tone(48000, FFTSize, 1000)
	frames := make([]float32, 2*len(samples)+1)
	for i, v := range samples {
		frames[2*i], frames[2*i+1] = v*0.5, v*1.5
		samples[i] = (frames[2*i] + frames[2*i+1]) / 2
	}
	frames[len(frames)-1] = 100 // ignored incomplete frame
	stereo.WriteStereo(frames)
	mono.WriteMono(samples)
	a, _ := mono.Reading()
	b, _ := stereo.Reading()
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("stereo mean differs: %+v, want %+v", b, a)
	}
}

func TestSnapshotSurvivesWritesAndReset(t *testing.T) {
	m := NewMeter(48000)
	m.WriteMono(tone(48000, FFTSize, 1000))
	s, ok := m.Snapshot()
	if !ok {
		t.Fatal("no snapshot")
	}
	m.WriteMono(make([]float32, 5*FFTSize))
	m.Reset()
	got := m.Analyze(s)
	if got.Bands[12] != 0.7 {
		t.Fatalf("snapshot changed: %+v", got)
	}
	m.WriteMono(tone(48000, FFTSize, 1000))
	next, _ := m.Reading()
	if !reflect.DeepEqual(next, got) {
		t.Fatalf("reset after snapshot failed: %+v, want %+v", next, got)
	}
}

func TestMeterConcurrentCaptureAndRead(t *testing.T) {
	m := NewMeter(48000)
	samples := tone(48000, FFTSize, 1000)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 100 {
			m.WriteMono(samples)
			m.Reset()
		}
	}()
	go func() {
		defer wg.Done()
		for range 100 {
			m.Reading()
		}
	}()
	wg.Wait()
}

func TestReadingCadence(t *testing.T) {
	if ReadingRate != 15 || ReadingInterval != time.Second/15 {
		t.Fatalf("cadence = %v Hz, %v", ReadingRate, ReadingInterval)
	}
}
