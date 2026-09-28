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

    public func int(_ key: String) -> Int? {
        guard let number = args[key] as? NSNumber, !isBool(number) else { return nil }
        return number.intValue
    }

    public func double(_ key: String) -> Double? {
        guard let number = args[key] as? NSNumber, !isBool(number) else { return nil }
        return number.doubleValue
    }

    public func strings(_ key: String) -> [String]? {
        args[key] as? [String]
    }

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
    public static func encode(_ message: Message) -> Data {
        let object: JSONObject
        switch message {
        case let .success(id, result):
            object = ["id": id, "ok": true, "result": result]
        case let .failure(id, error):
            object = ["id": id, "ok": false, "error": error]
        case let .event(name, fields):
            object = fields.merging(["event": name]) { _, new in new }
        }
        var data = (try? JSONSerialization.data(
            withJSONObject: object,
            options: [.sortedKeys, .withoutEscapingSlashes]
        )) ?? Data(#"{"event":"error","message":"failed to encode message"}"#.utf8)
        data.append(UInt8(ascii: "\n"))
        return data
    }
}
