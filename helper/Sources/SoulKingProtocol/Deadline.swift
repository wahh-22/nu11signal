import Foundation

/// Bounds an async operation by a timeout that holds even when the
/// operation ignores cancellation.
///
/// Structured concurrency (a task group) would wait for a hung child before
/// returning, so the operation runs in an unstructured task and whichever
/// finishes first, the operation or the timer, resumes the caller exactly
/// once. On timeout the operation is cancelled but may keep running in the
/// background; its eventual result is discarded.
public enum Deadline {
    public struct TimedOut: Error, CustomStringConvertible, Equatable {
        public let seconds: TimeInterval
        public var description: String { "timed out after \(seconds)s" }
    }

    public static func run<T>(
        seconds: TimeInterval,
        _ operation: @escaping () async throws -> T
    ) async throws -> T {
        try await withCheckedThrowingContinuation { continuation in
            let once = ResumeOnce(continuation)
            let work = Task {
                do {
                    once.resume(with: .success(try await operation()))
                } catch {
                    once.resume(with: .failure(error))
                }
            }
            Task {
                try? await Task.sleep(nanoseconds: UInt64(max(seconds, 0) * 1_000_000_000))
                if once.resume(with: .failure(TimedOut(seconds: seconds))) {
                    work.cancel()
                }
            }
        }
    }
}

/// Resumes a continuation at most once; later results are dropped.
private final class ResumeOnce<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var continuation: CheckedContinuation<T, Error>?

    init(_ continuation: CheckedContinuation<T, Error>) {
        self.continuation = continuation
    }

    @discardableResult
    func resume(with result: Result<T, Error>) -> Bool {
        lock.lock()
        let pending = continuation
        continuation = nil
        lock.unlock()
        guard let pending else { return false }
        pending.resume(with: result)
        return true
    }
}
