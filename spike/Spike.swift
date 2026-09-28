// Spike: validate that a windowless helper can authorize, search the
// Apple Music catalog, and play a full track via ApplicationMusicPlayer.
import Foundation
import MusicKit

@MainActor
func run(term: String) async throws {
    let status = await MusicAuthorization.request()
    print("authorization: \(status)")
    guard status == .authorized else { exit(1) }

    let subscription = try await MusicSubscription.current
    print("canPlayCatalogContent: \(subscription.canPlayCatalogContent)")

    var request = MusicCatalogSearchRequest(term: term, types: [Song.self])
    request.limit = 5
    let response = try await request.response()
    for song in response.songs {
        print("found: \(song.title) — \(song.artistName)")
    }
    guard let song = response.songs.first else {
        print("no results")
        exit(1)
    }

    let player = ApplicationMusicPlayer.shared
    player.queue = [song]
    try await player.play()
    print("playing: \(song.title) — \(song.artistName)")

    for _ in 0..<20 {
        try await Task.sleep(for: .seconds(1))
        print(String(format: "  %@ %.1fs", "\(player.state.playbackStatus)", player.playbackTime))
    }
    player.stop()
}

let term = CommandLine.arguments.dropFirst().joined(separator: " ")
let query = term.isEmpty ? "Daft Punk Get Lucky" : term

Task { @MainActor in
    do {
        try await run(term: query)
        exit(0)
    } catch {
        print("error: \(error)")
        exit(1)
    }
}
RunLoop.main.run()
