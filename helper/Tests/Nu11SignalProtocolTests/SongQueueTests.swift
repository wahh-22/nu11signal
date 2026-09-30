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

final class QueuePlanTests: XCTestCase {
    private struct Song: Equatable {
        let id: String
        var library = false
    }

    private func copies(_ local: [String]) -> [String: Song] {
        Dictionary(uniqueKeysWithValues: local.map { ($0, Song(id: "i.\($0)", library: true)) })
    }

    private func plan(_ ids: [String], start: Int, local: [String]) -> QueuePlan<Song> {
        QueuePlan(items: ids.map { Song(id: $0) }, start: start, id: \.id, libraryCopies: copies(local))
    }

    private func upFront(_ plan: QueuePlan<Song>) -> (items: [Song], start: Int)? {
        guard case let .upFront(items, start) = plan.shape else { return nil }
        return (items, start)
    }

    private func startThenAppend(_ plan: QueuePlan<Song>) -> (start: Song, followers: [Song])? {
        guard case let .startThenAppend(start, followers) = plan.shape else { return nil }
        return (start, followers)
    }

    func testAQueueWithoutLocalLibrarySongsIsQueuedUpFrontUnchanged() throws {
        let q = try XCTUnwrap(upFront(plan(["a", "b", "c"], start: 1, local: [])))
        XCTAssertEqual(q.items, ["a", "b", "c"].map { Song(id: $0) })
        XCTAssertEqual(q.start, 1)
        XCTAssertEqual(plan(["a", "b", "c"], start: 1, local: []).skipped, [])
        XCTAssertEqual(plan(["a", "b", "c"], start: 1, local: []).catalogIDs, [:])
    }

    func testACatalogStartWithLocalFollowersStartsAloneThenAppendsEveryFollower() throws {
        // A catalog start with local songs in the same queue fails (Code=6);
        // the start queued alone, then the rest inserted at the tail, keeps
        // them all, local songs as their library copies.
        let p = plan(["a", "b", "c", "d", "b"], start: 0, local: ["b", "c"])
        let q = try XCTUnwrap(startThenAppend(p))
        XCTAssertEqual(q.start, Song(id: "a"))
        XCTAssertEqual(q.followers.map(\.id), ["i.b", "i.c", "d", "i.b"])
        XCTAssertEqual(q.followers.map(\.library), [true, true, false, true])
        XCTAssertEqual(p.skipped, [])
        XCTAssertEqual(p.catalogIDs, ["i.b": "b", "i.c": "c"])
    }

    func testStartingThenAppendingLeavesTheSongsBeforeTheStartOut() throws {
        // The tail is the only place songs can be added: songs before the
        // start would play after the last one, so they are not queued.
        let p = plan(["x", "b", "a", "c"], start: 2, local: ["b", "c"])
        let q = try XCTUnwrap(startThenAppend(p))
        XCTAssertEqual(q.start, Song(id: "a"))
        XCTAssertEqual(q.followers.map(\.id), ["i.c"])
        XCTAssertEqual(p.skipped, [])
    }

    func testACatalogStartWithLocalSongsOnlyBeforeItLeavesThemOut() throws {
        // Nothing local follows the start, so the queue goes up front, and
        // the local songs before it (which would fail it) are left out.
        let p = plan(["b", "a", "c", "d"], start: 2, local: ["b"])
        let q = try XCTUnwrap(upFront(p))
        XCTAssertEqual(q.items.map(\.id), ["a", "c", "d"])
        XCTAssertEqual(q.start, 1)
        XCTAssertEqual(p.skipped, ["b"])
        XCTAssertFalse(q.items.contains { $0.library })
    }

    func testALocalStartQueuesLibraryCopiesAndSkipsCatalogOnlySongs() throws {
        // A local start makes a library queue, which silently drops catalog
        // songs: they are left out and reported instead.
        let p = plan(["a", "b", "c", "d", "b"], start: 1, local: ["b", "c"])
        let q = try XCTUnwrap(upFront(p))
        XCTAssertEqual(q.items.map(\.id), ["i.b", "i.c", "i.b"])
        XCTAssertTrue(q.items.allSatisfy(\.library))
        XCTAssertEqual(q.start, 0)
        XCTAssertEqual(p.skipped, ["a", "d"])
        XCTAssertEqual(p.catalogIDs, ["i.b": "b", "i.c": "c"])
    }

    func testALocalStartKeepsItsPositionAmongTheLibraryCopies() throws {
        let p = plan(["b", "a", "c", "b"], start: 3, local: ["b", "c"])
        let q = try XCTUnwrap(upFront(p))
        XCTAssertEqual(q.items.map(\.id), ["i.b", "i.c", "i.b"])
        XCTAssertEqual(q.start, 2)
        XCTAssertEqual(p.skipped, ["a"])
    }

    func testALibraryCopyIsMatchedByTheCatalogIDInItsPlayParameters() {
        // PlayParameters is opaque; its Codable form names the catalog id.
        let json = #"{"isLibrary":true,"kind":"song","id":"i.5PkLbY7FbmX2YNp","catalogId":"1825270816","musicKit_persistentID":"-678"}"#
        XCTAssertEqual(QueuePlan<Song>.catalogID(playParameters: Data(json.utf8)), "1825270816")
        for other in [#"{"id":"i.a","kind":"song"}"#, #"{"catalogId":""}"#, #"{"catalogId":7}"#, "", "[]"] {
            XCTAssertNil(QueuePlan<Song>.catalogID(playParameters: Data(other.utf8)), other)
        }
    }
}

final class QueueCheckTests: XCTestCase {
    func testAQueueHoldingEverySubmittedItemDroppedNothing() {
        XCTAssertEqual(QueueCheck.dropped(submitted: ["a", "i.b", "a"], queued: ["a", "i.b", "a"]), [])
    }

