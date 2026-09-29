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
        XCTAssertTrue(QueueStart.isUnexpectedStartItem(NSError(domain: domain, code: 6)))
        XCTAssertFalse(QueueStart.isUnexpectedStartItem(NSError(domain: domain, code: 5)))
        XCTAssertFalse(QueueStart.isUnexpectedStartItem(NSError(domain: "OtherDomain", code: 6)))
        XCTAssertFalse(QueueStart.isUnexpectedStartItem(ArgumentError(description: "x")))
    }
}
