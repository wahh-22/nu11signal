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

/// How the queue is handed to the player, given the queued songs the Mac's
/// local library also holds (`libraryCopies`, keyed by catalog id).
///
/// Seen live on macOS, the start song decides the kind of queue:
/// - A catalog start makes a catalog queue. Handed any song of the local
///   library up front, as its catalog song or its library copy, it fails
///   to prepare (Code=6, see QueueStart). Queued alone and playing, it
///   takes local songs inserted at its tail as their library copies,
///   along with catalog songs. So a catalog start with local songs after
///   it is queued alone, then its followers are appended
///   (`startThenAppend`); without local followers the queue goes up front.
/// - A local start makes a library queue, which prepares with library
///   copies but silently drops every catalog song, up front or inserted.
///   So it queues the local songs as their library copies and leaves the
///   catalog-only songs out (`skipped`).
public struct QueuePlan<Item> {
    public enum Shape {
        /// Hand the player every item at once, starting at `start`.
        case upFront(items: [Item], start: Int)
        /// Queue `start` alone and, once it plays, insert `followers` at
        /// the tail. Songs before the start are not queued: the tail is
        /// the only place songs can be added, and they would play last.
        case startThenAppend(start: Item, followers: [Item])
    }

    public let shape: Shape
    /// Ids left out of the queue because the player cannot hold them, in
    /// queue order (repeats included).
    public let skipped: [String]
    /// The catalog id of each library copy queued, keyed by the copy's own
    /// id: the player reports a copy by that id, and the state events name
    /// the catalog song asked for instead. Empty when no copy is queued.
    public let catalogIDs: [String: String]

    /// The caller guarantees `items.indices.contains(start)`.
    public init(items: [Item], start: Int, id: (Item) -> String, libraryCopies: [String: Item]) {
        func isLocal(_ item: Item) -> Bool { libraryCopies[id(item)] != nil }
        var catalogIDs: [String: String] = [:]
        func copyOrSelf(_ item: Item) -> Item {
            guard let copy = libraryCopies[id(item)] else { return item }
            catalogIDs[id(copy)] = id(item)
            return copy
        }
        let startIsLocal = isLocal(items[start])
        let followers = QueueStart.followers(of: items, after: start)
        if !startIsLocal, followers.contains(where: isLocal) {
            self.shape = .startThenAppend(start: items[start], followers: followers.map(copyOrSelf))
            self.skipped = []
            self.catalogIDs = catalogIDs
            return
        }
        // Up front: a local start keeps only local songs (as their copies),
        // a catalog start only catalog songs. The start itself always stays.
        var kept: [Item] = []
        var skipped: [String] = []
        var newStart = 0
        for (position, item) in items.enumerated() {
            if position == start {
                newStart = kept.count
            } else if isLocal(item) != startIsLocal {
                skipped.append(id(item))
                continue
            }
            kept.append(startIsLocal ? copyOrSelf(item) : item)
        }
        self.shape = .upFront(items: kept, start: newStart)
        self.skipped = skipped
        self.catalogIDs = catalogIDs
    }

    /// The catalog id a library song's play parameters name, from their
    /// Codable form (`"catalogId"`; PlayParameters has no public fields);
    /// nil when there is none.
    public static func catalogID(playParameters json: Data) -> String? {
        let object = try? JSONSerialization.jsonObject(with: json) as? JSONObject
        guard let id = object?["catalogId"] as? String, !id.isEmpty else { return nil }
        return id
    }
}

/// Finds the songs the player silently left out of a queue it accepted.
public enum QueueCheck {
    /// The `submitted` item ids (the ids of the items handed to the player,
    /// library copies by their own id) that `queued`, the ids of the
    /// player's queue entries in any order, does not hold: in submitted
    /// order, repeats counted. Nil when the check cannot tell: an entry
    /// without an id, or an id that was never submitted (the player naming
    /// an item by another form of its id), so a drop is never reported
    /// falsely.
    public static func dropped(submitted: [String], queued: [String?]) -> [String]? {
        var counts: [String: Int] = [:]
        for id in queued {
            guard let id else { return nil }
            counts[id, default: 0] += 1
        }
        var dropped: [String] = []
        for id in submitted {
            if let n = counts[id], n > 0 {
                counts[id] = n - 1
            } else {
                dropped.append(id)
            }
        }
        guard counts.values.allSatisfy({ $0 == 0 }) else { return nil }
        return dropped
    }
}

/// Recognizes the player's refusal to prepare a queue, and falls back.
public enum QueueStart {
    /// The MediaPlayer error domain behind `ApplicationMusicPlayer` on macOS.
    public static let playerErrorDomain = "MPMusicPlayerControllerErrorDomain"

    /// `MPMusicPlayerControllerErrorDomain` code of a queue the player could
    /// not prepare; its debug description says "Failed to prepare to play"
    /// or "Prepare queue failed with unexpected start item". The error
    /// has no public Swift name on macOS.
    public static let prepareFailureCode = 6

    /// Whether error is the player's prepare failure (code 6).
    public static func isPrepareFailure(_ error: Error) -> Bool {
        let error = error as NSError
        return error.domain == playerErrorDomain && error.code == prepareFailureCode
    }

    /// Runs attempt; if the player could not prepare it, calls
    /// beforeFallback with that error and runs fallback exactly once.
    /// Returns whether the fallback ran. Any other error of attempt, or
    /// any error of fallback, is thrown: never more than two attempts.
    public static func startWithFallback(
        _ attempt: () async throws -> Void,
        fallback: () async throws -> Void,
        beforeFallback: (Error) -> Void
    ) async throws -> Bool {
        do {
            try await attempt()
            return false
        } catch where isPrepareFailure(error) {
            beforeFallback(error)
            try await fallback()
            return true
        }
    }

    /// The songs that follow `start` in `items`, in order: what is appended
    /// after the start song when it is queued on its own. The songs before
    /// it are left out, as the player would play them next.
    public static func followers<Item>(of items: [Item], after start: Int) -> [Item] {
        guard items.indices.contains(start) else { return [] }
        return Array(items[(start + 1)...])
    }

    /// The error reported when song could not be started: a player error
    /// names the song, the error's domain and code, and the player's own
    /// debug description when it has one. Any other error passes through.
    public static func failure(_ error: Error, song: String) -> Error {
        let nsError = error as NSError
        guard nsError.domain == playerErrorDomain else { return error }
        var reason = "\(nsError.domain) \(nsError.code)"
        if let debug = nsError.userInfo[NSDebugDescriptionErrorKey] as? String, !debug.isEmpty {
            reason += ": \(debug)"
        }
        return ArgumentError(description: "Apple Music could not prepare \"\(song)\" to play (\(reason))")
    }
}
