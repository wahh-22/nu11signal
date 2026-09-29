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
            let timer = Task {
                try await Task.sleep(nanoseconds: nanoseconds(for: seconds))
                if once.resume(with: .failure(TimedOut(seconds: seconds))) {
                    work.cancel()
                }
            }
            // Stop the timer as soon as the operation settles, so a finished
            // command does not leave a sleeping task behind.
            once.onResume = { timer.cancel() }
        }
    }

    /// Converts a timeout to nanoseconds without trapping: negative or NaN
    /// values fire immediately, and huge or infinite ones saturate.
    static func nanoseconds(for seconds: TimeInterval) -> UInt64 {
        guard seconds > 0 else { return 0 }
        let nanos = seconds * 1_000_000_000
        return nanos < Double(UInt64.max) ? UInt64(nanos) : UInt64.max
    }
}

/// Resumes a continuation at most once; later results are dropped.
private final class ResumeOnce<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var continuation: CheckedContinuation<T, Error>?
    private var resumed = false
    private var hook: (() -> Void)?

    init(_ continuation: CheckedContinuation<T, Error>) {
        self.continuation = continuation
    }

    /// Runs once after the first resume; runs immediately if that already
    /// happened before the hook was installed.
    var onResume: (() -> Void)? {
        get { lock.lock(); defer { lock.unlock() }; return hook }
        set {
            lock.lock()
            let runNow = resumed
            if !runNow { hook = newValue }
            lock.unlock()
            if runNow { newValue?() }
        }
    }

    @discardableResult
    func resume(with result: Result<T, Error>) -> Bool {
        lock.lock()
        let pending = continuation
        continuation = nil
        resumed = true
        let callback = hook
        hook = nil
        lock.unlock()
        guard let pending else { return false }
        pending.resume(with: result)
        callback?()
        return true
    }
}
