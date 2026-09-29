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
    private let volume = AppVolume.shared
    /// When the playback command running now times out (see `respond`);
    /// playback commands run one at a time, so one is enough.
    private var playbackDeadline = Date.distantFuture

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

    /// Upper bound for the local library lookup `playSongs` makes before
    /// queueing (see `libraryCopies`); it reads this Mac's library only,
    /// so it stays a small part of `playbackTimeout`.
    static let libraryCopiesTimeout: TimeInterval = 1.5

    /// Runs one request and sends exactly one response. With a timeout, a
    /// command that has not finished in time is answered with an error.
    func respond(to request: Request, timeout: TimeInterval? = nil) async {
        do {
            let result: JSONObject
            if let timeout {
                playbackDeadline = Date().addingTimeInterval(timeout)
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
        case "libraryPlaylist": return try await libraryPlaylist(request)
        case "createPlaylist": return try await createPlaylist(request)
        case "addToPlaylist": _ = try await Self.edit(LibraryEdit.addToPlaylist(request), for: request.cmd); return [:]
        case "favorite": return try await favorite(request)
        case "favorites": return try await favorites(request)
        case "setFavorite": try await setFavorite(request); return [:]
        case "volume": return try volumeJSON()
        case "setVolume": try volume.set(VolumeLevel.requested(request)); return try volumeJSON()
        case "playSongs": return try await playSongs(request)
        case "playPlaylist": return try await playPlaylist(request)
        case "pause": player.pause(); return [:]
        case "resume": volume.prepareForPlayback(); try await player.play(); return [:]
        case "next": try await player.skipToNextEntry(); return [:]
        case "previous": try await player.skipToPreviousEntry(); return [:]
        case "stop": player.stop(); emitter.catalogIDs = [:]; return [:]
        case "seek": return try seek(request)
        case "setRepeat": player.state.repeatMode = Self.repeatMode(try RepeatSetting.requested(request)); return [:]
        default: throw CommandError("unknown command: \(request.cmd)")
        }
    }

    /// `{"level": 0...1, "mode": "app"|"system"}`: the app volume's level,
    /// or the system volume's (see AppVolume).
    private func volumeJSON() throws -> JSONObject {
        let current = try volume.current()
        return ["level": current.level, "mode": current.mode.rawValue]
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
            let storefront = try APIPathID.checked(try await MusicDataRequest.currentCountryCode)
            var url = URLComponents()
            url.scheme = "https"
            url.host = "api.music.apple.com"
            url.path = "/v1/catalog/\(storefront)/artists/\(try APIPathID.checked(id))"
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

    /// The library playlists, in the API's (alphabetical) order, with
    /// their API library ids ("p.…"), within `CatalogBudget.libraryRead`.
    private func playlists() async throws -> JSONObject {
        let playlists = try await Deadline.run(seconds: CatalogBudget.libraryRead) {
            try await LibraryRead.collect(
                from: LibraryRead.playlistsCall, cap: LibraryRead.maxPlaylists,
                fetch: { try await Self.data(for: $0, command: "playlists") },
                page: LibraryRead.playlistsPage)
        }
        return ["playlists": playlists.map { ["id": $0.id, "name": $0.name, "editable": $0.editable] as JSONObject }]
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
        return try await playCatalogSongs(ids, from: startIndex, context: "playSongs")
    }

    /// Looks up the catalog songs and plays them from `ids[startIndex]`
    /// (see `playSongs`). The result lists the ids the catalog did not
    /// return as `"missing"`, the songs left out because the player cannot
    /// queue them (see `PreparedQueue`) as `"skipped"`, and `"startedAlone":
    /// true` when only the start song could be queued (see `play`).
    private func playCatalogSongs(_ ids: [String], from startIndex: Int, context: String) async throws -> JSONObject {
        let found = try await Self.catalogSongs(ids)
        // The catalog may return songs in any order; SongQueue restores the
        // requested one and turns startIndex into a position in the queue.
        let queue = try SongQueue(ids: ids, found: found, id: \.id.rawValue, startIndex: startIndex)
        let prepared = PreparedQueue(
            items: queue.items, start: queue.start, id: \.id.rawValue,
            libraryCopies: await Self.libraryCopies(of: queue.items))
        if !prepared.skipped.isEmpty {
            log("\(context): left out \(prepared.skipped.count) songs of the local library "
                + "(\(prepared.skipped.joined(separator: ", "))): a catalog start cannot queue them")
        }
        // Set before the queue changes: the player announces the new entry
        // as soon as it is handed the queue. A failed start leaves no queue
        // to name (the player is stopped), so the mapping goes with it.
        emitter.catalogIDs = prepared.catalogIDs
        let startedAlone: Bool
        do {
            startedAlone = try await play(prepared.items, from: prepared.start, context: context)
        } catch {
            emitter.catalogIDs = [:]
            throw error
        }
        var result: JSONObject = [:]
        if !queue.missing.isEmpty { result["missing"] = queue.missing }
        if !prepared.skipped.isEmpty { result["skipped"] = prepared.skipped }
        if startedAlone { result["startedAlone"] = true }
        return result
    }

    /// The local library's copies of songs, keyed by catalog id: one
    /// library request, on this Mac only. A failed lookup finds none, and
    /// the queue is handed over as catalog songs.
    private nonisolated static func libraryCopies(of songs: [Song]) async -> [String: Song] {
        var request = MusicLibraryRequest<Song>()
        request.filter(matching: \.id, memberOf: Array(Set(songs.map(\.id))))
        guard let items = try? await Deadline.run(seconds: libraryCopiesTimeout, { try await request.response().items }) else {
            return [:]
        }
        var copies: [String: Song] = [:]
        for song in items {
            guard let parameters = song.playParameters,
                  let json = try? JSONEncoder().encode(parameters),
                  let catalogID = PreparedQueue<Song>.catalogID(playParameters: json)
            else { continue }
            copies[catalogID] = copies[catalogID] ?? song
        }
        return copies
    }

    /// The catalog songs with the given ids, in any order, looked up in
    /// concurrent batches of `CatalogBatches.size` ids.
    private nonisolated static func catalogSongs(_ ids: [String]) async throws -> [Song] {
        try await withThrowingTaskGroup(of: [Song].self) { group in
            for batch in CatalogBatches.split(Array(Set(ids)), size: CatalogBatches.size) {
                group.addTask {
                    let lookup = MusicCatalogResourceRequest<Song>(matching: \.id, memberOf: batch.map { MusicItemID($0) })
                    return Array(try await lookup.response().items)
                }
            }
            return try await group.reduce(into: []) { $0 += $1 }
        }
    }

    /// Replaces the queue with songs and plays from `songs[start]`;
    /// returns whether the start song had to be queued on its own.
    ///
    /// Each song gets its own queue entry and the start is named as that
    /// entry, so the player never has to find the start by matching a song
    /// (`Queue(for:startingAt:)` does, and a song listed twice matches its
    /// first copy). If the player cannot prepare the queue (Code=6, see
    /// QueueStart), playback is stopped and the start song is queued on its
    /// own, which prepares where the whole queue does not; once it plays,
    /// the songs after it are appended (`QueueStart.followers`), so the
    /// list still plays on. Any other error, or a failure of that fallback,
    /// is reported naming the song; a failed append is only logged, the
    /// start song playing on alone. Every step shares the command's
    /// `playbackTimeout`: the append gets what is left of it, so it cannot
    /// change the queue after the command was answered.
    ///
    /// The fallback is logged to stderr with context (the command) and the
    /// player's reason, so a Code=6 that keeps happening shows in the
    /// helper log. The caller guarantees `songs.indices.contains(start)`.
    private func play(_ songs: [Song], from start: Int, context: String) async throws -> Bool {
        func startQueue(_ songs: [Song], at start: Int) async throws {
            let entries = songs.map { MusicPlayer.Queue.Entry($0) }
            player.queue = ApplicationMusicPlayer.Queue(entries, startingAt: entries[start])
            volume.prepareForPlayback()
            try await player.play()
        }
        let song = songs[start]
        let startedAlone: Bool
        do {
            startedAlone = try await QueueStart.startWithFallback({
                try await startQueue(songs, at: start)
            }, fallback: {
                try await startQueue([song], at: 0)
            }, beforeFallback: { error in
                log("\(context): \(QueueStart.failure(error, song: song.title)) starting \(songs.count) songs at "
                    + "\(start) (song \(song.id.rawValue)); queueing that song alone")
                player.stop()
            })
        } catch {
            throw QueueStart.failure(error, song: song.title)
        }
        let followers = QueueStart.followers(of: songs, after: start)
        let remaining = playbackDeadline.timeIntervalSinceNow
        if startedAlone, !followers.isEmpty, remaining <= 0 {
            log("\(context): no time left to append the \(followers.count) songs after \"\(song.title)\"; it plays alone")
        } else if startedAlone, !followers.isEmpty {
            let entries = followers.map { MusicPlayer.Queue.Entry($0) }
            let queue = player.queue
            do {
                try await Deadline.run(seconds: remaining) { try await queue.insert(entries, position: .tail) }
            } catch {
                log("\(context): could not append the \(followers.count) songs after \"\(song.title)\" "
                    + "(\(QueueStart.failure(error, song: song.title))); it plays alone")
            }
        }
        return startedAlone
    }

    /// A library playlist page: its songs in order (music videos are left
    /// out) and its description. A song's id is its catalog id when it has
    /// one, else its API library id with `"libraryOnly": true` (it cannot
    /// be played). The playlist and its song pages load concurrently,
    /// within `CatalogBudget.libraryRead`.
    private func libraryPlaylist(_ request: Request) async throws -> JSONObject {
        let playlistCall = try LibraryRead.playlistCall(request)
        let tracksCall = try LibraryRead.tracksCall(request)
        return try await Deadline.run(seconds: CatalogBudget.libraryRead) {
            async let info = LibraryRead.playlistInfo(try await Self.data(for: playlistCall, command: request.cmd))
            async let tracks = Self.tracks(from: tracksCall, command: request.cmd)
            let playlist = try await info
            return [
                "playlist": ["id": try request.requiredID("playlistId"), "name": playlist.name],
                "tracks": try await tracks.map(\.json),
                "notes": EditorialText.plain(playlist.notes),
            ]
        }
    }

    /// Plays a library playlist from its first song or, with `startIndex`,
    /// from that song of the list `libraryPlaylist` returns.
    ///
    /// The playlist is queued as its catalog songs (see `LibraryQueuePlan`):
    /// the player can only be handed catalog songs here, as library items
    /// read through the Apple Music API are not MusicKit library items.
    /// Songs only in the library are skipped, and the start keeps pointing
    /// at the chosen song; choosing one of them is an error. The list is
    /// read again, so a playlist changed since `libraryPlaylist` listed it
    /// may shift the start (a start past the end is reported).
    private func playPlaylist(_ request: Request) async throws -> JSONObject {
        let tracksCall = try LibraryRead.tracksCall(request)
        let start = try PlaylistStart.index(request)
        let tracks = try await Self.tracks(from: tracksCall, command: request.cmd)
        let plan = try LibraryQueuePlan(tracks, startIndex: start)
        let id = try request.requiredID("playlistId")
        return try await playCatalogSongs(plan.ids, from: plan.start, context: "playPlaylist \(id)")
    }

    /// The songs of a library playlist, in order, up to
    /// `LibraryRead.maxPlaylistTracks`. `libraryPlaylist` and
    /// `playPlaylist` share it, so a `startIndex` means the same song.
    private nonisolated static func tracks(from call: MusicAPICall, command: String) async throws -> [LibraryTrack] {
        try await LibraryRead.collect(
            from: call, cap: LibraryRead.maxPlaylistTracks,
            fetch: { try await data(for: $0, command: command) },
            page: LibraryRead.tracksPage)
    }

    /// Creates a library playlist, with its songs when `songIds` lists any,
    /// and answers with its API library id ("p.…") and name. The new
    /// playlist may take a moment to appear in `playlists`.
    private func createPlaylist(_ request: Request) async throws -> JSONObject {
        let call = try LibraryEdit.createPlaylist(request)
        let data = try await Self.edit(call, for: request.cmd)
        let created = try LibraryEdit.createdPlaylist(data, requestedName: request.string("name") ?? "")
        return ["id": created.id, "name": created.name]
    }

    /// Whether a song is a favorite (loved). A song without a rating is
    /// answered with 404, which means false.
    private func favorite(_ request: Request) async throws -> JSONObject {
        let call = try LibraryEdit.favorite(request)
        return ["favorite": try LibraryEdit.favoriteAnswer(await Self.result { try await Self.edit(call, for: request.cmd) })]
    }

    /// Whether each of `songIds` is a favorite (loved), as
    /// `{"favorites": {"<id>": true|false}}`: one ratings read per kind of
    /// id and batch of `LibraryEdit.maxRatingIDs`, concurrently, within
    /// `CatalogBudget.libraryRead`. Any failed read fails the command.
    private func favorites(_ request: Request) async throws -> JSONObject {
        let ids = try LibraryEdit.favoriteIDs(request)
        let calls = try LibraryEdit.favoritesCalls(ids)
        let loved = try await Deadline.run(seconds: CatalogBudget.libraryRead) {
            try await withThrowingTaskGroup(of: Set<String>.self) { group in
                for call in calls {
                    group.addTask { try LibraryEdit.lovedIDs(await Self.result { try await Self.data(for: call, command: request.cmd) }) }
                }
                return try await group.reduce(into: Set<String>()) { $0.formUnion($1) }
            }
        }
        return LibraryEdit.favoritesAnswer(ids, loved: loved)
    }

    /// Loves a song or clears its rating; clearing a song without a rating
    /// succeeds (see `LibraryEdit.setFavoriteAnswer`).
    private func setFavorite(_ request: Request) async throws {
        let call = try LibraryEdit.setFavorite(request)
        let on = request.bool("on") ?? false
        try LibraryEdit.setFavoriteAnswer(on: on, await Self.result { try await Self.edit(call, for: request.cmd) })
    }

    private nonisolated static func result(_ run: () async throws -> Data) async -> Result<Data, Error> {
        do { return .success(try await run()) } catch { return .failure(error) }
    }

    /// Sends one library edit within `CatalogBudget.libraryEdit` and returns
    /// the response body. A write that times out is reported as having an
    /// unknown outcome (see `LibraryEdit.settled`) and never retried.
    private nonisolated static func edit(_ call: MusicAPICall, for command: String) async throws -> Data {
        do {
            return try await Deadline.run(seconds: CatalogBudget.libraryEdit) {
                try await data(for: call, command: command)
            }
        } catch {
            throw LibraryEdit.settled(command, error)
        }
    }

    /// Sends one Apple Music API request and returns the response body.
    /// MusicDataRequest adds the developer and user tokens; an error status
    /// becomes a MusicAPIFailure whose message names the likely cause.
    private nonisolated static func data(for call: MusicAPICall, command: String) async throws -> Data {
        var url = URLComponents()
        url.scheme = "https"
        url.host = "api.music.apple.com"
        url.path = call.path
        if !call.query.isEmpty { url.queryItems = call.query }
        guard let url = url.url else {
            throw CommandError("\(command): invalid Apple Music API path \(call.path)")
        }
        var urlRequest = URLRequest(url: url)
        urlRequest.httpMethod = call.method
        if let body = call.body {
            urlRequest.httpBody = body
            urlRequest.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        do {
            return try await MusicDataRequest(urlRequest: urlRequest).response().data
        } catch let error as MusicDataRequest.Error {
            throw MusicAPIFailure(command: command, status: error.status, title: error.title, detail: error.detailText)
        }
    }

    /// The player's repeat mode for a wire name (see RepeatSetting).
    private static func repeatMode(_ mode: String) -> MusicPlayer.RepeatMode {
        switch mode {
        case "all": return .all
        case "one": return .one
        default: return .none
        }
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
