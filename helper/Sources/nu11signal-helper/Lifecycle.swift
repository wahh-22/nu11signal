import Darwin
import Foundation
import MusicKit

/// Process shutdown shared by stdin EOF and stdout failure.
enum Lifecycle {
    /// How long stdin EOF waits for in-flight requests and queued output.
    static let shutdownGrace: TimeInterval = 3
    /// How long the main thread gets to stop playback before a hard exit.
    private static let stopGrace: TimeInterval = 2

    private static let lock = NSLock()
    nonisolated(unsafe) private static var shuttingDown = false

    /// Stops playback on the main actor, releases the app volume's tap and
    /// exits. Only the first call acts (the HAL also drops the private tap
    /// and aggregate device of a process that exits without doing so).
    /// If the main thread does not get there within `stopGrace`, the process
    /// exits anyway, so shutdown can never hang.
    static func shutdown(code: Int32) {
        lock.lock()
        let first = !shuttingDown
        shuttingDown = true
        lock.unlock()
        guard first else { return }

        DispatchQueue.global().asyncAfter(deadline: .now() + stopGrace) {
            log("playback did not stop within \(stopGrace)s; exiting")
            _exit(code)
        }
        DispatchQueue.main.async {
            MainActor.assumeIsolated {
                ApplicationMusicPlayer.shared.stop()
                AppVolume.shared.shutdown()
            }
            exit(code)
        }
    }
}
