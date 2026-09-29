// Pure pieces of the `libraryPlaylist` and `playPlaylist` commands, free of
// MusicKit so they can be unit tested.
import Foundation

/// Where `playPlaylist` starts: an optional `startIndex` into the songs
/// `libraryPlaylist` lists for the same playlist.
public enum PlaylistStart {
    /// The requested index, or nil to start at the first entry.
    public static func index(_ request: Request) throws -> Int? {
        guard request.args["startIndex"] != nil else { return nil }
        guard let index = request.int("startIndex"), index >= 0 else {
            throw ArgumentError(description: "\"startIndex\" must be a non-negative integer")
        }
        return index
    }

    /// The song at index, or an error naming how many songs there are.
    public static func item<Item>(in songs: [Item], at index: Int) throws -> Item {
        guard songs.indices.contains(index) else {
            throw ArgumentError(description: "startIndex \(index) is out of range: the playlist has \(songs.count) songs")
        }
        return songs[index]
    }
}

/// Durations of library items. On macOS, MusicKit reports a library song's
/// (and playlist entry's, and so the player's current entry's) `duration`
/// in milliseconds, although catalog
/// items and the documentation use seconds. Rather than trust either unit
/// blindly (a later macOS may fix it), a value above `millisecondsAbove`
/// is read as milliseconds: that misreads only songs shorter than
/// 7.2 seconds reported in milliseconds, or longer than two hours reported
/// in seconds.
public enum LibraryDuration {
    public static let millisecondsAbove: TimeInterval = 7200

    /// The duration in seconds; nil and negative values become 0.
    public static func seconds(_ raw: TimeInterval?) -> TimeInterval {
        guard let raw, raw.isFinite, raw > 0 else { return 0 }
        return raw > millisecondsAbove ? raw / 1000 : raw
    }
}
