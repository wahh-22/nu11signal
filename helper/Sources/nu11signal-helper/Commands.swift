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
        case "searchCatalog": return try await searchCatalog(request)
        case "artist": return try await artist(request)
        case "album": return try await album(request)
        case "songAlbum": return try await songAlbum(request)
        case "catalogPlaylist": return try await catalogPlaylist(request)
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

    /// Mixed catalog search, as Apple Music shows it: term suggestions, the
    /// top results across kinds, then artists, albums, songs and playlists.
    /// `limit` applies to each result type; top results stop at
    /// `topResultLimit`. Suggestions are best effort: when that request
    /// fails the command still succeeds with an empty list.
    private func searchCatalog(_ request: Request) async throws -> JSONObject {
        let query = try CatalogSearchQuery(request)
        var search = MusicCatalogSearchRequest(
            term: query.term, types: [Artist.self, Album.self, Song.self, Playlist.self])
        search.limit = query.limit
        search.includeTopResults = true
        async let suggestions = suggestionTerms(for: query.term, limit: query.suggestionLimit)
        let response = try await search.response()
        return [
            "suggestions": await suggestions,
            "top": Array(response.topResults.compactMap(topResultJSON).prefix(query.topResultLimit)),
            "artists": response.artists.map(artistJSON),
            "albums": response.albums.map(albumJSON),
            "songs": response.songs.map(songJSON),
            "playlists": response.playlists.map(catalogPlaylistJSON),
        ]
    }

    /// A top result as {"kind": ..., "<kind>": {...}}; nil for the kinds the
    /// UI cannot open (stations, music videos, curators, radio shows,
    /// record labels, and any added later).
    private func topResultJSON(_ result: MusicCatalogSearchResponse.TopResult) -> JSONObject? {
        switch result {
        case .artist(let artist): return ["kind": "artist", "artist": artistJSON(artist)]
        case .album(let album): return ["kind": "album", "album": albumJSON(album)]
        case .song(let song): return ["kind": "song", "song": songJSON(song)]
        case .playlist(let playlist): return ["kind": "playlist", "playlist": catalogPlaylistJSON(playlist)]
        default: return nil
        }
    }

    private func suggestionTerms(for term: String, limit: Int) async -> [String] {
        var request = MusicCatalogSearchSuggestionsRequest(term: term)
        request.limit = limit
        guard let response = try? await request.response() else { return [] }
        return response.suggestions.map(\.searchTerm)
    }

    /// An artist page, as Apple Music lays it out. Only the artist lookup
    /// can fail the command; each section is its own concurrent request
    /// (`with` loads one relationship at a time) so one that fails or hangs
    /// leaves just that section empty. `featuredAlbums` stands in for
    /// "Essential Albums". The command stays within `CatalogBudget.artist`:
    /// the lookup, then the parts under one section deadline each.
    private func artist(_ request: Request) async throws -> JSONObject {
        let id = try request.requiredID("artistId")
        let artist = try await Deadline.run(seconds: CatalogBudget.lookup) {
            try await MusicCatalogResourceRequest<Artist>(matching: \.id, equalTo: MusicItemID(id)).response().items.first
        }
        guard let artist else {
            throw CommandError("artist \(id) was not found in the catalog")
        }
        async let topSongs = Self.section(artist, .topSongs, \.topSongs)
        async let essential = Self.section(artist, .featuredAlbums, \.featuredAlbums)
        async let albums = Self.section(artist, .fullAlbums, \.fullAlbums)
        async let singles = Self.section(artist, .singles, \.singles)
        async let compilations = Self.section(artist, .compilationAlbums, \.compilationAlbums)
        async let playlists = Self.artistPlaylists(artist)
        async let loadedFacts = Self.artistFacts(id: id)

        let notes = artist.editorialNotes.flatMap { $0.standard ?? $0.short } ?? ""
        let facts = await loadedFacts
        return [
            "artist": artistJSON(artist),
            "topSongs": await topSongs.map(songJSON),
            "essentialAlbums": await essential.map(albumJSON),
            "albums": await albums.map(albumJSON),
            "singles": await singles.map(albumJSON),
            "compilations": await compilations.map(albumJSON),
            "playlists": await playlists.map(catalogPlaylistJSON),
            "about": [
                "notes": EditorialText.plain(notes),
                "genre": artist.genreNames?.first ?? "",
                "origin": facts.origin,
                "formed": facts.formed,
            ],
        ]
    }

    /// Loads one relationship of artist, or nothing when it fails or takes
    /// longer than `CatalogBudget.section`.
    private nonisolated static func section<Item>(
        _ artist: Artist,
        _ property: MusicRelationshipProperty<Artist, Item>,
        _ items: KeyPath<Artist, MusicItemCollection<Item>?>
    ) async -> [Item] {
        guard let loaded = try? await Deadline.run(seconds: CatalogBudget.section, {
            try await artist.with([property])
        }) else { return [] }
        return Array(loaded[keyPath: items] ?? [])
    }

    /// The artist's playlists, falling back to `featuredPlaylists` only when
    /// `playlists` is empty. Both requests share one section deadline, so
    /// the fallback never stretches the command past its budget.
    private nonisolated static func artistPlaylists(_ artist: Artist) async -> [Playlist] {
        (try? await Deadline.run(seconds: CatalogBudget.section) {
            let own = try await artist.with([.playlists]).playlists ?? []
            if !own.isEmpty { return Array(own) }
            return Array(try await artist.with([.featuredPlaylists]).featuredPlaylists ?? [])
        }) ?? []
    }

    /// Where the artist is from and when it was born or formed. MusicKit
    /// does not model these, so they come from the raw catalog API; any
    /// failure leaves them empty.
    private nonisolated static func artistFacts(id: String) async -> ArtistFacts {
        let none = ArtistFacts(origin: "", formed: "")
        return (try? await Deadline.run(seconds: CatalogBudget.section) {
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

    /// An album page: the album with its tracks (songs only; music videos
    /// are left out) and the facts listed under them.
    private func album(_ request: Request) async throws -> JSONObject {
        let id = try request.requiredID("albumId")
        return try await albumPage(id: id)
    }

    /// The page of the album containing a song: its first album. Two
    /// sequential lookups, within `CatalogBudget.songAlbum`.
    private func songAlbum(_ request: Request) async throws -> JSONObject {
        let id = try request.requiredID("songId")
        let song = try await Deadline.run(seconds: CatalogBudget.lookup) {
            var lookup = MusicCatalogResourceRequest<Song>(matching: \.id, equalTo: MusicItemID(id))
            lookup.properties = [.albums]
            return try await lookup.response().items.first
        }
        guard let song else {
            throw CommandError("song \(id) was not found in the catalog")
        }
        guard let album = song.albums?.first else {
            throw CommandError("song \(id) has no album in the catalog")
        }
        return try await albumPage(id: album.id.rawValue)
    }

    /// Looks up an album with its tracks, within `CatalogBudget.lookup`.
    private func albumPage(id: String) async throws -> JSONObject {
        let album = try await Deadline.run(seconds: CatalogBudget.lookup) {
            var lookup = MusicCatalogResourceRequest<Album>(matching: \.id, equalTo: MusicItemID(id))
            lookup.properties = [.tracks]
            return try await lookup.response().items.first
        }
        guard let album else {
            throw CommandError("album \(id) was not found in the catalog")
        }
        let notes = album.editorialNotes.flatMap { $0.standard ?? $0.short } ?? ""
        return [
            "album": albumJSON(album),
            "tracks": Self.songs(in: album.tracks).map(trackJSON),
            "genre": album.genreNames.first ?? "",
            "releaseDate": album.releaseDate.map(CatalogDate.iso) ?? "",
            "recordLabel": album.recordLabelName ?? "",
            "copyright": album.copyright ?? "",
            "notes": EditorialText.plain(notes),
        ]
    }

    /// A catalog playlist page: its songs in order (music videos are left
    /// out) and its description.
    private func catalogPlaylist(_ request: Request) async throws -> JSONObject {
        let id = try request.requiredID("playlistId")
        let playlist = try await Deadline.run(seconds: CatalogBudget.lookup) {
            var lookup = MusicCatalogResourceRequest<Playlist>(matching: \.id, equalTo: MusicItemID(id))
            lookup.properties = [.tracks]
            return try await lookup.response().items.first
        }
        guard let playlist else {
            throw CommandError("playlist \(id) was not found in the catalog")
        }
        let notes = playlist.standardDescription ?? playlist.shortDescription ?? ""
        return [
            "playlist": catalogPlaylistJSON(playlist),
            "tracks": Self.songs(in: playlist.tracks).map(songJSON),
            "notes": EditorialText.plain(notes),
        ]
    }

    /// The songs of a track list, in order. Only the loaded batch is used:
    /// very long lists are cut at the catalog's first page.
    private nonisolated static func songs(in tracks: MusicItemCollection<Track>?) -> [Song] {
        (tracks ?? []).compactMap { track in
            if case let .song(song) = track { return song }
            return nil
        }
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

    private func albumJSON(_ album: Album) -> JSONObject {
        [
            "id": album.id.rawValue,
            "title": album.title,
            "artist": album.artistName,
            // Read in UTC (see CatalogDate), or 1 January could move back a year.
            "year": album.releaseDate.map { CatalogDate.utc.component(.year, from: $0) } ?? 0,
            "trackCount": album.trackCount,
        ]
    }

    private func catalogPlaylistJSON(_ playlist: Playlist) -> JSONObject {
        ["id": playlist.id.rawValue, "name": playlist.name, "curator": playlist.curatorName ?? ""]
    }

    /// A song on an album, with its position; 0 when unknown.
    private func trackJSON(_ song: Song) -> JSONObject {
        songJSON(song).merging([
            "trackNumber": song.trackNumber ?? 0,
            "discNumber": song.discNumber ?? 0,
        ]) { _, position in position }
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
