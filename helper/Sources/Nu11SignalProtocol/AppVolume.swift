// Pure decisions and math of the app volume (a Core Audio process tap on
// the player's audio, re-rendered with a gain), free of CoreAudio so they
// can be unit tested. The helper wires them to the HAL in AppVolume.swift.
import Foundation

/// Which volume the `volume` and `setVolume` commands drive, reported as
/// their result's `mode` and as `volumeMode` in `state` events.
public enum VolumeMode: String, Sendable {
    /// The helper's own gain: the system volume is never touched.
    case app
    /// The default output device's volume (the fallback).
    case system
}

/// How the helper process is launched so that macOS asks the user, not
/// the launching terminal, for audio capture permission.
public enum HelperLaunch {
    /// Set before the helper re-executes itself as its own responsible
    /// process, so the new image does not do it again.
    public static let relaunchedKey = "NU11SIGNAL_HELPER_RELAUNCHED"
    /// `system` forces the system volume (no tap, no relaunch, no prompt).
    public static let volumeModeKey = "NU11SIGNAL_VOLUME_MODE"

    /// Whether to re-execute once, disclaiming the launcher's responsibility.
    public static func shouldRelaunch(environment: [String: String]) -> Bool {
        environment[relaunchedKey] == nil && !forcesSystemVolume(environment: environment)
    }

    public static func forcesSystemVolume(environment: [String: String]) -> Bool {
        environment[volumeModeKey] == VolumeMode.system.rawValue
    }
}

/// The audio capture (TCC) permission a process tap needs.
public enum CapturePermission: Sendable, Equatable {
    case authorized, denied, undetermined
    /// The permission cannot be checked on this system.
    case unavailable

    /// Maps a `TCCAccessPreflight` result: 0 granted, 1 denied, anything
    /// else not asked yet.
    public init(preflightResult: Int) {
        switch preflightResult {
        case 0: self = .authorized
        case 1: self = .denied
        default: self = .undetermined
        }
    }
}

/// Decides the volume mode from what this process and system allow.
public struct VolumeModePolicy: Sendable {
    /// The helper is its own responsible process (see HelperLaunch).
    public var disclaimed: Bool
    /// The system has Core Audio process taps (macOS 14.2+).
    public var tapsSupported: Bool
    public var forcedSystem: Bool

    public init(disclaimed: Bool, tapsSupported: Bool, forcedSystem: Bool) {
        self.disclaimed = disclaimed
        self.tapsSupported = tapsSupported
        self.forcedSystem = forcedSystem
    }

    private var capable: Bool { disclaimed && tapsSupported && !forcedSystem }

    /// The app volume only with every prerequisite and a granted
    /// permission; the system volume otherwise, including while the
    /// permission prompt is pending, so a muted tap never waits on it.
    public func mode(_ permission: CapturePermission) -> VolumeMode {
        capable && permission == .authorized ? .app : .system
    }

    /// Whether to ask the user for the permission.
    public func shouldRequest(_ permission: CapturePermission) -> Bool {
        capable && permission == .undetermined
    }
}

/// The app volume level, 0...1, as stored and as applied.
public enum AppGain {
    /// The helper's UserDefaults key for the last level set.
    public static let defaultsKey = "appVolume"
    public static let defaultLevel: Double = 1

    /// The stored level, clamped; anything but a finite number is the
    /// default.
    public static func stored(_ value: Any?) -> Double {
        guard let number = value as? NSNumber, CFGetTypeID(number) != CFBooleanGetTypeID() else {
            return defaultLevel
        }
        let level = number.doubleValue
        guard level.isFinite else { return defaultLevel }
        return min(max(level, 0), 1)
    }

    /// The amplitude a level applies: a square law, so equal steps sound
    /// roughly equal (0.5 is about -12 dB) instead of crowding the audible
    /// change into the bottom of the range. NaN is silence.
    public static func amplitude(level: Double) -> Float {
        guard !level.isNaN else { return 0 }
        let clamped = min(max(level, 0), 1)
        return Float(clamped * clamped)
    }
}

