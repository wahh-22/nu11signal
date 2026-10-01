import Darwin
import Foundation
import MusicKit
import Nu11SignalProtocol

/// Process shutdown shared by stdin EOF and stdout failure.
enum Lifecycle {
    /// How long stdin EOF waits for in-flight requests and queued output.
    static let shutdownGrace = ShutdownPlan.requestGraceSeconds
    /// How long the main thread gets for the quiet exit before a hard exit.
    private static let stopGrace = ShutdownPlan.backstopSeconds

    private static let lock = NSLock()
    nonisolated(unsafe) private static var shuttingDown = false

    /// Quiets playback on the main actor (see ShutdownPlan: a rendering
    /// app volume fades to 0, then the player pauses, then the tap goes),
    /// and exits. Only the first call acts (the HAL also drops the private
    /// tap and aggregate device of a process that exits without doing so).
    /// If the main actor does not finish within `stopGrace`, the process
    /// exits anyway, so shutdown can never hang.
    static func shutdown(code: Int32) {
        lock.lock()
        let first = !shuttingDown
        shuttingDown = true
        lock.unlock()
        guard first else { return }

        DispatchQueue.global().asyncAfter(deadline: .now() + stopGrace) {
            log("playback did not quiet down within \(stopGrace)s; exiting")
            _exit(code)
        }
        Task { @MainActor in
            await quietDown()
            exit(code)
        }
    }

    /// Performs the quiet exit's steps; MusicKit and the HAL only here.
    @MainActor
    private static func quietDown() async {
        let player = ApplicationMusicPlayer.shared
        let volume = AppVolume.shared
        for step in volume.beginShutdown(playing: isPlaying(player)) {
            switch step {
            case let .fadeOut(wait):
                volume.fadeOut()
                await sleep(wait)
            case .pausePlayer:
                player.pause()
            case let .awaitPaused(timeout):
                let deadline = Date().addingTimeInterval(timeout)
                while isPlaying(player), Date() < deadline {
                    await sleep(0.01)
                }
            case let .settle(seconds):
                await sleep(seconds)
            case .teardown:
                volume.shutdown()
            }
        }
    }

    @MainActor
    private static func isPlaying(_ player: ApplicationMusicPlayer) -> Bool {
        PlaybackActivity.isPlaying(Snapshot.name(of: player.state.playbackStatus))
    }

    private static func sleep(_ seconds: Double) async {
        try? await Task.sleep(nanoseconds: UInt64(max(seconds, 0) * 1_000_000_000))
    }
}
