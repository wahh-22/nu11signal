import Darwin
import Foundation
import Nu11SignalProtocol

/// stdout writer: producers enqueue complete lines and return immediately;
/// one dedicated thread writes them to the file descriptor (no stdio
/// buffering). If the parent stops reading, the writer blocks but the main
/// actor does not: state events are dropped once the bounded queue is full,
/// responses are always kept. A failed write (EPIPE: the parent is gone)
/// shuts the helper down.
final class Output: @unchecked Sendable {
    static let shared = Output()

    /// Enough state events for ~2 minutes of playback at 2 per second.
    private static let capacity = 256

    private let outbox = Outbox(capacity: Output.capacity)

    private init() {
        let thread = Thread { [outbox] in Output.writeLoop(outbox) }
        thread.name = "nu11signal-helper.stdout"
        thread.start()
    }

    func send(_ message: Message) {
        outbox.push(Codec.encode(message), droppable: message.isDroppable)
    }

    func event(_ name: String, _ fields: JSONObject = [:]) {
        send(.event(name: name, fields: fields))
    }

    /// Waits until queued lines are written, or the deadline passes.
    @discardableResult
    func flush(before deadline: Date) -> Bool {
        outbox.waitUntilFlushed(before: deadline)
    }

    private static func writeLoop(_ outbox: Outbox) {
        // drain closes the outbox on a failed write (unit tested in
        // Nu11SignalProtocol); only the process-level reaction lives here.
        guard let failure = outbox.drain({ writeAll(STDOUT_FILENO, $0) }) else { return }
        log("stdout write failed: \(String(cString: strerror(failure))); shutting down")
        Lifecycle.shutdown(code: failure == EPIPE ? 0 : 1)
    }
}

/// Writes every byte, retrying on EINTR. Returns the errno of a failure.
private func writeAll(_ fd: Int32, _ data: Data) -> Int32? {
    data.withUnsafeBytes { buffer -> Int32? in
        guard var pointer = buffer.baseAddress else { return nil }
        var remaining = buffer.count
        while remaining > 0 {
            let written = Darwin.write(fd, pointer, remaining)
            if written < 0 {
                if errno == EINTR { continue }
                return errno
            }
            pointer += written
            remaining -= written
        }
        return nil
    }
}

/// Diagnostics go to stderr so stdout stays pure protocol. Logging is best
/// effort: a failed or closed stderr is ignored, never raised.
func log(_ text: String) {
    _ = writeAll(STDERR_FILENO, Data("nu11signal-helper: \(text)\n".utf8))
}