/// The gain ramp that keeps level changes free of clicks. Everything here
/// is allocation- and lock-free, so the real-time IOProc can call it.
public enum GainRamp {
    /// How long a full 0...1 swing takes.
    public static let seconds = 0.015

    /// The gain change per frame at sampleRate; a jump without a rate.
    public static func increment(sampleRate: Double) -> Float {
        guard sampleRate.isFinite, sampleRate > 0 else { return 1 }
        return Float(1 / (sampleRate * seconds))
    }

    /// The gain `frame` frames into a ramp from start toward target.
    @inline(__always)
    public static func gain(atFrame frame: Int, start: Float, target: Float, increment: Float) -> Float {
        let moved = increment * Float(frame)
        return start < target ? min(start + moved, target) : max(start - moved, target)
    }

    /// Writes `frames` samples of one channel, the source scaled along the
    /// ramp; frames the source lacks (or all, without one) are silence.
    /// Strides are in samples, for interleaved buffers.
    @inline(__always)
    public static func apply(source: UnsafePointer<Float>?, sourceStride: Int, sourceFrames: Int,
                             destination: UnsafeMutablePointer<Float>, destinationStride: Int, frames: Int,
                             start: Float, target: Float, increment: Float) {
        let available = source == nil ? 0 : min(sourceFrames, frames)
        if let source {
            for frame in 0..<available {
                destination[frame * destinationStride] =
                    source[frame * sourceStride] * gain(atFrame: frame, start: start, target: target, increment: increment)
            }
        }
        for frame in available..<max(frames, available) {
            destination[frame * destinationStride] = 0
        }
    }
}

/// A HAL process object, as the helper reads it.
public struct AudioProcessRecord: Sendable, Equatable {
    public var objectID: UInt32
    public var pid: Int32
    public var bundleID: String
    public var isRunningOutput: Bool
    /// The process macOS holds responsible for it, when that can be read.
    public var responsiblePID: Int32?

    public init(objectID: UInt32, pid: Int32, bundleID: String, isRunningOutput: Bool, responsiblePID: Int32?) {
        self.objectID = objectID
        self.pid = pid
        self.bundleID = bundleID
        self.isRunningOutput = isRunningOutput
        self.responsiblePID = responsiblePID
    }
}

/// Picks the process that renders this helper's MusicKit audio: MediaPlayer
/// runs a RemotePlayerService copy per client, and idle copies for other
/// clients may be around.
public enum RemotePlayerTarget {
    public static let bundleID = "com.apple.MediaPlayer.RemotePlayerService"

    /// The copy this helper is responsible for; else the newest copy that
    /// did not exist before the helper played (`baseline`: the copies' pids
    /// seen at startup), playing ones first. Nil when there is none yet.
    public static func select(_ records: [AudioProcessRecord], helperPID: Int32,
                              baseline: Set<Int32>) -> AudioProcessRecord? {
        let copies = records.filter { $0.bundleID == bundleID }
        if let own = copies.filter({ $0.responsiblePID == helperPID }).max(by: { $0.pid < $1.pid }) {
            return own
        }
        let fresh = copies.filter { !baseline.contains($0.pid) }
        return fresh.filter(\.isRunningOutput).max(by: { $0.pid < $1.pid })
            ?? fresh.max(by: { $0.pid < $1.pid })
    }
}

/// The stream formats the gain IOProc renders.
public enum TapFormat {
    private static let linearPCM: UInt32 = 0x6C70_636D // kAudioFormatLinearPCM, 'lpcm'
    private static let isFloatFlag: UInt32 = 1         // kAudioFormatFlagIsFloat

    /// Only 32-bit float linear PCM (what taps and the HAL use); anything
    /// else is left alone rather than misread.
    public static func isFloat32(formatID: UInt32, flags: UInt32, bitsPerChannel: UInt32) -> Bool {
        formatID == linearPCM && flags & isFloatFlag != 0 && bitsPerChannel == 32
    }
}
