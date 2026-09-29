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

/// The queue as the player is handed it, given the queued songs the Mac's
/// local library also holds (`libraryCopies`, keyed by catalog id).
///
/// Seen live on macOS: a queue whose start is a catalog song fails with
/// "failed to prepare to play" (or "unexpected start item") as soon as it
/// holds any song of the local library, whether queued as its catalog song
/// or as its library copy, although each song prepares on its own; with a
/// library copy as its start, the same mixed queue prepares. So a local
/// start song takes every local song as its library copy, and a catalog
/// start leaves the local songs out (`skipped`) rather than fail.
public struct PreparedQueue<Item> {
    public let items: [Item]
    /// Index into `items` of the first song to play.
    public let start: Int
    /// Ids left out of the queue, in queue order (repeats included).
    public let skipped: [String]

    /// The caller guarantees `items.indices.contains(start)`.
    public init(items: [Item], start: Int, id: (Item) -> String, libraryCopies: [String: Item]) {
        if libraryCopies[id(items[start])] != nil {
            self.items = items.map { libraryCopies[id($0)] ?? $0 }
            self.start = start
            self.skipped = []
            return
        }
        var kept: [Item] = []
        var skipped: [String] = []
        var newStart = 0
        for (position, item) in items.enumerated() {
            if position == start {
                newStart = kept.count
            } else if libraryCopies[id(item)] != nil {
                skipped.append(id(item))
                continue
            }
            kept.append(item)
        }
        self.items = kept
        self.start = newStart
        self.skipped = skipped
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
