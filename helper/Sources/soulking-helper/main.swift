// soulking-helper: a windowless MusicKit player driven by JSON Lines on
// stdin/stdout. It must run from inside the signed SoulKingHelper.app bundle.
import Foundation
import MusicKit
import SoulKingProtocol

// A vanished reader must surface as a write error, not kill the process.
signal(SIGPIPE, SIG_IGN)

let emitter = MainActor.assumeIsolated { StateEmitter() }
let handler = MainActor.assumeIsolated { CommandHandler(emitter: emitter) }
let inFlight = DispatchGroup()

/// Reads stdin on a background thread. Playback commands run one at a time
/// in arrival order (each awaits the previous one); read-only commands run
/// concurrently, so a slow search never delays `pause`. Each playback command
/// is bounded by `CommandHandler.playbackTimeout`: a hung one is answered
/// with an error and the chain moves on.
///
/// At EOF, waits up to `Lifecycle.shutdownGrace` for in-flight requests and
/// queued output, then stops playback and exits regardless.
func readRequests() {
    var playbackTail: Task<Void, Never>?
    while let line = readLine(strippingNewline: true) {
        if line.trimmingCharacters(in: .whitespaces).isEmpty { continue }
        switch Codec.decode(line) {
        case let .failure(failure):
            Output.shared.send(.failure(id: failure.id, error: failure.message))
        case let .success(request):
            inFlight.enter()
            if request.mutatesPlayback {
                let previous = playbackTail
                playbackTail = Task { @MainActor in
                    await previous?.value
                    await handler.respond(to: request, timeout: CommandHandler.playbackTimeout)
                    inFlight.leave()
                }
            } else {
                Task { @MainActor in
                    await handler.respond(to: request)
                    inFlight.leave()
                }
            }
        }
    }
    let deadline = Date().addingTimeInterval(Lifecycle.shutdownGrace)
    if inFlight.wait(wallTimeout: .now() + Lifecycle.shutdownGrace) == .timedOut {
        log("requests still running \(Lifecycle.shutdownGrace)s after stdin closed; exiting anyway")
    }
    Output.shared.flush(before: deadline)
    Lifecycle.shutdown(code: 0)
}

MainActor.assumeIsolated { emitter.start() }
Output.shared.event("ready")
Thread.detachNewThread(readRequests)
RunLoop.main.run()
