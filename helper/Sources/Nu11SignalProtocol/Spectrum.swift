import Accelerate
import Foundation
import Synchronization

// Spectrum levels for the TUI's equalizer: in app-volume mode the helper
// renders the music itself (see AppVolume), so it can measure it. The
// IOProc copies each buffer, mixed to mono, into a SampleRing; a timer off
// the real-time thread reads the newest window, runs an FFT
// (SpectrumAnalyzer), smooths the bands (LevelSmoother) and emits them as a
// `levels` event: {"event":"levels","bands":[0...100, ...]}.

/// The shape of the `levels` event.
public enum LevelsEvent {
    public static let name = "levels"
    /// Bands per event; the TUI resamples them to the bars it shows.
    public static let bands = 24
    public static let minHz = 40.0
    public static let maxHz = 16_000.0
    /// Samples per FFT: 43-46 ms at 44.1-48 kHz, 21-23 Hz per bin.
    public static let fftSize = 2048
    /// Seconds between events (15 Hz): faster than the TUI's 10 fps
    /// playing tick, so every frame it draws has a fresh reading.
    public static let interval = 1.0 / 15
}

/// Log-spaced bands between minHz and maxHz, as FFT bin ranges.
public enum SpectrumBands {
    /// The bands+1 band edges in Hz, geometrically spaced.
    public static func edges(bands: Int, minHz: Double, maxHz: Double) -> [Double] {
        let ratio = maxHz / minHz
        return (0...bands).map { minHz * pow(ratio, Double($0) / Double(bands)) }
    }

    /// The band a frequency falls in; nil outside minHz..<maxHz.
    public static func band(of hz: Double, bands: Int, minHz: Double, maxHz: Double) -> Int? {
        guard hz >= minHz, hz < maxHz else { return nil }
        return min(Int(log(hz / minHz) / log(maxHz / minHz) * Double(bands)), bands - 1)
    }

    /// The FFT bins each band sums: the bins whose center frequency falls
    /// inside it. A band narrower than a bin (the lowest ones) reads the
    /// single bin nearest its geometric center, so no band is ever empty.
    /// Bin 0 (DC) and fftSize/2 (Nyquist) are never used.
    public static func binRanges(sampleRate: Double, fftSize: Int, bands: Int, minHz: Double, maxHz: Double) -> [Range<Int>] {
        let binHz = sampleRate / Double(fftSize)
        let lastBin = fftSize / 2 - 1
        let edges = edges(bands: bands, minHz: minHz, maxHz: maxHz)
        return (0..<bands).map { band in
            let lower = max(Int((edges[band] / binHz).rounded(.up)), 1)
            let upper = min(Int((edges[band + 1] / binHz).rounded(.up)), lastBin + 1)
            if lower < upper { return lower..<upper }
            let center = (sqrt(edges[band] * edges[band + 1]) / binHz).rounded()
            let bin = min(max(Int(center), 1), lastBin)
            return bin..<(bin + 1)
        }
    }
}

public enum HannWindow {
    /// The periodic Hann window: its first coefficient is 0, the middle one 1.
    public static func coefficients(_ count: Int) -> [Float] {
        (0..<count).map { Float(0.5 - 0.5 * cos(2 * .pi * Double($0) / Double(count))) }
    }
}

/// Band power in dBFS mapped to 0...1 bars.
public enum LevelScale {
    /// The power that draws an empty bar, and the one that fills it.
    /// Measured on current pop masters, the smoothed bands sit around -28
    /// dBFS (bass peaks near -10, the top band near -40), so a 40 dB range
    /// keeps the bars mid-height with room to move both ways.
    public static let floorDB = -50.0
    public static let ceilingDB = -10.0

    /// A band's power (the sum of its bins' squared magnitudes, where a
    /// full-scale sine peaks at 1) as a 0...1 level, linear in decibels.
    public static func normalized(power: Double) -> Double {
        guard power > 0, power.isFinite else { return 0 }
        let db = 10 * log10(power)
        return min(max((db - floorDB) / (ceilingDB - floorDB), 0), 1)
    }

    /// Levels as the integer percentages the event carries.
    public static func quantized(_ levels: [Double]) -> [Int] {
        levels.map { level in
            guard level.isFinite else { return 0 }
            return Int((min(max(level, 0), 1) * 100).rounded())
        }
    }
}

/// Fast attack, slow release: a bar jumps up with a beat and falls back
/// over about a second, as analog meters do.
public struct LevelSmoother {
    /// The share of a rise taken each update.
    public let attack: Double
    /// The share of a fall taken each update.
    public let release: Double
    private var levels: [Double]

    public init(count: Int, attack: Double = 0.7, release: Double = 0.2) {
        self.attack = attack
        self.release = release
        levels = [Double](repeating: 0, count: count)
    }

    /// Moves every level toward its new reading and returns them all;
    /// levels below 0.005 (under half a percent) snap to 0.
    public mutating func update(_ readings: [Double]) -> [Double] {
        for i in levels.indices {
            let reading = i < readings.count ? readings[i] : 0
            let rate = reading > levels[i] ? attack : release
            levels[i] += (reading - levels[i]) * rate
            if levels[i] < 0.005 { levels[i] = 0 }
        }
        return levels
    }

    public mutating func reset() {
        levels = [Double](repeating: 0, count: levels.count)
    }
}

/// Windowed real FFT of the newest samples, grouped into log-spaced bands
/// and scaled to 0...1. It keeps its buffers between calls; use it from one
/// thread at a time.
public final class SpectrumAnalyzer {
    public let fftSize: Int
    public let ranges: [Range<Int>]
    private let log2n: vDSP_Length
    private let setup: FFTSetup
    private let window: [Float]
    private var windowed: [Float]
    private var real: [Float]
    private var imaginary: [Float]
    private var power: [Float]

