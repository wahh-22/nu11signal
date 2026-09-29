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
import Foundation

/// One Apple Music API request: the helper turns it into a URLRequest.
public struct MusicAPICall: Equatable {
    public let method: String
    /// The path below https://api.music.apple.com.
    public let path: String
    /// The JSON body, or nil for none.
    public let body: Data?

    public init(method: String, path: String, body: Data?) {
        self.method = method
        self.path = path
        self.body = body
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
    /// such as the persistent ids `libraryPlaylist` lists, is an error.
    public static func songType(_ id: String) throws -> String {
        if id.hasPrefix("i."), id.count > 2 { return "library-songs" }
        if (1...maxCatalogIDDigits).contains(id.count), id.allSatisfy({ $0.isASCII && $0.isNumber }) {
            return "songs"
        }
        throw ArgumentError(description:
            "song \"\(id)\" has no Apple Music API id (library songs listed by this Mac are not supported yet)")
    }

    /// A library playlist id the Apple Music API accepts ("p.…", as
    /// `createPlaylist` returns). The persistent ids `playlists` lists are
    /// an error.
    public static func playlistID(_ id: String) throws -> String {
        guard id.hasPrefix("p."), id.count > 2 else {
            throw ArgumentError(description:
                "playlist \"\(id)\" has no Apple Music API id (playlists listed by this Mac are not supported yet)")
        }
        return id
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
    /// (-1), or anything unreadable, is not a favorite.
    public static func isFavorite(_ data: Data) -> Bool {
        let attributes = firstItem(data)?["attributes"] as? JSONObject
        return (attributes?["value"] as? NSNumber)?.intValue == 1
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
