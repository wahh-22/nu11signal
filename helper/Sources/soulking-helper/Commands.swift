import Foundation
import MusicKit
import SoulKingProtocol

struct CommandError: Error, CustomStringConvertible {
    let description: String
    init(_ description: String) { self.description = description }
}

/// Maps protocol commands onto MusicKit. Each request runs in its own task,
/// so a slow network call (search, playlists) never blocks transport commands.
@MainActor
final class CommandHandler {
    private let player = ApplicationMusicPlayer.shared
    private let emitter: StateEmitter

    init(emitter: StateEmitter) {
        self.emitter = emitter
    }

    func respond(to request: Request) async {
        do {
            let result = try await handle(request)
            Output.shared.send(.success(id: request.id, result: result))
        } catch {
            Output.shared.send(.failure(id: request.id, error: String(describing: error)))
        }
        emitter.checkForChange()
    }

    private func handle(_ request: Request) async throws -> JSONObject {
        switch request.cmd {
        case "authorize": return try await authorize()
        case "search": return try await search(request)
        case "playlists": return try await playlists()
        case "playSongs": return try await playSongs(request)
        case "playPlaylist": return try await playPlaylist(request)
        case "pause": player.pause(); return [:]
        case "resume": try await player.play(); return [:]
        case "next": try await player.skipToNextEntry(); return [:]
        case "previous": try await player.skipToPreviousEntry(); return [:]
        case "stop": player.stop(); return [:]
        case "seek": return try seek(request)
        default: throw CommandError("unknown command: \(request.cmd)")
        }
    }

    private func authorize() async throws -> JSONObject {
        let status: String
        switch await MusicAuthorization.request() {
        case .authorized: status = "authorized"
        case .denied: status = "denied"
        case .restricted: status = "restricted"
        case .notDetermined: status = "notDetermined"
        @unknown default: status = "notDetermined"
        }
        return ["status": status]
    }

    private func search(_ request: Request) async throws -> JSONObject {
        guard let term = request.string("term"), !term.isEmpty else {
            throw CommandError("search requires a non-empty \"term\"")
        }
        var search = MusicCatalogSearchRequest(term: term, types: [Song.self])
        // The catalog search endpoint accepts at most 25 results per page.
        search.limit = min(max(request.int("limit") ?? 25, 1), 25)
        let response = try await search.response()
        return ["songs": response.songs.map(songJSON)]
    }

    private func playlists() async throws -> JSONObject {
        var library = MusicLibraryRequest<Playlist>()
        library.sort(by: \.name, ascending: true)
        let response = try await library.response()
        let items = response.items.map { ["id": $0.id.rawValue, "name": $0.name] }
        return ["playlists": items]
    }

    private func playSongs(_ request: Request) async throws -> JSONObject {
        guard let ids = request.strings("ids"), !ids.isEmpty else {
            throw CommandError("playSongs requires a non-empty \"ids\" array")
        }
        let startIndex = request.int("startIndex") ?? 0
        guard ids.indices.contains(startIndex) else {
            throw CommandError("startIndex \(startIndex) is out of range")
        }
        let lookup = MusicCatalogResourceRequest<Song>(
            matching: \.id, memberOf: ids.map { MusicItemID($0) }
        )
        let found = try await lookup.response().items
        // The catalog may return songs in any order; keep the requested order.
        let byID = Dictionary(found.map { ($0.id.rawValue, $0) }, uniquingKeysWith: { first, _ in first })
        let songs = ids.compactMap { byID[$0] }
        guard let start = byID[ids[startIndex]] else {
            throw CommandError("song \(ids[startIndex]) was not found in the catalog")
        }
        player.queue = ApplicationMusicPlayer.Queue(for: songs, startingAt: start)
        try await player.play()
        return [:]
    }

    private func playPlaylist(_ request: Request) async throws -> JSONObject {
        guard let id = request.string("playlistId"), !id.isEmpty else {
            throw CommandError("playPlaylist requires a non-empty \"playlistId\"")
        }
        var library = MusicLibraryRequest<Playlist>()
        library.filter(matching: \.id, equalTo: MusicItemID(id))
        guard let playlist = try await library.response().items.first else {
            throw CommandError("playlist \(id) was not found in the library")
        }
        player.queue = [playlist]
        try await player.play()
        return [:]
    }

    private func seek(_ request: Request) throws -> JSONObject {
        guard let seconds = request.double("seconds"), seconds >= 0 else {
            throw CommandError("seek requires a non-negative number \"seconds\"")
        }
        player.playbackTime = seconds
        return [:]
    }

    private func songJSON(_ song: Song) -> JSONObject {
        [
            "id": song.id.rawValue,
            "title": song.title,
            "artist": song.artistName,
            "album": song.albumTitle ?? "",
            "duration": song.duration ?? 0,
        ]
    }
}
