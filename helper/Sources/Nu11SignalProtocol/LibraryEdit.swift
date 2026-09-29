// Pure pieces of the library edit commands (createPlaylist, addToPlaylist,
// favorite, setFavorite), free of MusicKit so they can be unit tested.
//
// MusicKit's MusicLibrary editing API is unavailable on macOS and it has no
// favorites API, so these commands call the Apple Music API
// (https://api.music.apple.com) through MusicDataRequest, which signs each
// request with the developer and user tokens. Endpoints, as documented at
// https://developer.apple.com/documentation/applemusicapi:
//   Create a New Library Playlist      POST   /v1/me/library/playlists
//   Add Tracks to a Library Playlist   POST   /v1/me/library/playlists/{id}/tracks
//   Get/Add/Delete a Personal (Library) Song Rating
//                                      GET|PUT|DELETE /v1/me/ratings/{songs|library-songs}/{id}
//   Get Multiple Personal (Library) Song(s) Ratings
//                                      GET /v1/me/ratings/{songs|library-songs}?ids=a,b
import Foundation

/// One Apple Music API request: the helper turns it into a URLRequest.
public struct MusicAPICall: Equatable {
    public let method: String
    /// The path below https://api.music.apple.com.
    public let path: String
    /// The query string's items, in order; empty for none.
    public let query: [URLQueryItem]
    /// The JSON body, or nil for none.
    public let body: Data?

    public init(method: String, path: String, query: [URLQueryItem] = [], body: Data?) {
        self.method = method
        self.path = path
        self.query = query
        self.body = body
    }
}

/// An Apple Music API request answered with an error status. description
/// is the message for the UI (see LibraryEdit.failureMessage).
public struct MusicAPIFailure: Error, CustomStringConvertible, Equatable {
    public let status: Int
    public let description: String

    /// The failure of command, from the status and the API's error title
    /// and detail (MusicDataRequest.Error); the detail wins when present.
    public init(command: String, status: Int, title: String, detail: String) {
        self.status = status
        self.description = LibraryEdit.failureMessage(
            command: command, status: status, detail: detail.isEmpty ? title : detail)
    }
}

/// Ids interpolated into Apple Music API paths.
public enum APIPathID {
    /// id when it is safe as one path segment: ASCII letters, digits and
    /// dots only, and no "..", so it can neither add a segment nor climb
    /// out of the resource. Every id that reaches a path passes here.
    public static func checked(_ id: String) throws -> String {
        let allowed = !id.isEmpty && id.unicodeScalars.allSatisfy { scalar in
            scalar.isASCII && (CharacterSet.alphanumerics.contains(scalar) || scalar == ".")
        }
        guard allowed, id != ".", !id.contains("..") else {
            throw ArgumentError(description: "\"\(id)\" is not a valid Apple Music id")
        }
        return id
    }
}

/// The playlist `createPlaylist` answers with.
public struct CreatedPlaylist: Equatable {
    public let id: String
    public let name: String

    public init(id: String, name: String) {
        self.id = id
        self.name = name
    }
}

public enum LibraryEdit {
    /// The longest id read as a catalog song id. Catalog ids are short
    /// decimal numbers (10 digits today); MusicKit on macOS names library
    /// songs by 64-bit persistent ids (up to 19 digits, possibly negative),
    /// which the Apple Music API does not know. A random 64-bit id has 12
    /// digits or fewer about once in ten million.
    static let maxCatalogIDDigits = 12

    /// The Apple Music API resource type of a song id: "library-songs" for
    /// an API library id ("i.…"), "songs" for a catalog id. Any other id,
    /// such as a MusicKit persistent id, is an error, as is one unsafe in
    /// a path (see APIPathID).
    public static func songType(_ id: String) throws -> String {
        _ = try APIPathID.checked(id)
        if id.hasPrefix("i."), id.count > 2 { return "library-songs" }
        if (1...maxCatalogIDDigits).contains(id.count), id.allSatisfy({ $0.isASCII && $0.isNumber }) {
            return "songs"
        }
        throw ArgumentError(description:
            "song \"\(id)\" has no Apple Music API id")
    }

    /// A library playlist id the Apple Music API accepts ("p.…", as
    /// `playlists` lists and `createPlaylist` returns). MusicKit's
    /// persistent ids are an error.
    public static func playlistID(_ id: String) throws -> String {
        guard id.hasPrefix("p."), id.count > 2 else {
            throw ArgumentError(description: "playlist \"\(id)\" has no Apple Music API id")
        }
        return try APIPathID.checked(id)
    }

