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
    /// Set right before the helper tries to re-execute itself as its own
    /// responsible process, whether or not that works, so no image tries
    /// again. It says nothing about the outcome: Relaunch.isDisclaimed
    /// checks that.
    public static let relaunchAttemptedKey = "NU11SIGNAL_HELPER_RELAUNCH_ATTEMPTED"
    /// `system` forces the system volume (no tap, no relaunch, no prompt).
    public static let volumeModeKey = "NU11SIGNAL_VOLUME_MODE"

    /// Whether to re-execute once, disclaiming the launcher's responsibility.
    public static func shouldRelaunch(environment: [String: String]) -> Bool {
        environment[relaunchAttemptedKey] == nil && !forcesSystemVolume(environment: environment)
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

    /// Maps a `TCCAccessPreflight` result: 0 granted, 1 denied, 2 not
    /// asked yet; any other value is not understood, so the permission is
    /// neither used nor asked for.
    public init(preflightResult: Int) {
        switch preflightResult {
        case 0: self = .authorized
        case 1: self = .denied
        case 2: self = .undetermined
        default: self = .unavailable
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
    /// How long a full swing takes while the helper exits (see
    /// ShutdownPlan): slow enough to sound like a fade, not a cut.
    public static let shutdownSeconds = ShutdownPlan.fadeSeconds

    /// The gain change per frame at sampleRate for a full swing over
    /// `seconds`; a jump without a rate or a duration.
    public static func increment(sampleRate: Double, seconds: Double = GainRamp.seconds) -> Float {
        guard sampleRate.isFinite, sampleRate > 0, seconds.isFinite, seconds > 0 else { return 1 }
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
/// runs a RemotePlayerService copy per client, and other clients' copies
/// may be around, idle or playing.
public enum RemotePlayerTarget {
    public static let bundleID = "com.apple.MediaPlayer.RemotePlayerService"

    /// The newest copy macOS holds this helper responsible for; nil when
    /// there is none (yet), or responsibility cannot be read. Only that
    /// positive attribution counts: a muted tap on another client's copy
    /// would silence that client, so a copy that merely appeared after the
    /// helper started is never taken.
    public static func select(_ records: [AudioProcessRecord], helperPID: Int32) -> AudioProcessRecord? {
        records.filter { $0.bundleID == bundleID && $0.responsiblePID == helperPID }.max(by: { $0.pid < $1.pid })
    }
}

/// The player's activity, from the status names `state` events carry.
public enum PlaybackActivity: Sendable, Equatable {
    case stopped, paused, playing

    /// "playing" and "seeking" play; "stopped" stops; anything else
    /// ("paused", "interrupted") is paused.
    public init(status: String) {
        switch status {
        case "playing", "seeking": self = .playing
        case "stopped": self = .stopped
        default: self = .paused
        }
    }

    public static func isPlaying(_ status: String) -> Bool {
        PlaybackActivity(status: status) == .playing
    }
}

/// Why building or starting the tap failed.
public enum TapFailure: Sendable, Equatable {
    /// Worth retrying: an OSStatus from the HAL (often mid device change),
    /// no output device, the player process not attributed yet.
    case transient(String)
    /// Retrying cannot help (e.g. a format the IOProc cannot render).
    case permanent(String)
}

/// The app volume tap's lifecycle, free of CoreAudio: AppVolume feeds it
/// events (playback, build results, device and process changes, retry
/// timers, the gain) and performs the actions it returns, in order,
/// feeding their results back. It is only fed in app mode.
///
/// A rebuild builds the new tap before releasing the old one: `build`
/// never destroys the tap in use, and `releaseReplaced` drops the old one
/// once the new one has started. So a failed rebuild keeps the old muted
/// tap, with its audio thread stopped, while it retries after a growing
/// delay (`retryDelays`): as long as that tap still mutes the player (an
/// output device change), the music is silent, never suddenly at the
/// system volume. When no tap mutes the player (none was built, or the
/// player moved to a process the old tap does not cover) and the app gain
/// is below the system volume, the player is paused for the retries
/// (`pausePlayer`) and resumed once a new tap is built (`resumePlayer`);
/// at full gain it keeps playing, since the system volume is then no
/// louder. One more failure after the last retry, or any permanent
/// failure, falls back to the system volume for the session (`fallBack`,
/// after which AppVolume releases everything), a held player staying
/// paused. A tap that starts forgets earlier failures.
public struct TapLifecycle: Sendable, Equatable {
    public enum Phase: Sendable, Equatable {
        /// No tap.
        case idle
        /// The muted tap is (being) built; its audio thread waits for the
        /// player to play.
        case building
        /// The audio thread renders.
        case running
        /// The audio thread stopped with the player; the muted tap stays,
        /// so resuming is never briefly loud.
        case paused
        /// A retry is scheduled; the last muted tap, if any, is kept.
        case retrying(attempt: Int)
        /// The system volume, for the rest of the session.
        case fallback(reason: String)
    }

    /// What a rebuild answers to.
    public enum Change: Sendable, Equatable {
        /// A new default output device: the tap still mutes the player,
        /// only its aggregate device renders to the old output.
        case outputDevice
        /// The player's process changed (or went away): the tap, if any,
        /// no longer mutes the player.
        case playerProcess
    }

    public enum Event: Sendable, Equatable {
        /// The helper is about to ask the player to play.
        case prepare
        /// That play threw.
        case playFailed
        /// The player reported an activity.
        case playback(PlaybackActivity)
        /// `build` made a new tap (the one it replaces, if any, stays until
        /// `releaseReplaced`).
        case built
        /// `build` found no player process attributed to the helper.
        case noTarget
        /// `build` or `start` failed; the tap in use, if any, is untouched.
        case failed(TapFailure)
        /// `start` started the audio thread.
        case started
        /// The default output device or the player process changed.
        case rebuild(Change)
        /// A scheduled retry is due.
        case retryDue
        /// The app level is (not) below the system volume, i.e. under 1.
        case gain(quieterThanSystem: Bool)
    }

    public enum Action: Sendable, Equatable {
        /// Build a new muted tap, keeping the one in use until it starts.
        case build
        case start, stop
        /// Release the tap the last `build` replaced.
        case releaseReplaced
        /// Release every tap.
        case teardown
        case scheduleRetry(after: Double)
        /// Pause the player: no tap mutes it and it would play louder.
        case pausePlayer
        /// Resume the player `pausePlayer` paused: a new tap is built.
        case resumePlayer
        /// Switch to the system volume for the session, releasing the tap.
        case fallBack(reason: String)
    }

    /// The delays, in seconds, before each retry: about five seconds in
    /// all, enough for a device change to settle.
    public static let retryDelays: [Double] = [0.5, 1.5, 3]

    public private(set) var phase: Phase = .idle
    public private(set) var activity: PlaybackActivity = .stopped
    /// A play was asked for and the player has not reported since.
    public private(set) var expecting = false
    /// A muted tap exists (in use, or kept through a retry).
    public private(set) var hasTap = false
    /// That tap mutes the player's current process.
    public private(set) var tapMutesPlayer = false
    /// The player is paused by `pausePlayer`, awaiting `resumePlayer`.
    public private(set) var holdingPlayer = false
    private var quieterThanSystem = false
    /// The pending `build` replaces a tap, to release once it starts.
    private var replacing = false
    private var failures = 0

    public init() {}

    /// Whether the tap should exist: audio plays or is about to.
    private var wantsTap: Bool { activity == .playing || expecting }

    /// Whether a change of the HAL's process list is worth comparing the
    /// player's process for (then `rebuild(.playerProcess)` if it moved):
    /// while a tap exists or is wanted, and during a retry only while the
    /// kept tap still mutes the player (the retry builds for the new
    /// process by itself, but the player may need pausing meanwhile).
    public var watchesPlayerProcess: Bool {
        switch phase {
        case .idle, .building, .running, .paused: hasTap || wantsTap
        case .retrying: hasTap && tapMutesPlayer
        case .fallback: false
        }
    }

    public mutating func handle(_ event: Event) -> [Action] {
        if case .fallback = phase { return [] }
        switch event {
        case .prepare:
            expecting = true
            guard phase == .idle else { return [] } // a retry pending, or a tap already
            phase = .building
            return build()
        case .playFailed:
            expecting = false
            guard activity != .playing else { return [] } // the music playing keeps its tap
            switch phase {
            case .building:
                phase = .idle
                return teardown()
            case .retrying:
                reset()
                return teardown()
            default:
                return []
            }
        case let .playback(next):
            activity = next
            expecting = false
            return playback(next)
        case .built:
            hasTap = true
            tapMutesPlayer = true
            let held = holdingPlayer
            holdingPlayer = false
            guard activity == .playing else {
                phase = .building
                return held ? [.resumePlayer] : []
            }
            phase = .running
            return [.start]
        case .noTarget:
            // Before the audio starts the copy may not exist yet (the
            // process list change asks again); once it plays (or is held
            // for the retry), it should.
            guard activity == .playing || holdingPlayer else {
                phase = .idle
                return hasTap ? teardown() : []
            }
            return fail(.transient("no player process attributed to the helper"))
        case let .failed(failure):
            return fail(failure)
        case .started:
            failures = 0
            guard replacing else { return [] }
            replacing = false
            return [.releaseReplaced]
        case let .rebuild(change):
            if change == .playerProcess { tapMutesPlayer = false }
            switch phase {
            case .building, .running, .paused:
                guard wantsTap else {
                    phase = .idle
                    return teardown()
                }
                phase = .building
                return build()
            case .idle:
                guard wantsTap else { return [] }
                phase = .building
                return build()
            case .retrying:
                // The pending retry builds for the new state; a player the
                // kept tap no longer mutes may need pausing until then.
                return change == .playerProcess ? holdIfExposed() : []
            case .fallback:
                return []
            }
        case .retryDue:
            guard case .retrying = phase else { return [] } // cancelled
            guard wantsTap || holdingPlayer else {
                phase = .idle
                return teardown()
            }
            phase = .building
            return build()
        case let .gain(quieter):
            quieterThanSystem = quieter
            return []
        }
    }

    private mutating func playback(_ next: PlaybackActivity) -> [Action] {
        switch next {
        case .playing:
            switch phase {
            case .idle:
                phase = .building
                return build()
            case .building, .paused:
                phase = .running
                return [.start]
            case .retrying:
                // Playing while held is the user resuming: respect it.
                guard !holdingPlayer else {
                    holdingPlayer = false
                    return []
                }
                return holdIfExposed()
            default:
                return []
            }
        case .paused:
            guard phase == .running else { return [] }
            phase = .paused
            return [.stop]
        case .stopped:
            switch phase {
            case .idle:
                return []
            default:
                reset()
                return teardown()
            }
        }
    }

    private mutating func build() -> [Action] {
        replacing = hasTap
        return [.build]
    }

    private mutating func teardown() -> [Action] {
        hasTap = false
        tapMutesPlayer = false
        replacing = false
        return [.teardown]
    }

    private mutating func reset() {
        phase = .idle
        failures = 0
        holdingPlayer = false
    }

    /// Pauses a playing player no tap mutes, when it would play louder.
    private mutating func holdIfExposed() -> [Action] {
        guard activity == .playing, !tapMutesPlayer, quieterThanSystem, !holdingPlayer else { return [] }
        holdingPlayer = true
        return [.pausePlayer]
    }

    private mutating func fail(_ failure: TapFailure) -> [Action] {
        switch failure {
        case let .permanent(reason):
            phase = .fallback(reason: reason)
            return [.fallBack(reason: reason)]
        case let .transient(reason):
            failures += 1
            guard failures <= Self.retryDelays.count else {
                let final = "\(reason), \(Self.retryDelays.count) retries failed"
                phase = .fallback(reason: final)
                return [.fallBack(reason: final)]
            }
            phase = .retrying(attempt: failures)
            return [.stop] + holdIfExposed() + [.scheduleRetry(after: Self.retryDelays[failures - 1])]
        }
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
