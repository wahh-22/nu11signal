import XCTest
@testable import Nu11SignalProtocol

final class SongQueueTests: XCTestCase {
    private struct Song: Equatable {
        let id: String
    }

    private func queue(_ ids: [String], found: [String], start: Int) throws -> SongQueue<Song> {
        try SongQueue(ids: ids, found: found.map(Song.init), id: \.id, startIndex: start)
    }

    func testKeepsTheRequestedOrderWhateverOrderTheCatalogReturns() throws {
        let q = try queue(["a", "b", "c", "d"], found: ["d", "b", "a", "c"], start: 3)
        XCTAssertEqual(q.items.map(\.id), ["a", "b", "c", "d"])
        XCTAssertEqual(q.start, 3)
        XCTAssertEqual(q.missing, [])
    }

    func testStartIsThePositionNotTheFirstSongWithTheSameID() throws {
        let q = try queue(["a", "b", "a"], found: ["a", "b"], start: 2)
        XCTAssertEqual(q.items.map(\.id), ["a", "b", "a"])
        XCTAssertEqual(q.start, 2)
    }

    func testDuplicateCatalogResultsKeepTheFirst() throws {
        let q = try queue(["a", "b"], found: ["a", "b", "a"], start: 1)
        XCTAssertEqual(q.items.map(\.id), ["a", "b"])
        XCTAssertEqual(q.start, 1)
    }

    func testMissingSongsAreLeftOutAndTheStartMovesToTheNextFound() throws {
        let q = try queue(["a", "b", "c", "d"], found: ["a", "d"], start: 1)
        XCTAssertEqual(q.items.map(\.id), ["a", "d"])
        XCTAssertEqual(q.start, 1)
        XCTAssertEqual(q.missing, ["b", "c"])
    }

    func testAMissingStartWithNothingAfterItStartsAtTheFirstFound() throws {
        let q = try queue(["a", "b", "c"], found: ["a", "b"], start: 2)
        XCTAssertEqual(q.items.map(\.id), ["a", "b"])
        XCTAssertEqual(q.start, 0)
        XCTAssertEqual(q.missing, ["c"])
    }

    func testStartIndexOutOfRangeIsAnError() {
        for start in [-1, 2] {
            XCTAssertThrowsError(try queue(["a", "b"], found: ["a", "b"], start: start)) { error in
                XCTAssertEqual(String(describing: error), "startIndex \(start) is out of range", "\(start)")
            }
        }
        XCTAssertThrowsError(try queue([], found: [], start: 0))
    }

    func testNothingFoundIsAnError() {
        XCTAssertThrowsError(try queue(["a", "b"], found: [], start: 0)) { error in
            XCTAssertEqual(
                String(describing: error),
                "none of the requested songs were found in the catalog: a, b")
        }
    }
}

final class QueueStartTests: XCTestCase {
    func testOnlyTheUnexpectedStartItemErrorIsRetried() {
        let domain = QueueStart.playerErrorDomain
        let code = QueueStart.unexpectedStartItemCode
        XCTAssertEqual(code, 6)
        XCTAssertTrue(QueueStart.isUnexpectedStartItem(NSError(domain: domain, code: code)))
        XCTAssertFalse(QueueStart.isUnexpectedStartItem(NSError(domain: domain, code: code - 1)))
        XCTAssertFalse(QueueStart.isUnexpectedStartItem(NSError(domain: "OtherDomain", code: code)))
        XCTAssertFalse(QueueStart.isUnexpectedStartItem(ArgumentError(description: "x")))
    }

    private let unexpectedStart = NSError(domain: QueueStart.playerErrorDomain, code: QueueStart.unexpectedStartItemCode)

    /// Runs startRetryingOnce with attempts that fail with the given
    /// errors in turn (nil succeeds); returns the attempts made, the
    /// errors announced as retried, and the error thrown, if any.
    private func run(_ outcomes: [Error?]) async -> (attempts: Int, retried: [Error], thrown: Error?) {
        var attempts = 0
        var retried: [Error] = []
        do {
            try await QueueStart.startRetryingOnce({
                defer { attempts += 1 }
                if let error = outcomes[attempts] { throw error }
            }, beforeRetry: { retried.append($0) })
            return (attempts, retried, nil)
        } catch {
            return (attempts, retried, error)
        }
    }

    func testAFirstSuccessIsNotRetried() async {
        let r = await run([nil])
        XCTAssertEqual(r.attempts, 1)
        XCTAssertTrue(r.retried.isEmpty)
        XCTAssertNil(r.thrown)
    }

    func testAnUnexpectedStartItemIsRetriedOnceAndAnnounced() async {
        let r = await run([unexpectedStart, nil])
        XCTAssertEqual(r.attempts, 2)
        XCTAssertEqual(r.retried.map { ($0 as NSError).code }, [QueueStart.unexpectedStartItemCode])
        XCTAssertNil(r.thrown)
    }

    func testASecondFailureIsReportedWithoutAThirdAttempt() async {
        let second = NSError(domain: QueueStart.playerErrorDomain, code: QueueStart.unexpectedStartItemCode, userInfo: ["n": 2])
        let r = await run([unexpectedStart, second, nil])
        XCTAssertEqual(r.attempts, 2)
        XCTAssertEqual(r.retried.count, 1)
        XCTAssertEqual((r.thrown as NSError?)?.userInfo["n"] as? Int, 2)
    }

    func testOtherErrorsAreNotRetried() async {
        let other = NSError(domain: QueueStart.playerErrorDomain, code: 5)
        let r = await run([other, nil])
        XCTAssertEqual(r.attempts, 1)
        XCTAssertTrue(r.retried.isEmpty)
        XCTAssertEqual((r.thrown as NSError?)?.code, 5)
    }
}
