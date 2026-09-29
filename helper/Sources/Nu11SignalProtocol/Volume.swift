// Pure argument handling of the `volume` and `setVolume` commands, free of
// CoreAudio so it can be unit tested.
import Foundation

/// Output volume levels on the wire: numbers from 0 (silent) to 1 (full).
public enum VolumeLevel {
    /// The level `setVolume` asks for, clamped to 0...1. A missing,
    /// non-numeric or non-finite `level` is an error rather than a guess.
    public static func requested(_ request: Request) throws -> Float32 {
        guard let level = request.double("level"), level.isFinite else {
            throw ArgumentError(description: "\(request.cmd) requires a number \"level\" from 0 to 1")
        }
        return Float32(min(max(level, 0), 1))
    }

    /// A level read from the device, clamped to 0...1 and rounded to four
    /// decimals so Float32 noise (0.3 reads back as 0.30000001192...)
    /// stays off the wire. NaN reports as 0.
    public static func reported(_ level: Float32) -> Double {
        guard !level.isNaN else { return 0 }
        let clamped = min(max(Double(level), 0), 1)
        return (clamped * 10_000).rounded() / 10_000
    }
}
