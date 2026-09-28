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

/// Reads stdin on a background thread; each request runs in its own task.
/// At EOF, waits for in-flight requests, stops playback, and exits.
func readRequests() {
    while let line = readLine(strippingNewline: true) {
        if line.trimmingCharacters(in: .whitespaces).isEmpty { continue }
        switch Codec.decode(line) {
        case let .failure(failure):
            Output.shared.send(.failure(id: failure.id, error: failure.message))
        case let .success(request):
            inFlight.enter()
            Task { @MainActor in
                await handler.respond(to: request)
                inFlight.leave()
            }
        }
    }
    inFlight.notify(queue: .main) {
        MainActor.assumeIsolated { ApplicationMusicPlayer.shared.stop() }
        exit(0)
    }
}

MainActor.assumeIsolated { emitter.start() }
Output.shared.event("ready")
Thread.detachNewThread(readRequests)
RunLoop.main.run()
