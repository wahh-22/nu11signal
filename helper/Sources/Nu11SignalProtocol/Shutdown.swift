// The helper's quiet exit: the order and timings of shutting playback
// down without a click, free of MusicKit and CoreAudio so they can be unit
// tested. Lifecycle.swift performs the steps.
//
// The Go client mirrors requestGraceSeconds and backstopSeconds in its
// close timeout (internal/helper: DefaultCloseTimeout, which documents the
// whole chain); a Go test reads them from this file.
import Foundation

/// What the helper does, in order, between stdin closing (once in-flight
/// requests are done) and exiting.
///
/// Stopping the aggregate device while it renders a non-zero sample clicks,
/// and so does releasing the muted tap while the player still plays (a few
/// milliseconds at the system volume). So a rendering tap first fades the
/// app gain to 0 over GainRamp.shutdownSeconds, much slower than a level
/// change, and only then is the player paused (never stopped) and, once it
/// reports so, the tap released: its IOProc stops on silence and the tap,
/// muted to the last, goes after its aggregate. Without a rendering tap (the
/// system volume) the player is paused and given a moment to settle.
public enum ShutdownPlan {
    public enum Step: Sendable, Equatable {
        /// Ramp the app gain to 0 at the shutdown rate and wait this long.
        case fadeOut(wait: Double)
        /// Pause the player (never stop it: that may cut the audio short).
        case pausePlayer
        /// Wait until the player no longer plays, at most this long.
        case awaitPaused(timeout: Double)
        /// Wait this long for a paused player's audio to die out.
        case settle(seconds: Double)
        /// Release every tap: the IOProc stops, then the aggregate and the
        /// muted tap go. A no-op without a tap.
        case teardown
    }

    /// How long stdin EOF waits for in-flight requests and queued output.
    public static let requestGraceSeconds: TimeInterval = 3
    /// How long the main thread gets for the quiet exit before a hard exit.
    public static let backstopSeconds: TimeInterval = 2
    /// How long the app volume's fade to silence takes (GainRamp.shutdownSeconds).
    public static let fadeSeconds: TimeInterval = 0.2
    /// Added to the fade's wait: one IO buffer or so may still be in flight
    /// when the gain target drops, and the last one must render silence.
    public static let fadeMarginSeconds: TimeInterval = 0.03
    /// How long a faded player gets to report that it paused.
    public static let pauseWaitSeconds: TimeInterval = 0.15
    /// How long a player at the system volume gets after its pause.
    public static let settleSeconds: TimeInterval = 0.2

    /// The steps for what plays now: `rendering` when the app volume's
    /// IOProc runs, `playing` when the player reports it plays.
    public static func steps(rendering: Bool, playing: Bool) -> [Step] {
        if rendering {
            return [.fadeOut(wait: GainRamp.shutdownSeconds + fadeMarginSeconds), .pausePlayer,
                    .awaitPaused(timeout: pauseWaitSeconds), .teardown]
        }
        if playing {
            return [.pausePlayer, .settle(seconds: settleSeconds), .teardown]
        }
        return [.teardown]
    }

    /// The longest the steps wait in all.
    public static func duration(_ steps: [Step]) -> Double {
        steps.reduce(0) { total, step in
            switch step {
            case let .fadeOut(wait): total + wait
            case let .awaitPaused(timeout): total + timeout
            case let .settle(seconds): total + seconds
            case .pausePlayer, .teardown: total
            }
        }
    }
}
