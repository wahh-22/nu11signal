import Foundation
import MusicKit
import Nu11SignalProtocol

struct CommandError: Error, CustomStringConvertible {
    let description: String
    init(_ description: String) { self.description = description }
}

/// Maps protocol commands onto MusicKit. main.swift decides concurrency:
/// playback commands arrive here one at a time in order, read-only ones
/// concurrently, so a slow network call never blocks transport commands.
@MainActor
final class CommandHandler {
    private let player = ApplicationMusicPlayer.shared
    private let emitter: StateEmitter

    init(emitter: StateEmitter) {
        self.emitter = emitter
    }

    /// Upper bound for one serialized playback command. MusicKit calls do
    /// not honour cancellation, so a timed-out call may still finish (and
    /// change playback) later; its late result is discarded. The trade-off
    /// favours a responsive chain over strict ordering of a hung call.
    /// Kept below the Go client's per-call deadline (8s, `defaultCallTimeout`
    /// in internal/radio) so the helper reports the timeout first.
    static let playbackTimeout: TimeInterval = 6

    /// Runs one request and sends exactly one response. With a timeout, a
    /// command that has not finished in time is answered with an error.
    func respond(to request: Request, timeout: TimeInterval? = nil) async {
        do {
            let result: JSONObject
            if let timeout {
                result = try await Deadline.run(seconds: timeout) { try await self.handle(request) }
            } else {
                result = try await handle(request)
            }
            Output.shared.send(.success(id: request.id, result: result))
        } catch {
            Output.shared.send(.failure(id: request.id, error: String(describing: error)))
        }
        if request.cmd == "seek" {
            // The signature ignores position, so a seek must be announced.
            emitter.emitNow()
        } else {
            emitter.checkForChange()
        }
    }

    private func handle(_ request: Request) async throws -> JSONObject {
        switch request.cmd {
        case "authorize": return try await authorize()
        case "search": return try await search(request)
        case "searchCatalog": return try await searchCatalog(request)
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
        search.limit = min(max(try optionalInt(request, "limit") ?? 25, 1), 25)
        let response = try await search.response()
        return ["songs": response.songs.map(songJSON)]
    }

    /// Mixed catalog search, as Apple Music shows it: term suggestions, then
    /// artists and songs. `limit` applies to each result type. Suggestions
    /// are best effort: when that request fails the command still succeeds
    /// with an empty list.
    private func searchCatalog(_ request: Request) async throws -> JSONObject {
        guard let term = request.string("term"), !term.isEmpty else {
            throw CommandError("searchCatalog requires a non-empty \"term\"")
        }
        // The catalog search endpoint accepts at most 25 results per page.
        let limit = min(max(try optionalInt(request, "limit") ?? 25, 1), 25)
        var search = MusicCatalogSearchRequest(term: term, types: [Artist.self, Song.self])
        search.limit = limit
        async let suggestions = suggestionTerms(for: term, limit: limit)
        let response = try await search.response()
        return [
            "suggestions": await suggestions,
            "artists": response.artists.map(artistJSON),
            "songs": response.songs.map(songJSON),
        ]
    }

    private func suggestionTerms(for term: String, limit: Int) async -> [String] {
        var request = MusicCatalogSearchSuggestionsRequest(term: term)
        // The suggestions endpoint returns at most 10 terms.
        request.limit = min(limit, 10)
        guard let response = try? await request.response() else { return [] }
        return response.suggestions.map(\.searchTerm)
    }

    private func playlists() async throws -> JSONObject {
        var library = MusicLibraryRequest<Playlist>()
        library.sort(by: \.name, ascending: true)
        let response = try await library.response()
        let items = response.items.map { ["id": $0.id.rawValue, "name": $0.name] }
        return ["playlists": items]
    }

    /// Plays the requested songs in order. Ids the catalog does not return
    /// are reported: an error when none are found, otherwise the found songs
    /// play and the result lists `"missing"`. If the song at `startIndex` is
    /// missing, playback starts at the next found song (or the first found).
    private func playSongs(_ request: Request) async throws -> JSONObject {
        guard let ids = request.strings("ids"), !ids.isEmpty else {
            throw CommandError("playSongs requires a non-empty \"ids\" array")
        }
        let startIndex = try optionalInt(request, "startIndex") ?? 0
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
        let missing = ids.filter { byID[$0] == nil }
        guard !songs.isEmpty else {
            throw CommandError("none of the requested songs were found in the catalog: \(missing.joined(separator: ", "))")
        }
        let start = ids[startIndex...].lazy.compactMap { byID[$0] }.first ?? songs[0]
        player.queue = ApplicationMusicPlayer.Queue(for: songs, startingAt: start)
        try await player.play()
        return missing.isEmpty ? [:] : ["missing": missing]
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

    /// Reads an optional integer argument; a present but non-integral value
    /// is an error rather than silently falling back to the default.
    private func optionalInt(_ request: Request, _ key: String) throws -> Int? {
        guard request.args[key] != nil else { return nil }
        guard let value = request.int(key) else {
            throw CommandError("\"\(key)\" must be an integer")
        }
        return value
    }

    private func artistJSON(_ artist: Artist) -> JSONObject {
        [
            "id": artist.id.rawValue,
            "name": artist.name,
            "genres": artist.genreNames ?? [],
        ]
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
