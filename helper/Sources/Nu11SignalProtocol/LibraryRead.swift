// Pure pieces of the library read commands (playlists, libraryPlaylist,
// and the queue playPlaylist builds), free of MusicKit so they can be unit
// tested.
//
// The library is read through the Apple Music API rather than
// MusicLibraryRequest: MusicKit on macOS names library items by persistent
// ids the API does not know, so nothing it lists could be edited
// (LibraryEdit) or favorited. Endpoints, as documented at
// https://developer.apple.com/documentation/applemusicapi:
//   Get All Library Playlists           GET /v1/me/library/playlists
//   Get a Library Playlist              GET /v1/me/library/playlists/{id}
//   Get a Library Playlist's Relationship Directly by Name
//                                       GET /v1/me/library/playlists/{id}/tracks
// Collections come in pages of at most `pageLimit` resources; a page's
// `next` is the path (with its offset) of the following one.
import Foundation

/// A library playlist as `playlists` lists it.
public struct LibraryPlaylistSummary: Equatable {
    /// The API library id ("p.…").
    public let id: String
    public let name: String
    /// Whether songs may be added to it (`canEdit`): false for playlists
    /// followed from the catalog.
    public let editable: Bool

    public init(id: String, name: String, editable: Bool) {
        self.id = id
        self.name = name
        self.editable = editable
    }
}

/// A library playlist's own attributes.
public struct LibraryPlaylistInfo: Equatable {
    public let name: String
    public let editable: Bool
    /// The description as the API sends it (possibly HTML); empty for none.
    public let notes: String
}

/// A song of a library playlist.
public struct LibraryTrack: Equatable {
    /// The API library id ("i.…"), or the catalog id for a catalog song
    /// listed as such.
    public let libraryID: String
    /// The catalog id (`playParams.catalogId`), nil for a song that is
    /// only in the library, such as an upload or a song no longer in the
    /// catalog.
    public let catalogID: String?
    public let title: String
    public let artist: String
    public let album: String
    /// In seconds; 0 when unknown.
    public let duration: TimeInterval

    public init(libraryID: String, catalogID: String?, title: String, artist: String, album: String, duration: TimeInterval) {
        self.libraryID = libraryID
        self.catalogID = catalogID
        self.title = title
        self.artist = artist
        self.album = album
        self.duration = duration
    }

    /// The id the UI works with: the catalog id when there is one, so
    /// favorites and catalog playback accept it, else the library id.
    public var id: String { catalogID ?? libraryID }

    /// Whether the helper can play it: only catalog songs can be queued.
    public var playable: Bool { catalogID != nil }

    /// The song on the wire. `libraryOnly` is sent only when true, so a
    /// song without it is playable.
    public var json: JSONObject {
        var song: JSONObject = ["id": id, "title": title, "artist": artist, "album": album, "duration": duration]
        if !playable { song["libraryOnly"] = true }
        return song
    }
}

/// One page of a collection and the call for the next one.
public struct LibraryPage<Item> {
    public let items: [Item]
    /// The next page, or nil on the last one.
    public let next: MusicAPICall?
    /// How many resources the API sent, items left out included: a page
    /// whose resources were all left out (music videos) is not the end.
    public let resources: Int
}

public enum LibraryRead {
    /// The most resources the API returns per page.
    public static let pageLimit = 100
    /// Upper bound on the playlists `playlists` lists.
    public static let maxPlaylists = 500
    /// Upper bound on the songs read from one playlist.
    public static let maxPlaylistTracks = 1000

    private static let playlistsPath = "/v1/me/library/playlists"
    private static let firstPage = [URLQueryItem(name: "limit", value: String(pageLimit))]

    /// The first page of the library playlists (alphabetical, per the API).
    public static let playlistsCall = MusicAPICall(method: "GET", path: playlistsPath, query: firstPage, body: nil)

    /// The playlist in `playlistId` itself.
    public static func playlistCall(_ request: Request) throws -> MusicAPICall {
        MusicAPICall(method: "GET", path: "\(playlistsPath)/\(try playlistID(request))", body: nil)
    }

    /// The first page of the tracks of the playlist in `playlistId`.
    public static func tracksCall(_ request: Request) throws -> MusicAPICall {
        MusicAPICall(method: "GET", path: "\(playlistsPath)/\(try playlistID(request))/tracks", query: firstPage, body: nil)
    }

    /// A page of `playlists`. Items without a usable id are left out.
    public static func playlistsPage(_ data: Data, of call: MusicAPICall) throws -> LibraryPage<LibraryPlaylistSummary> {
        let (resources, next) = try page(data, of: call)
        let items = resources.compactMap { resource -> LibraryPlaylistSummary? in
            guard let id = resource["id"] as? String, (try? APIPathID.checked(id)) != nil else { return nil }
            let attributes = resource["attributes"] as? JSONObject ?? [:]
            return LibraryPlaylistSummary(
                id: id, name: attributes["name"] as? String ?? "", editable: attributes["canEdit"] as? Bool ?? false)
        }
        return LibraryPage(items: items, next: next, resources: resources.count)
    }

