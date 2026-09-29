// Pure pieces of the queue the `playSongs` and `playPlaylist` commands
// build, free of MusicKit so they can be unit tested.
import Foundation

/// The songs `playSongs` queues: the found songs in the requested order and
/// the position playback starts at.
///
/// The start is a position in `items`, never a song: the helper hands the
/// player one queue entry per item and names the start entry itself, so a
/// song listed twice cannot make the player start at the wrong copy.
public struct SongQueue<Item> {
    public let items: [Item]
    /// Index into `items` of the first song to play.
    public let start: Int
    /// Requested ids the catalog did not return, in requested order.
    public let missing: [String]

    /// Orders `found` (in any order, possibly incomplete or repeated) as
    /// `ids` lists them. Songs not found are left out; if the song at
    /// `startIndex` is one of them, playback starts at the next found song,
    /// or at the first found when none follows.
    public init(ids: [String], found: [Item], id: (Item) -> String, startIndex: Int) throws {
        guard ids.indices.contains(startIndex) else {
            throw ArgumentError(description: "startIndex \(startIndex) is out of range")
        }
        var byID: [String: Item] = [:]
        for item in found where byID[id(item)] == nil {
            byID[id(item)] = item
        }
        var items: [Item] = []
        var start: Int?
        var missing: [String] = []
        for (position, songID) in ids.enumerated() {
            guard let item = byID[songID] else {
                missing.append(songID)
                continue
            }
            if start == nil, position >= startIndex {
                start = items.count
            }
            items.append(item)
        }
        guard !items.isEmpty else {
            throw ArgumentError(
                description: "none of the requested songs were found in the catalog: \(missing.joined(separator: ", "))")
        }
        self.items = items
        self.start = start ?? 0
        self.missing = missing
    }
}

/// Recognizes the player's transient refusal to start a queue.
public enum QueueStart {
    /// The MediaPlayer error domain behind `ApplicationMusicPlayer` on macOS.
    public static let playerErrorDomain = "MPMusicPlayerControllerErrorDomain"

    /// Whether error is `MPMusicPlayerControllerErrorDomain` code 6, "prepare
    /// queue failed with unexpected start item": the player prepared a queue
    /// whose current item was not the requested start. Seen intermittently
    /// right after the queue is replaced; the same request succeeds when it
    /// is sent again, so the helper retries it once.
    public static func isUnexpectedStartItem(_ error: Error) -> Bool {
        let error = error as NSError
        return error.domain == playerErrorDomain && error.code == 6
    }
}
