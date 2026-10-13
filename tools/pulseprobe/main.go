// pulseprobe checks the Linux default sink monitor through the shared DSP.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback/pulsecapture"
	"github.com/wahh-22/nu11signal/internal/playback/spectrum"
)

func main() {
	seconds := flag.Float64("seconds", 3, "capture duration in seconds")
	flag.Parse()
	if *seconds <= 0 {
		fmt.Fprintln(os.Stderr, "pulseprobe: seconds must be positive")
		os.Exit(1)
	}
	if err := run(time.Duration(*seconds * float64(time.Second))); err != nil {
		fmt.Fprintln(os.Stderr, "pulseprobe:", err)
		os.Exit(1)
	}
}

func run(duration time.Duration) error {
	meter := spectrum.NewMeter(48000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := pulsecapture.Start(ctx, func(samples []float32, _ int) { meter.WriteMono(samples) })
	if err != nil {
		return err
	}
	defer stop()
	ticker := time.NewTicker(spectrum.ReadingInterval)
	defer ticker.Stop()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	sums := make([]float64, spectrum.BandCount)
	readings := 0
	for {
		select {
		case <-ticker.C:
			reading, ok := meter.Reading()
			if !ok {
				continue
			}
			readings++
			fmt.Print("bands=")
			for i, level := range reading.Bands {
				if i > 0 {
					fmt.Print(",")
				}
				fmt.Printf("%.2f", level)
				sums[i] += level
			}
			fmt.Println()
		case <-timer.C:
			if readings == 0 {
				return fmt.Errorf("no monitor samples received")
			}
			maxIndex, maxValue, mean := 0, 0.0, 0.0
			for i := range sums {
				sums[i] /= float64(readings)
				mean += sums[i] / float64(len(sums))
				if sums[i] > maxValue {
					maxIndex, maxValue = i, sums[i]
				}
			}
			fmt.Printf("summary readings=%d max_band=%d max=%.3f mean=%.3f bands=", readings, maxIndex, maxValue, mean)
			for i, value := range sums {
				if i > 0 {
					fmt.Print(",")
				}
				fmt.Printf("%.3f", value)
			}
			fmt.Println()
			return nil
		}
	}
}