    /// `createPlaylist` with `name`, an optional `description` and optional
    /// `songIds`, in order. An empty description or song list is left out.
    public static func createPlaylist(_ request: Request) throws -> MusicAPICall {
        guard let name = request.string("name"), !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw ArgumentError(description: "\(request.cmd) requires a non-empty \"name\"")
        }
        var attributes: JSONObject = ["name": name]
        if request.args["description"] != nil {
            guard let description = request.string("description") else {
                throw ArgumentError(description: "\"description\" must be a string")
            }
            if !description.isEmpty { attributes["description"] = description }
        }
        var body: JSONObject = ["attributes": attributes]
        var songIDs: [String] = []
        if request.args["songIds"] != nil {
            guard let ids = request.strings("songIds") else {
                throw ArgumentError(description: "\"songIds\" must be an array of strings")
            }
            songIDs = ids
        }
        if !songIDs.isEmpty {
            body["relationships"] = ["tracks": ["data": try tracks(songIDs)]]
        }
        return MusicAPICall(method: "POST", path: "/v1/me/library/playlists", body: try json(body))
    }

    /// `addToPlaylist`: appends `songIds`, in order, to `playlistId`.
    public static func addToPlaylist(_ request: Request) throws -> MusicAPICall {
        let playlist = try playlistID(request.requiredID("playlistId"))
        guard let ids = request.strings("songIds"), !ids.isEmpty else {
            throw ArgumentError(description: "\(request.cmd) requires a non-empty \"songIds\" array")
        }
        return MusicAPICall(
            method: "POST", path: "/v1/me/library/playlists/\(playlist)/tracks",
            body: try json(["data": try tracks(ids)]))
    }

    /// `favorite`: reads the rating of `songId`. The API answers 404 when
    /// the song has none, which means "not a favorite".
    public static func favorite(_ request: Request) throws -> MusicAPICall {
        MusicAPICall(method: "GET", path: try ratingPath(request), body: nil)
    }

    /// The most ids one ratings read asks for; longer lists are split.
    public static let maxRatingIDs = 100

    /// The `songIds` of `favorites`: an array of strings, possibly empty.
    public static func favoriteIDs(_ request: Request) throws -> [String] {
        guard let ids = request.strings("songIds") else {
            throw ArgumentError(description: #"\#(request.cmd) requires a "songIds" array of strings"#)
        }
        return ids
    }

    /// The ratings reads that answer `favorites` for ids: library ids
    /// ("i.…") and catalog ids go to their own endpoint, each without
    /// repeats and in batches of `maxRatingIDs`. Every id is checked (see
    /// `songType`) before it goes into a query, so none can add a
    /// parameter or another id.
    public static func favoritesCalls(_ ids: [String]) throws -> [MusicAPICall] {
        var byType: [(type: String, ids: [String])] = []
        var seen: Set<String> = []
        for id in ids {
            let type = try songType(id)
            guard seen.insert(id).inserted else { continue }
            if let index = byType.firstIndex(where: { $0.type == type }) {
                byType[index].ids.append(id)
            } else {
                byType.append((type, [id]))
            }
        }
        return byType.flatMap { group in
            CatalogBatches.split(group.ids, size: maxRatingIDs).map { batch in
                MusicAPICall(
                    method: "GET", path: "/v1/me/ratings/\(group.type)",
                    query: [URLQueryItem(name: "ids", value: batch.joined(separator: ","))], body: nil)
            }
        }
    }

    /// The loved ids (rating value 1) in one ratings read. Songs without a
    /// rating are simply absent; a 404 is read as none rated.
    public static func lovedIDs(_ result: Result<Data, Error>) throws -> Set<String> {
        switch result {
        case .success(let data):
            return Set(try ratingItems(data).compactMap { item in
                let value = (item["attributes"] as? JSONObject)?["value"] as? NSNumber
                return value?.intValue == 1 ? item["id"] as? String : nil
            })
        case .failure(let failure as MusicAPIFailure) where failure.status == 404:
            return []
        case .failure(let error):
            throw error
        }
    }

    /// The answer to `favorites`: every requested id, loved or not.
    public static func favoritesAnswer(_ ids: [String], loved: Set<String>) -> JSONObject {
        ["favorites": Dictionary(ids.map { ($0, loved.contains($0)) }) { first, _ in first }]
    }

    /// `setFavorite`: `on` true loves `songId` (rating 1, what Apple Music
    /// shows as Favorite); false removes its rating.
    public static func setFavorite(_ request: Request) throws -> MusicAPICall {
        let path = try ratingPath(request)
        guard let on = request.bool("on") else {
            throw ArgumentError(description: "\(request.cmd) requires a boolean \"on\"")
        }
        guard on else { return MusicAPICall(method: "DELETE", path: path, body: nil) }
        return MusicAPICall(method: "PUT", path: path, body: try json(["type": "rating", "attributes": ["value": 1]]))
    }

    /// The new playlist in a creation response (`data[0]`); its name falls
    /// back to the requested one.
    public static func createdPlaylist(_ data: Data, requestedName: String) throws -> CreatedPlaylist {
        guard let item = firstItem(data), let id = item["id"] as? String, !id.isEmpty else {
            throw ArgumentError(description: "the Apple Music API did not return the new playlist")
        }
        let name = (item["attributes"] as? JSONObject)?["name"] as? String
        return CreatedPlaylist(id: id, name: name ?? requestedName)
    }

    /// Whether a rating response loves the song (`value` 1). A dislike
    /// (-1), or a rating without a value, is not a favorite; a body that
    /// is not a ratings answer is an error (see `ratingItems`).
    public static func isFavorite(_ data: Data) throws -> Bool {
        let attributes = try ratingItems(data).first?["attributes"] as? JSONObject
        return (attributes?["value"] as? NSNumber)?.intValue == 1
    }

    /// The ratings of a ratings response (its `data` array). A body without
    /// one is an error: read as no ratings, it would report every song as
    /// not loved, and a toggle would then love a song already loved.
    static func ratingItems(_ data: Data) throws -> [JSONObject] {
        let object = try? JSONSerialization.jsonObject(with: data) as? JSONObject
        guard let items = object?["data"] as? [JSONObject] else {
            throw ArgumentError(description: "the Apple Music API sent an unreadable ratings answer")
        }
        return items
    }

    /// The answer to `favorite`: whether the rating read loves the song.
    /// The API answers 404 for a song without a rating: not a favorite.
    public static func favoriteAnswer(_ result: Result<Data, Error>) throws -> Bool {
        switch result {
        case .success(let data): return try isFavorite(data)
        case .failure(let failure as MusicAPIFailure) where failure.status == 404: return false
        case .failure(let error): throw error
        }
    }

    /// The answer to `setFavorite`. Clearing the rating of a song that has
    /// none is answered with 404; the song is then already not a favorite,
    /// so that succeeds.
    public static func setFavoriteAnswer(on: Bool, _ result: Result<Data, Error>) throws {
        switch result {
        case .success: return
        case .failure(let failure as MusicAPIFailure) where !on && failure.status == 404: return
        case .failure(let error): throw error
        }
    }

    /// The error command reports for error. A write that timed out may
    /// still be applied later (MusicDataRequest cannot be cancelled), so
    /// its message says the outcome is unknown instead of that it failed;
    /// it is never retried automatically, which could apply it twice.
    public static func settled(_ command: String, _ error: Error) -> Error {
        guard let timeout = error as? Deadline.TimedOut else { return error }
        let outcome: String
        switch command {
        case "createPlaylist":
            outcome = "the playlist may or may not have been created; check the library before trying again"
        case "addToPlaylist":
            outcome = "the songs may or may not have been added; check the playlist before trying again"
        default:
            return error
        }
        return ArgumentError(description: "\(command) \(timeout.description): \(outcome)")
    }

    /// A failed request as a message for the UI. detail is the API's
    /// error detail (or title), possibly empty.
    public static func failureMessage(command: String, status: Int, detail: String) -> String {
        let cause = detail.isEmpty ? "HTTP \(status)" : detail
        switch status {
        case 401:
            return "Apple Music did not accept the credentials: check the subscription and sign-in (\(cause))"
        case 403 where command == "addToPlaylist":
            return "playlist is not editable (\(cause))"
        case 403:
            return "Apple Music refused the request: check the subscription and that the app may access your library (\(cause))"
        case 404:
            return "not found in the Apple Music library (\(cause))"
        case 400..<500:
            return "Apple Music rejected the request (HTTP \(status)\(detail.isEmpty ? "" : ": \(detail)"))"
        default:
            return "Apple Music failed (HTTP \(status)\(detail.isEmpty ? "" : ": \(detail)"))"
        }
    }

    private static func ratingPath(_ request: Request) throws -> String {
        let id = try request.requiredID("songId")
        return "/v1/me/ratings/\(try songType(id))/\(id)"
    }

    private static func tracks(_ ids: [String]) throws -> [JSONObject] {
        try ids.map { ["id": $0, "type": try songType($0)] }
    }

    private static func json(_ object: JSONObject) throws -> Data {
        try JSONSerialization.data(withJSONObject: object, options: [.sortedKeys, .withoutEscapingSlashes])
    }

    private static func firstItem(_ data: Data) -> JSONObject? {
        let object = try? JSONSerialization.jsonObject(with: data) as? JSONObject
        return (object?["data"] as? [JSONObject])?.first
    }
}
