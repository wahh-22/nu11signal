import Foundation

// The waveform the TUI's oscilloscope draws: the `levels` event also
// carries the analysis window, before the gain and mixed to mono,
// decimated to LevelsEvent.wavePoints points:
// {"event":"levels","bands":[...],"wave":[-100...100, ...]}.

extension LevelsEvent {
    /// Waveform points per event, oldest sample first.
    public static let wavePoints = 64
}

public enum Waveform {
    /// Splits samples into `points` consecutive buckets (as even as
    /// integer division allows) and keeps each bucket's largest swing, its
    /// sample farthest from zero with its sign, so a transient survives
    /// that averaging would flatten. Samples are clipped to -1...1 and NaN
    /// reads as 0. Fewer samples than points is no waveform: all zeros.
    public static func decimated(_ samples: [Float], points: Int) -> [Double] {
        samples.withUnsafeBufferPointer { decimated($0.baseAddress, count: $0.count, points: points) }
    }

    /// decimated(_:points:) of the `count` samples at `samples`.
    public static func decimated(_ samples: UnsafePointer<Float>?, count: Int, points: Int) -> [Double] {
        guard points > 0 else { return [] }
        var out = [Double](repeating: 0, count: points)
        guard let samples, count >= points else { return out }
        for point in 0..<points {
            var peak: Float = 0
            for i in (point * count / points)..<((point + 1) * count / points) {
                let sample = samples[i].isNaN ? 0 : min(max(samples[i], -1), 1)
                if abs(sample) > abs(peak) { peak = sample }
            }
            out[point] = Double(peak)
        }
        return out
    }

    /// A waveform as the integer hundredths the event carries, -100...100.
    public static func quantized(_ wave: [Double]) -> [Int] {
        wave.map { point in
            guard point.isFinite else { return 0 }
            return Int((min(max(point, -1), 1) * 100).rounded())
        }
    }
}