    func testItemsMissingFromTheQueueAreDroppedInSubmittedOrder() {
        XCTAssertEqual(QueueCheck.dropped(submitted: ["a", "i.b", "c", "i.b"], queued: ["i.b", "a"]), ["c", "i.b"])
    }

    func testTheQueuedOrderDoesNotMatter() {
        XCTAssertEqual(QueueCheck.dropped(submitted: ["a", "b", "c"], queued: ["c", "a", "b"]), [])
    }

    func testAnEntryWithoutAnIDMakesTheCheckInconclusive() {
        XCTAssertNil(QueueCheck.dropped(submitted: ["a", "b"], queued: ["a", nil]))
    }

    func testAnIDThatWasNotSubmittedMakesTheCheckInconclusive() {
        // The player may name an item by another form of its id (catalog
        // for library or back): the check cannot tell a drop from that.
        XCTAssertNil(QueueCheck.dropped(submitted: ["a", "i.b"], queued: ["a", "b"]))
        XCTAssertNil(QueueCheck.dropped(submitted: ["a"], queued: ["a", "a"]))
    }
}

final class QueueStartTests: XCTestCase {
    func testTheFollowersOfAStartSongAreTheSongsAfterIt() {
        XCTAssertEqual(QueueStart.followers(of: ["a", "b", "c", "d"], after: 1), ["c", "d"])
        XCTAssertEqual(QueueStart.followers(of: ["a", "b"], after: 1), [])
        XCTAssertEqual(QueueStart.followers(of: ["a"], after: 3), [])
        XCTAssertEqual(QueueStart.followers(of: [String](), after: 0), [])
    }

    func testOnlyThePlayersPrepareFailureFallsBack() {
        let domain = QueueStart.playerErrorDomain
        let code = QueueStart.prepareFailureCode
        XCTAssertEqual(code, 6)
        XCTAssertTrue(QueueStart.isPrepareFailure(NSError(domain: domain, code: code)))
        XCTAssertFalse(QueueStart.isPrepareFailure(NSError(domain: domain, code: code - 1)))
        XCTAssertFalse(QueueStart.isPrepareFailure(NSError(domain: "OtherDomain", code: code)))
        XCTAssertFalse(QueueStart.isPrepareFailure(ArgumentError(description: "x")))
    }

    private let prepareFailure = NSError(
        domain: QueueStart.playerErrorDomain, code: QueueStart.prepareFailureCode,
        userInfo: [NSDebugDescriptionErrorKey: "Failed to prepare to play"])

    /// Runs startWithFallback with a queue attempt and a fallback that
    /// fail with the given errors (nil succeeds); returns what ran, the
    /// errors announced before the fallback, whether it was used and the
    /// error thrown, if any.
    private func run(_ queue: Error?, _ fallback: Error?) async -> (runs: [String], announced: [Error], usedFallback: Bool?, thrown: Error?) {
        var runs: [String] = []
        var announced: [Error] = []
        do {
            let used = try await QueueStart.startWithFallback({
                runs.append("queue")
                if let queue { throw queue }
            }, fallback: {
                runs.append("fallback")
                if let fallback { throw fallback }
            }, beforeFallback: { announced.append($0) })
            return (runs, announced, used, nil)
        } catch {
            return (runs, announced, nil, error)
        }
    }

    func testAQueueThatStartsNeedsNoFallback() async {
        let r = await run(nil, nil)
        XCTAssertEqual(r.runs, ["queue"])
        XCTAssertTrue(r.announced.isEmpty)
        XCTAssertEqual(r.usedFallback, false)
    }

    func testAPrepareFailureFallsBackOnceAndIsAnnounced() async {
        let r = await run(prepareFailure, nil)
        XCTAssertEqual(r.runs, ["queue", "fallback"])
        XCTAssertEqual(r.announced.map { ($0 as NSError).code }, [QueueStart.prepareFailureCode])
        XCTAssertEqual(r.usedFallback, true)
        XCTAssertNil(r.thrown)
    }

    func testAFailedFallbackIsReported() async {
        let second = NSError(domain: QueueStart.playerErrorDomain, code: QueueStart.prepareFailureCode, userInfo: ["n": 2])
        let r = await run(prepareFailure, second)
        XCTAssertEqual(r.runs, ["queue", "fallback"])
        XCTAssertEqual((r.thrown as NSError?)?.userInfo["n"] as? Int, 2)
    }

    func testOtherErrorsDoNotFallBack() async {
        let other = NSError(domain: QueueStart.playerErrorDomain, code: 5)
        let r = await run(other, nil)
        XCTAssertEqual(r.runs, ["queue"])
        XCTAssertTrue(r.announced.isEmpty)
        XCTAssertEqual((r.thrown as NSError?)?.code, 5)
    }

    func testAFailureToStartNamesTheSongAndThePlayersReason() {
        XCTAssertEqual(
            String(describing: QueueStart.failure(prepareFailure, song: "Para Qué")),
            #"Apple Music could not prepare "Para Qué" to play (MPMusicPlayerControllerErrorDomain 6: Failed to prepare to play)"#)
        XCTAssertEqual(
            String(describing: QueueStart.failure(NSError(domain: QueueStart.playerErrorDomain, code: 2), song: "X")),
            #"Apple Music could not prepare "X" to play (MPMusicPlayerControllerErrorDomain 2)"#)
        // Errors that are not the player's pass through unchanged.
        XCTAssertEqual(
            String(describing: QueueStart.failure(ArgumentError(description: "boom"), song: "X")), "boom")
        XCTAssertEqual((QueueStart.failure(NSError(domain: "D", code: 2), song: "X") as NSError).domain, "D")
    }
}
