import Foundation

/// A thread-safe FIFO of encoded lines between producers (commands, the
/// state emitter) and one dedicated stdout writer thread.
///
/// Producers never block. Capacity bounds only droppable lines (state
/// events): when the queue is full, the oldest queued droppable line is
/// evicted in favour of the newer one, or the new droppable line is dropped
/// when nothing droppable is queued. Non-droppable lines (responses, ready,
/// error events) are always accepted, so a caller waiting on a response is
/// never left without one; they are bounded by the requests the parent sent.
public final class Outbox: @unchecked Sendable {
    private struct Entry {
        let data: Data
        let droppable: Bool
    }

    private let capacity: Int
    private let condition = NSCondition()
    private var entries: [Entry] = []
    private var writing = false
    private var closed = false
    private var droppedCount = 0

    public init(capacity: Int) {
        precondition(capacity > 0, "Outbox capacity must be positive")
        self.capacity = capacity
    }

    /// Queued lines, excluding one the writer is currently writing.
    public var count: Int { locked { entries.count } }

    /// Droppable lines discarded because the queue was full.
    public var dropped: Int { locked { droppedCount } }

    /// Queues a line. Returns false when it was dropped or the outbox is closed.
    @discardableResult
    public func push(_ data: Data, droppable: Bool) -> Bool {
        locked {
            guard !closed else { return false }
            if droppable, entries.count >= capacity {
                guard let oldest = entries.firstIndex(where: \.droppable) else {
                    droppedCount += 1
                    return false
                }
                entries.remove(at: oldest)
                droppedCount += 1
            }
            entries.append(Entry(data: data, droppable: droppable))
            condition.broadcast()
            return true
        }
    }

    /// Blocks until a line is available and hands it to the writer, which
    /// must call `finishWrite()` afterwards. Returns nil once closed.
    public func pop() -> Data? {
        condition.lock()
        defer { condition.unlock() }
        while entries.isEmpty, !closed { condition.wait() }
        guard !closed else { return nil }
        writing = true
        return entries.removeFirst().data
    }

    /// Marks the line returned by the last `pop()` as written.
    public func finishWrite() {
        locked {
            writing = false
            condition.broadcast()
        }
    }

    /// The writer loop: hands queued lines to `write` in order until the
    /// outbox is closed (returns nil) or a write fails. On a failure the
    /// outbox is closed, so producers stop queueing and flush waiters are
    /// released, and the failure's errno is returned for the caller to act
    /// on. `write` returns nil on success or an errno.
    public func drain(_ write: (Data) -> Int32?) -> Int32? {
        while let line = pop() {
            if let failure = write(line) {
                close()
                return failure
            }
            finishWrite()
        }
        return nil
    }

    /// Waits until every queued line has been written, or the deadline passes.
    public func waitUntilFlushed(before deadline: Date) -> Bool {
        condition.lock()
        defer { condition.unlock() }
        while !entries.isEmpty || writing {
            if closed || !condition.wait(until: deadline) { return false }
        }
        return true
    }

    /// Discards queued lines and releases a writer blocked in `pop()`.
    public func close() {
        locked {
            closed = true
            entries.removeAll()
            condition.broadcast()
        }
    }

    private func locked<T>(_ body: () -> T) -> T {
        condition.lock()
        defer { condition.unlock() }
        return body()
    }
}
