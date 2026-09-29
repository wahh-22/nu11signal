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

    /// Upper bound for each optional part of an artist page (one
    /// relationship, or the origin/formed lookup). A part that fails or runs
    /// out of time degrades to an empty section instead of failing the
    /// command. Kept below the Go client's artist deadline (15s,
    /// `artistCallTimeout` in internal/radio) minus the artist lookup itself.
    static let artistSectionTimeout: TimeInterval = 8

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
        case "searchCatalog": return try await searchCatalog(request)
        case "artist": return try await artist(request)
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

    /// An artist page, as Apple Music lays it out. Only the artist lookup
    /// can fail the command; each section is its own concurrent request
    /// (`with` loads one relationship at a time) so one that fails or hangs
    /// leaves just that section empty. `featuredAlbums` stands in for
    /// "Essential Albums", and artist playlists fall back to
    /// `featuredPlaylists` when `playlists` is empty.
    private func artist(_ request: Request) async throws -> JSONObject {
        guard let id = request.string("artistId"), !id.isEmpty else {
            throw CommandError("artist requires a non-empty \"artistId\"")
        }
        let lookup = MusicCatalogResourceRequest<Artist>(matching: \.id, equalTo: MusicItemID(id))
        guard let artist = try await lookup.response().items.first else {
            throw CommandError("artist \(id) was not found in the catalog")
        }
        async let topSongs = Self.section(artist, .topSongs, \.topSongs)
        async let essential = Self.section(artist, .featuredAlbums, \.featuredAlbums)
        async let albums = Self.section(artist, .fullAlbums, \.fullAlbums)
        async let singles = Self.section(artist, .singles, \.singles)
        async let compilations = Self.section(artist, .compilationAlbums, \.compilationAlbums)
        async let playlists = Self.section(artist, .playlists, \.playlists)
        async let featuredPlaylists = Self.section(artist, .featuredPlaylists, \.featuredPlaylists)
        async let facts = Self.artistFacts(id: id)

        let notes = artist.editorialNotes.flatMap { $0.standard ?? $0.short } ?? ""
        var lists = await playlists
        if lists.isEmpty {
            lists = await featuredPlaylists
        }
        let origin = await facts
        return [
            "artist": artistJSON(artist),
            "topSongs": await topSongs.map(songJSON),
            "essentialAlbums": await essential.map(albumJSON),
            "albums": await albums.map(albumJSON),
            "singles": await singles.map(albumJSON),
            "compilations": await compilations.map(albumJSON),
            "playlists": lists.map { ["id": $0.id.rawValue, "name": $0.name, "curator": $0.curatorName ?? ""] },
            "about": [
                "notes": EditorialText.plain(notes),
                "genre": artist.genreNames?.first ?? "",
                "origin": origin.origin,
                "formed": origin.formed,
            ],
        ]
    }

    /// Loads one relationship of artist, or nothing when it fails or takes
    /// longer than `artistSectionTimeout`.
    private nonisolated static func section<Item>(
        _ artist: Artist,
        _ property: MusicRelationshipProperty<Artist, Item>,
        _ items: KeyPath<Artist, MusicItemCollection<Item>?>
    ) async -> [Item] {
        guard let loaded = try? await Deadline.run(seconds: artistSectionTimeout, {
            try await artist.with([property])
        }) else { return [] }
        return Array(loaded[keyPath: items] ?? [])
    }

    /// Where the artist is from and when it was born or formed. MusicKit
    /// does not model these, so they come from the raw catalog API; any
    /// failure leaves them empty.
    private nonisolated static func artistFacts(id: String) async -> ArtistFacts {
        let none = ArtistFacts(origin: "", formed: "")
        return (try? await Deadline.run(seconds: artistSectionTimeout) {
            let storefront = try await MusicDataRequest.currentCountryCode
            var url = URLComponents()
            url.scheme = "https"
            url.host = "api.music.apple.com"
            url.path = "/v1/catalog/\(storefront)/artists/\(id)"
            url.queryItems = [URLQueryItem(name: "extend", value: "origin,bornOrFormed")]
            guard let url = url.url else { return none }
            let response = try await MusicDataRequest(urlRequest: URLRequest(url: url)).response()
            return ArtistFacts.parse(response.data)
        }) ?? none
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

    /// Release years are read in UTC: catalog release dates are calendar
    /// dates, and a local time zone west of UTC would move 1 January back a
    /// year.
    private static let utcCalendar: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        return calendar
    }()

    private func albumJSON(_ album: Album) -> JSONObject {
        [
            "id": album.id.rawValue,
            "title": album.title,
            "artist": album.artistName,
            "year": album.releaseDate.map { Self.utcCalendar.component(.year, from: $0) } ?? 0,
            "trackCount": album.trackCount,
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
