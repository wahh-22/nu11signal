// Pure pieces of the read-only catalog commands (artist, album, songAlbum,
// catalogPlaylist), free of MusicKit so they can be unit tested.
import Foundation

/// Time budgets of the catalog commands, in seconds. MusicKit requests do
/// not honour cancellation, so each step runs under `Deadline.run`; a
/// command's budget is the sum of its sequential steps and must stay below
/// the Go client's deadline for it, so the helper's own error (not a Go
/// timeout) reaches the UI. The Go deadlines are mirrored here and pinned
/// on the Go side by `TestCatalogBudgetMatchesTheHelper` in
/// internal/radio, which reads this file: keep each derived budget a sum
/// of products of integers and the constants declared above it.
public enum CatalogBudget {
    /// One catalog lookup by id, relationships included.
    public static let lookup: TimeInterval = 5
    /// One optional artist page part (a relationship, or origin/formed);
    /// the parts load concurrently after the lookup.
    public static let section: TimeInterval = 8

    /// `artist`: the lookup, then its parts concurrently.
    public static let artist = lookup + section
    /// `album` and `catalogPlaylist`: one lookup with tracks.
    public static let album = lookup
    public static let catalogPlaylist = lookup
    /// `songAlbum`: the song lookup, then the album lookup.
    public static let songAlbum = 2 * lookup
    /// A library edit (createPlaylist, addToPlaylist, favorite,
    /// setFavorite): one Apple Music API request. A write that times out
    /// may still be applied later; the command reports the timeout.
    public static let libraryEdit = lookup
    /// A library read (`playlists`, `libraryPlaylist`): the Apple Music
    /// API pages of one collection, fetched one after another (up to 5
    /// pages of playlists or 10 of songs).
    public static let libraryRead = 2 * lookup

    /// `artistCallTimeout` in internal/radio.
    public static let goArtistCallTimeout: TimeInterval = 15
    /// `detailCallTimeout` in internal/radio (album, song album, playlist,
    /// the library playlists, and the library edits).
    public static let goDetailCallTimeout: TimeInterval = 12
}

/// A request argument that is missing or unusable.
public struct ArgumentError: Error, CustomStringConvertible, Equatable {
    public let description: String
}

extension Request {
    /// The catalog id in argument `key`: a string that is not blank. Ids are
    /// never named "id", which is the request's own correlation id.
    public func requiredID(_ key: String) throws -> String {
        guard let id = string(key), !id.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw ArgumentError(description: "\(cmd) requires a non-empty \"\(key)\"")
        }
        return id
    }
}

/// Catalog release dates on the wire.
public enum CatalogDate {
    /// The date as "2006-01-02", read in UTC: catalog release dates are
    /// calendar dates, and a local zone west of UTC would move them back a
    /// day.
    public static func iso(_ date: Date) -> String {
        let c = utc.dateComponents([.year, .month, .day], from: date)
        return String(format: "%04d-%02d-%02d", c.year ?? 0, c.month ?? 0, c.day ?? 0)
    }

    public static let utc: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        return calendar
    }()
}