    public init(fftSize: Int = LevelsEvent.fftSize, sampleRate: Double, bands: Int = LevelsEvent.bands,
                minHz: Double = LevelsEvent.minHz, maxHz: Double = LevelsEvent.maxHz) {
        precondition(fftSize >= 16 && fftSize & (fftSize - 1) == 0, "the FFT size must be a power of two")
        self.fftSize = fftSize
        log2n = vDSP_Length(fftSize.trailingZeroBitCount)
        guard let setup = vDSP_create_fftsetup(log2n, FFTRadix(kFFTRadix2)) else {
            fatalError("vDSP_create_fftsetup failed for \(fftSize) points")
        }
        self.setup = setup
        ranges = SpectrumBands.binRanges(sampleRate: sampleRate, fftSize: fftSize, bands: bands, minHz: minHz, maxHz: maxHz)
        window = HannWindow.coefficients(fftSize)
        windowed = [Float](repeating: 0, count: fftSize)
        real = [Float](repeating: 0, count: fftSize / 2)
        imaginary = [Float](repeating: 0, count: fftSize / 2)
        power = [Float](repeating: 0, count: fftSize / 2)
    }

    deinit {
        vDSP_destroy_fftsetup(setup)
    }

    /// The band levels of fftSize samples.
    public func levels(_ samples: [Float]) -> [Double] {
        precondition(samples.count >= fftSize, "need \(fftSize) samples")
        return samples.withUnsafeBufferPointer { levels($0.baseAddress!) }
    }

    /// The band levels of the fftSize samples at `samples`.
    public func levels(_ samples: UnsafePointer<Float>) -> [Double] {
        let half = fftSize / 2
        vDSP_vmul(samples, 1, window, 1, &windowed, 1, vDSP_Length(fftSize))
        real.withUnsafeMutableBufferPointer { realPart in
            imaginary.withUnsafeMutableBufferPointer { imaginaryPart in
                var split = DSPSplitComplex(realp: realPart.baseAddress!, imagp: imaginaryPart.baseAddress!)
                windowed.withUnsafeBytes { bytes in
                    vDSP_ctoz(bytes.bindMemory(to: DSPComplex.self).baseAddress!, 2, &split, 1, vDSP_Length(half))
                }
                vDSP_fft_zrip(setup, &split, 1, log2n, FFTDirection(FFT_FORWARD))
                vDSP_zvmags(&split, 1, &power, 1, vDSP_Length(half))
            }
        }
        // zrip returns twice the DFT and the Hann window halves a tone, so
        // a sine of amplitude A peaks at A * fftSize / 2: dividing the
        // squared magnitudes by (fftSize / 2)^2 makes a full-scale sine 1.
        let scale = 1 / (Double(half) * Double(half))
        return ranges.map { range in
            var sum = 0.0
            for bin in range { sum += Double(power[bin]) }
            return LevelScale.normalized(power: sum * scale)
        }
    }
}

/// A single-producer, single-consumer ring of mono samples between the
/// real-time IOProc and the analyzer's timer. The producer never blocks,
/// allocates or locks: it stores the samples, then publishes the new total
/// with a release store; the consumer reads the newest window after an
/// acquire load and discards it if the producer lapped it meanwhile.
@available(macOS 15.0, *)
public final class SampleRing: @unchecked Sendable {
    public let capacity: Int
    private let mask: Int
    private let storage: UnsafeMutablePointer<Float>
    /// Samples written since the start (or the last reset).
    private let written = Atomic<Int>(0)

    /// capacity is rounded up to a power of two.
    public init(capacity: Int) {
        var size = 1
        while size < capacity { size <<= 1 }
        self.capacity = size
        mask = size - 1
        storage = UnsafeMutablePointer<Float>.allocate(capacity: size)
        storage.initialize(repeating: 0, count: size)
    }

    deinit {
        storage.deallocate()
    }

    /// Appends `frames` frames, each the mean of `channels` consecutive
    /// samples `stride` samples apart from the previous frame (interleaved
    /// audio). Real-time safe. Producer only.
    public func write(source: UnsafePointer<Float>, stride: Int, channels: Int, frames: Int) {
        guard frames > 0, channels > 0 else { return }
        let start = written.load(ordering: .relaxed)
        let scale = 1 / Float(channels)
        // Only the newest `capacity` frames of a long write can survive.
        let skipped = max(frames - capacity, 0)
        for frame in skipped..<frames {
            let base = source + frame * stride
            var sum: Float = 0
            for channel in 0..<channels { sum += base[channel] }
            storage[(start + frame) & mask] = sum * scale
        }
        written.store(start + frames, ordering: .releasing)
    }

    /// Copies the newest `count` samples, oldest first. False, leaving
    /// `destination` unspecified, when fewer were written or the producer
    /// overwrote them during the copy. Consumer only.
    public func readLatest(into destination: UnsafeMutablePointer<Float>, count: Int) -> Bool {
        guard count > 0, count <= capacity else { return false }
        let end = written.load(ordering: .acquiring)
        guard end >= count else { return false }
        let start = end - count
        for i in 0..<count {
            destination[i] = storage[(start + i) & mask]
        }
        // The copy must happen before the check below reads the total.
        atomicMemoryFence(ordering: .acquiring)
        return written.load(ordering: .relaxed) - start <= capacity
    }

    /// Forgets every sample. Only while no producer runs.
    public func reset() {
        written.store(0, ordering: .releasing)
    }
}
