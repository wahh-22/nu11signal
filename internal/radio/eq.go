package radio

// eqBands is the number of bars the equalizer tracks; narrow views show a
// prefix of them.
const eqBands = 48

// eq is the spectrum display. When the player measures what it plays (the
// helper in app-volume mode, see playback.LevelSource) the bars follow its
// readings; otherwise they are decorative, driven by playback state and a
// seeded pseudo-random walk. It is a value type: stepping returns a new eq
// and never shares memory.
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

// follow draws a spectrum reading on the first n bars, the ones the panel
// shows, resampled to them; the hidden bars are cleared. Pausing then
// lets step decay the bars from the reading.
func (e eq) follow(levels []float64, n int) eq {
	n = min(max(n, 0), eqBands)
	shown := resampleLevels(levels, n)
	for i := range e {
		e[i] = 0
		if i < n {
			e[i] = shown[i]
		}
	}
	return e
}

// ease moves every bar the share k of the way to target.
func (e eq) ease(target eq, k float64) eq {
	for i := range e {
		e[i] += (target[i] - e[i]) * k
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
