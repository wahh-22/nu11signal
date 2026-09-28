package radio

import "strings"

// eqBands is the number of bars the equalizer tracks; narrow views show a
// prefix of them.
const eqBands = 48

// eq is the decorative spectrum display. MusicKit exposes no audio samples,
// so the bars are driven by playback state and a seeded pseudo-random walk.
// It is a value type: stepping returns a new eq and never shares memory.
type eq [eqBands]float64

// eqDecay is the per-frame multiplier applied to every bar when not playing.
const eqDecay = 0.6

// step advances the bars by one animation frame. While playing, each band
// eases toward a pseudo-random target derived from (seed, frame, band); the
// result is fully deterministic. Otherwise the bars decay to flat.
func (e eq) step(playing bool, seed, frame uint64) eq {
	for i := range e {
		if !playing {
			e[i] *= eqDecay
			if e[i] < 0.02 {
				e[i] = 0
			}
			continue
		}
		r := unit(mix(seed, frame, uint64(i)))
		// A gentle envelope makes low bands louder, like a real mix.
		target := 0.1 + 0.85*r*(1-0.45*float64(i)/eqBands)
		e[i] += (target - e[i]) * 0.55
	}
	return e
}

func (e eq) flat() bool { return e.total() == 0 }

func (e eq) total() float64 {
	var sum float64
	for _, v := range e {
		sum += v
	}
	return sum
}

var eqGlyphs = []rune(" ▁▂▃▄▅▆▇█")

// render draws the bands as height rows of block glyphs, top row first,
// one band every other column so the bars read as a spectrum. Every row is
// exactly width cells; bands beyond eqBands are left blank.
func (e eq) render(width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	steps := len(eqGlyphs) - 1
	rows := make([]string, height)
	for r := range height {
		var b strings.Builder
		floor := (height - 1 - r) * steps // eighths below this row
		for c := range width {
			band := c / 2
			if c%2 == 1 || band >= eqBands {
				b.WriteByte(' ')
				continue
			}
			level := int(e[band]*float64(height*steps) + 0.5)
			fill := min(max(level-floor, 0), steps)
			b.WriteRune(eqGlyphs[fill])
		}
		rows[r] = b.String()
	}
	return rows
}

// mix hashes its inputs with SplitMix64 finalization rounds.
func mix(vals ...uint64) uint64 {
	h := uint64(0x9E3779B97F4A7C15)
	for _, v := range vals {
		h ^= v + 0x9E3779B97F4A7C15 + (h << 6) + (h >> 2)
		h ^= h >> 30
		h *= 0xBF58476D1CE4E5B9
		h ^= h >> 27
		h *= 0x94D049BB133111EB
		h ^= h >> 31
	}
	return h
}

// unit maps a hash to [0, 1).
func unit(h uint64) float64 { return float64(h>>11) / (1 << 53) }

// glitchGlyphs replace characters of a title while it glitches.
var glitchGlyphs = []rune("░▒▓█▞▚/\\#%&@$01<>")

// glitchText substitutes some characters of s for noise glyphs; frames is
// the remaining glitch intensity (0 returns s unchanged). Deterministic for
// a given (s, frames, seed); spaces are preserved to keep word shapes.
func glitchText(s string, frames int, seed uint64) string {
	if frames <= 0 {
		return s
	}
	runes := []rune(s)
	changed := false
	threshold := 0.12 * float64(frames)
	for i, r := range runes {
		if r == ' ' {
			continue
		}
		h := mix(seed, uint64(frames), uint64(i))
		if unit(h) < threshold {
			runes[i] = glitchGlyphs[h%uint64(len(glitchGlyphs))]
			changed = true
		}
	}
	if !changed {
		// Guarantee a visible flicker on short titles.
		for i, r := range runes {
			if r != ' ' {
				runes[i] = glitchGlyphs[mix(seed, uint64(frames))%uint64(len(glitchGlyphs))]
				break
			}
		}
	}
	return string(runes)
}