    /// A page of a playlist's tracks: its songs, library or catalog, in
    /// order. Music videos, and items without an id, are left out.
    public static func tracksPage(_ data: Data, of call: MusicAPICall) throws -> LibraryPage<LibraryTrack> {
        let (resources, next) = try page(data, of: call)
        let items = resources.compactMap { resource -> LibraryTrack? in
            guard let id = resource["id"] as? String, !id.isEmpty else { return nil }
            let type = resource["type"] as? String
            guard type == "library-songs" || type == "songs" else { return nil }
            let attributes = resource["attributes"] as? JSONObject ?? [:]
            let playParams = attributes["playParams"] as? JSONObject
            var catalogID = type == "songs" ? id : playParams?["catalogId"] as? String
            if let candidate = catalogID, (try? APIPathID.checked(candidate)) == nil { catalogID = nil }
            let millis = (attributes["durationInMillis"] as? NSNumber)?.doubleValue ?? 0
            return LibraryTrack(
                libraryID: id, catalogID: catalogID,
                title: attributes["name"] as? String ?? "",
                artist: attributes["artistName"] as? String ?? "",
                album: attributes["albumName"] as? String ?? "",
                duration: millis.isFinite && millis > 0 ? millis / 1000 : 0)
        }
        return LibraryPage(items: items, next: next, resources: resources.count)
    }

    /// The playlist a `playlistCall` answer holds.
    public static func playlistInfo(_ data: Data) throws -> LibraryPlaylistInfo {
        guard let resource = try page(data, of: nil).resources.first else {
            throw ArgumentError(description: "the Apple Music API did not return the playlist")
        }
        let attributes = resource["attributes"] as? JSONObject ?? [:]
        let description = attributes["description"] as? JSONObject
        return LibraryPlaylistInfo(
            name: attributes["name"] as? String ?? "",
            editable: attributes["canEdit"] as? Bool ?? false,
            notes: description?["standard"] as? String ?? description?["short"] as? String ?? "")
    }

    /// Fetches `first` and the pages after it, collecting at most cap
    /// items in order. It stops at the last page, at the cap (no page is
    /// fetched past it), at a page the API sent empty, or at a `next` whose
    /// offset was already fetched, so a `next` that points back can neither
    /// loop nor list a resource twice.
    public static func collect<Item>(
        from first: MusicAPICall,
        cap: Int,
        fetch: (MusicAPICall) async throws -> Data,
        page read: (Data, MusicAPICall) throws -> LibraryPage<Item>
    ) async throws -> [Item] {
        var items: [Item] = []
        var fetched: Set<Int> = []
        var call: MusicAPICall? = first
        while let current = call, items.count < cap, fetched.insert(offset(of: current)).inserted {
            let page = try read(try await fetch(current), current)
            items += page.items
            call = page.resources == 0 ? nil : page.next
        }
        return Array(items.prefix(cap))
    }

    /// The offset a page call starts at: its `offset` query item, 0 when
    /// absent (the first page) or unreadable.
    static func offset(of call: MusicAPICall) -> Int {
        call.query.last { $0.name == "offset" }?.value.flatMap { Int($0) } ?? 0
    }

    private static func playlistID(_ request: Request) throws -> String {
        try LibraryEdit.playlistID(request.requiredID("playlistId"))
    }

    /// The resources of a collection page and the call for the next page
    /// of call's resource. A `next` must be a path under call's own path,
    /// so a page can never send the helper to another resource.
    private static func page(_ data: Data, of call: MusicAPICall?) throws -> (resources: [JSONObject], next: MusicAPICall?) {
        guard let object = try? JSONSerialization.jsonObject(with: data) as? JSONObject,
              let resources = object["data"] as? [JSONObject] else {
            throw ArgumentError(description: "the Apple Music API answered with an unreadable page")
        }
        guard let call, let link = object["next"] as? String else { return (resources, nil) }
        guard link.hasPrefix("/"), !link.hasPrefix("//"),
              let parts = URLComponents(string: link),
              parts.scheme == nil, parts.host == nil,
              parts.path == call.path, !parts.path.contains("..") else {
            throw ArgumentError(description: "the Apple Music API answered with an unexpected next page")
        }
        return (resources, MusicAPICall(method: "GET", path: parts.path, query: parts.queryItems ?? [], body: nil))
    }
}

/// What `playPlaylist` queues for a library playlist: the catalog ids of
/// its playable songs, in order, and the position of the start song among
/// them. Songs only in the library are skipped.
public struct LibraryQueuePlan: Equatable {
    public let ids: [String]
    public let start: Int

    public init(ids: [String], start: Int) {
        self.ids = ids
        self.start = start
    }

    /// The plan for tracks, starting at the first playable song or, with
    /// startIndex, at tracks[startIndex], which must be playable.
    public init(_ tracks: [LibraryTrack], startIndex: Int?) throws {
        guard !tracks.isEmpty else { throw ArgumentError(description: "the playlist has no songs") }
        var start = 0
        if let startIndex {
            let chosen = try PlaylistStart.item(in: tracks, at: startIndex)
            guard chosen.playable else {
                throw ArgumentError(description:
                    "\"\(chosen.title)\" is not in the Apple Music catalog, so it cannot be played here")
            }
            start = tracks[..<startIndex].filter(\.playable).count
        }
        let ids = tracks.compactMap(\.catalogID)
        guard !ids.isEmpty else {
            throw ArgumentError(description: "none of the playlist's songs are in the Apple Music catalog")
        }
        self.init(ids: ids, start: start)
    }
}

/// Catalog lookups by id go in batches: the API serves at most `size` songs
/// per request.
public enum CatalogBatches {
    public static let size = 300

    public static func split(_ ids: [String], size: Int) -> [[String]] {
        stride(from: 0, to: ids.count, by: size).map { Array(ids[$0..<min($0 + size, ids.count)]) }
    }
}
