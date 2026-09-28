import Foundation
import SoulKingProtocol

/// Serialized writer for stdout: every message becomes one complete line,
/// written straight to the file descriptor (no stdio buffering to flush).
final class Output: @unchecked Sendable {
    static let shared = Output()

    private let lock = NSLock()
    private let handle = FileHandle.standardOutput

    func send(_ message: Message) {
        let line = Codec.encode(message)
        lock.lock()
        defer { lock.unlock() }
        do {
            try handle.write(contentsOf: line)
        } catch {
            log("stdout write failed: \(error)")
        }
    }

    func event(_ name: String, _ fields: JSONObject = [:]) {
        send(.event(name: name, fields: fields))
    }
}

/// Diagnostics go to stderr so stdout stays pure protocol.
func log(_ text: String) {
    FileHandle.standardError.write(Data("soulking-helper: \(text)\n".utf8))
}
