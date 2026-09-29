// JSON Lines protocol shared with the Go adapter.
//
// Requests:  {"id":"<string>","cmd":"<name>", ...args}
// Responses: {"id":"<id>","ok":true,"result":{...}} | {"id":"<id>","ok":false,"error":"<msg>"}
// Events:    {"event":"<name>", ...fields}
import Foundation

public typealias JSONObject = [String: Any]

/// A decoded request line. Arguments are kept loosely typed and read through
/// the typed accessors below, so each command validates only what it needs.
public struct Request {
    public let id: String
    public let cmd: String
    public let args: JSONObject

    public init(id: String, cmd: String, args: JSONObject = [:]) {
        self.id = id
        self.cmd = cmd
        self.args = args
    }

    public func string(_ key: String) -> String? {
        args[key] as? String
    }

    /// An integral number within ±(2^53 - 1), the range a JSON number keeps
    /// exact in every decoder (2^53 itself is ambiguous: 2^53 + 1 rounds to
    /// it). The same bound applies whether the number was decoded as an
    /// integer or as a double. Fractional or larger values are rejected
    /// rather than truncated, so a bad argument is reported instead of
    /// silently changed.
    public func int(_ key: String) -> Int? {
        guard let number = args[key] as? NSNumber, !isBool(number) else { return nil }
        if !(number is NSDecimalNumber), !CFNumberIsFloatType(number) {
            let value = number.int64Value
            return value.magnitude < Request.exactIntegerLimit ? Int(value) : nil
        }
        let value = number.doubleValue
        guard value.isFinite, value.rounded(.towardZero) == value,
              value.magnitude < Double(Request.exactIntegerLimit)
        else { return nil }
        return Int(value)
    }

    /// Exclusive bound on the magnitude of an accepted integer.
    private static let exactIntegerLimit: UInt64 = 1 << 53

    public func double(_ key: String) -> Double? {
        guard let number = args[key] as? NSNumber, !isBool(number) else { return nil }
        return number.doubleValue
    }

    public func strings(_ key: String) -> [String]? {
        args[key] as? [String]
    }

    /// Commands that change playback run one at a time in arrival order;
    /// read-only commands (authorize, searchCatalog, artist, album,
    /// songAlbum, catalogPlaylist, playlists) run concurrently.
    public static let playbackCommands: Set<String> = [
        "playSongs", "playPlaylist", "pause", "resume", "next", "previous", "stop", "seek",
    ]

    public var mutatesPlayback: Bool { Request.playbackCommands.contains(cmd) }

    private func isBool(_ number: NSNumber) -> Bool {
        CFGetTypeID(number) == CFBooleanGetTypeID()
    }
}

/// A line that could not become a Request. `id` is the request id when it
/// could be recovered, or "" when the line was not parseable at all.
public struct DecodeFailure: Error, Equatable {
    public let id: String
    public let message: String
}

/// Everything the helper writes to stdout.
public enum Message {
    case success(id: String, result: JSONObject)
    case failure(id: String, error: String)
    case event(name: String, fields: JSONObject)

    /// Periodic state events may be dropped under backpressure (a newer one
    /// always follows); responses and other events never are.
    public var isDroppable: Bool {
        if case .event(name: "state", fields: _) = self { return true }
        return false
    }
}

public enum Codec {
    public static func decode(_ line: String) -> Result<Request, DecodeFailure> {
        guard let data = line.data(using: .utf8),
              let object = try? JSONSerialization.jsonObject(with: data),
              var fields = object as? JSONObject
        else {
            return .failure(DecodeFailure(id: "", message: "malformed JSON request"))
        }
        guard let id = fields.removeValue(forKey: "id") as? String else {
            return .failure(DecodeFailure(id: "", message: "request is missing a string \"id\""))
        }
        guard let cmd = fields.removeValue(forKey: "cmd") as? String, !cmd.isEmpty else {
            return .failure(DecodeFailure(id: id, message: "request is missing a string \"cmd\""))
        }
        return .success(Request(id: id, cmd: cmd, args: fields))
    }

    /// Encodes one message as a single JSON line terminated by "\n".
    ///
    /// A message that is not valid JSON (NaN, infinity, a non-JSON type) never
    /// crashes the helper: a response falls back to a failure that keeps its
    /// `id`, so the caller waiting on that id still gets an answer, and an
    /// event falls back to an `error` event naming the event that failed.
    public static func encode(_ message: Message) -> Data {
        let object: JSONObject
        let fallback: JSONObject
        switch message {
        case let .success(id, result):
            object = ["id": id, "ok": true, "result": result]
            fallback = ["id": id, "ok": false, "error": "failed to encode response"]
        case let .failure(id, error):
            object = ["id": id, "ok": false, "error": error]
            fallback = ["id": id, "ok": false, "error": "failed to encode response"]
        case let .event(name, fields):
            object = fields.merging(["event": name]) { _, new in new }
            fallback = ["event": "error", "message": "failed to encode \(name) event"]
        }
        var data = serialize(object) ?? serialize(fallback)
            ?? Data(#"{"event":"error","message":"failed to encode message"}"#.utf8)
        data.append(UInt8(ascii: "\n"))
        return data
    }

    /// JSONSerialization raises an Objective-C exception (which Swift cannot
    /// catch) for invalid objects, so validate before serializing.
    private static func serialize(_ object: JSONObject) -> Data? {
        guard JSONSerialization.isValidJSONObject(object) else { return nil }
        return try? JSONSerialization.data(
            withJSONObject: object,
            options: [.sortedKeys, .withoutEscapingSlashes]
        )
    }
}
