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

final class QueueSegmentsTests: XCTestCase {
    /// "L" marks a song of the local library, "C" a catalog-only one.
    private func segments(_ kinds: String, start: Int) -> QueueSegments {
        QueueSegments(local: kinds.map { $0 == "L" }, start: start)
    }

    func testCatalogOnlySongsAfterLocalOnesArePlayedAsTheNextSegment() throws {
        // A local start makes a library queue, which drops catalog songs:
        // "Tocayo" (three local songs, then a catalog-only one) queues the
        // local run and keeps the last song for the next segment.
        let s = segments("LLLC", start: 0)
        XCTAssertEqual(s.current, 0..<3)
        XCTAssertEqual(s.startInCurrent, 0)
        XCTAssertNil(s.previous)
        let next = try XCTUnwrap(s.next)
        XCTAssertEqual(next.current, 3..<4)
        XCTAssertEqual(next.start, 3)
        XCTAssertNil(next.next)
    }

    func testACatalogStartHoldsEverySongAfterIt() {
        // A catalog start takes local followers appended at its tail, so it
        // runs to the end of the list (see QueuePlan.startThenAppend).
        let s = segments("CLLLLL", start: 0)
        XCTAssertEqual(s.current, 0..<6)
        XCTAssertNil(s.next)
        XCTAssertNil(s.previous)
        XCTAssertTrue(s.isWholeList)
    }

    func testALocalRunEndsAtTheNextCatalogOnlySong() throws {
        let s = segments("LCL", start: 0)
        XCTAssertEqual(s.current, 0..<1)
        XCTAssertEqual(try XCTUnwrap(s.next).current, 1..<3)
    }

    func testALocalStartKeepsTheLocalSongsBeforeItInItsSegment() throws {
        // A library queue can start at any of its entries, so the local run
        // holding the start is queued whole and PREV stays in the player.
        let s = segments("CLLLC", start: 2)
        XCTAssertEqual(s.current, 1..<4)
        XCTAssertEqual(s.startInCurrent, 1)
        XCTAssertEqual(try XCTUnwrap(s.next).current, 4..<5)
        XCTAssertEqual(try XCTUnwrap(s.previous).current, 0..<5)
    }

    func testACatalogStartInTheMiddleLeavesTheSongsBeforeItToPreviousSegments() throws {
        // Songs can only be appended at the tail, so a catalog segment
        // starts at its start; the songs before it are played by going
        // back, each previous segment from its last song.
        let s = segments("LLCLL", start: 2)
        XCTAssertEqual(s.current, 2..<5)
        XCTAssertEqual(s.startInCurrent, 0)
        XCTAssertNil(s.next)
        let previous = try XCTUnwrap(s.previous)
        XCTAssertEqual(previous.current, 0..<2)
        XCTAssertEqual(previous.start, 1)
        XCTAssertEqual(previous.startInCurrent, 1)
        XCTAssertNil(previous.previous)
        XCTAssertEqual(try XCTUnwrap(previous.next).current, 2..<5)
    }

    func testGoingBackFromALocalRunAfterACatalogSongStartsAtThatSong() throws {
        let s = segments("LCL", start: 2)
        XCTAssertEqual(s.current, 2..<3)
        let previous = try XCTUnwrap(s.previous)
        XCTAssertEqual(previous.current, 1..<3)
        XCTAssertEqual(previous.start, 1)
        XCTAssertEqual(try XCTUnwrap(previous.previous).current, 0..<1)
    }

    func testAllLocalOrAllCatalogIsOneSegment() {
        for kinds in ["LLLL", "CCCC", "C", "L"] {
            for start in 0..<kinds.count {
                let s = segments(kinds, start: start)
                XCTAssertTrue(s.isWholeList, "\(kinds) \(start)")
                XCTAssertEqual(s.startInCurrent, start, "\(kinds) \(start)")
                XCTAssertNil(s.next, "\(kinds) \(start)")
                XCTAssertNil(s.previous, "\(kinds) \(start)")
            }
        }
    }

    func testNextOnTheLastEntryMovesToTheNextSegment() throws {
        let s = segments("LLLC", start: 0)
        XCTAssertEqual(try XCTUnwrap(s.advance(entry: 2, entryCount: 3, repeatAll: false, pressed: false)).current, 3..<4)
        // Before the last entry, or with no current entry, the player skips.
        XCTAssertNil(s.advance(entry: 1, entryCount: 3, repeatAll: false, pressed: false))
        XCTAssertNil(s.advance(entry: nil, entryCount: 3, repeatAll: false, pressed: false))
    }

    func testTheLastSegmentWrapsToTheFirstOnlyWithRepeatAll() throws {
        let last = try XCTUnwrap(segments("LLLC", start: 0).next)
        XCTAssertNil(last.advance(entry: 0, entryCount: 1, repeatAll: false, pressed: false))
        let first = try XCTUnwrap(last.advance(entry: 0, entryCount: 1, repeatAll: true, pressed: false))
        XCTAssertEqual(first.current, 0..<3)
        XCTAssertEqual(first.start, 0)
        // A list played as one queue is wrapped by the player itself.
        XCTAssertNil(segments("CLLL", start: 0).advance(entry: 3, entryCount: 4, repeatAll: true, pressed: false))
    }

    func testNextPressedOnTheLastSongOfTheListGoesBackToTheFirst() throws {
        // Pressed on the last song, NEXT always wraps, with or without
        // repeat all: from the last segment...
        let last = try XCTUnwrap(segments("LLLC", start: 0).next)
        let first = try XCTUnwrap(last.advance(entry: 0, entryCount: 1, repeatAll: false, pressed: true))
        XCTAssertEqual(first.current, 0..<3)
        XCTAssertEqual(first.start, 0)
        // ...and from a list played as one queue, whose player stops there.
        let whole = try XCTUnwrap(segments("CLLL", start: 2).advance(entry: 1, entryCount: 2, repeatAll: false, pressed: true))
        XCTAssertEqual(whole.start, 0)
        // Before the last entry it still leaves the skip to the player.
        XCTAssertNil(segments("LLLL", start: 0).advance(entry: 2, entryCount: 4, repeatAll: false, pressed: true))
    }

    func testPreviousOnTheFirstEntryNearItsStartGoesBackASegment() throws {
        let s = segments("LLCLL", start: 2)
        let back = try XCTUnwrap(s.back(entry: 0, playbackTime: 1))
        XCTAssertEqual(back.current, 0..<2)
        XCTAssertEqual(back.start, 1)
        // Past the restart threshold the player restarts the song; on a
        // later entry it goes back within the queue.
        XCTAssertNil(s.back(entry: 0, playbackTime: QueueSegments.restartThreshold))
        XCTAssertNil(s.back(entry: 1, playbackTime: 1))
        XCTAssertNil(s.back(entry: nil, playbackTime: 1))
        // The first segment has nothing before it.
        XCTAssertNil(back.back(entry: 0, playbackTime: 1))
    }
}

final class SegmentEndTests: XCTestCase {
    private func at(
        _ entry: Int?, of count: Int = 3, playing: Bool = true, position: Double, duration: Double = 200
    ) -> SegmentEnd.Observation {
        SegmentEnd.Observation(entry: entry, entryCount: count, playing: playing, position: position, duration: duration)
    }

    private let finishing = SegmentEnd.Observation(entry: 2, entryCount: 3, playing: true, position: 199.2, duration: 200)

    func testTheLastSongPlayingIntoItsLastSecondsIsFinishing() {
        XCTAssertTrue(SegmentEnd.isFinishing(finishing))
        XCTAssertFalse(SegmentEnd.isFinishing(at(1, position: 199.5)))
        XCTAssertFalse(SegmentEnd.isFinishing(at(2, position: 150)))
        XCTAssertFalse(SegmentEnd.isFinishing(at(2, playing: false, position: 199.5)))
        // Without a known duration the end cannot be told.
        XCTAssertFalse(SegmentEnd.isFinishing(at(2, position: 1, duration: 0)))
    }

    func testTheQueueStoppingAfterItsLastSongEndsTheSegment() {
        for now in [
            at(2, playing: false, position: 0),
            at(2, playing: false, position: 200),
            at(2, playing: false, position: 199.4),
            at(nil, playing: false, position: 0),
            at(0, playing: false, position: 0),
        ] {
            XCTAssertTrue(SegmentEnd.ended(before: finishing, now: now, repeatMode: "off"), "\(now)")
        }
    }

    func testRepeatAllWrappingToTheFirstEntryEndsTheSegment() {
        XCTAssertTrue(SegmentEnd.ended(before: finishing, now: at(0, position: 0.3), repeatMode: "all"))
        // A one-song queue wraps onto the same entry: its position restarts.
        let alone = at(0, of: 1, position: 199.5)
        XCTAssertTrue(SegmentEnd.ended(before: alone, now: at(0, of: 1, position: 0.2), repeatMode: "all"))
        XCTAssertFalse(SegmentEnd.ended(before: alone, now: at(0, of: 1, position: 0.2), repeatMode: "off"))
    }

    func testRepeatOneNeverEndsTheSegment() {
        XCTAssertFalse(SegmentEnd.ended(before: finishing, now: at(2, position: 0.2), repeatMode: "one"))
        XCTAssertFalse(SegmentEnd.ended(before: finishing, now: at(2, playing: false, position: 0), repeatMode: "one"))
    }

    func testPlayingOnOrPausingMidSongIsNoEnd() {
        XCTAssertFalse(SegmentEnd.ended(before: finishing, now: at(2, position: 199.7), repeatMode: "off"))
        XCTAssertFalse(SegmentEnd.ended(before: finishing, now: at(2, playing: false, position: 120), repeatMode: "off"))
        // Only a finishing last song can end the segment.
        XCTAssertFalse(SegmentEnd.ended(before: at(1, position: 199.5), now: at(2, position: 0), repeatMode: "off"))
        XCTAssertFalse(SegmentEnd.ended(before: at(2, position: 100), now: at(nil, playing: false, position: 0), repeatMode: "off"))
    }
}

final class QueueCheckPollTests: XCTestCase {
    func testOnlyAConclusiveCheckShowingDropsIsReadAgain() {
        // An inconclusive check stays inconclusive: waiting cannot help.
        XCTAssertFalse(QueueCheck.readAgain(nil))
        XCTAssertFalse(QueueCheck.readAgain([]))
        XCTAssertTrue(QueueCheck.readAgain(["i.b"]))
    }
}

extension QueueSegmentsTests {
    func testACatalogStartWithNothingLocalAfterItKeepsTheCatalogSongsBeforeIt() throws {
        // Queued up front, the catalog songs before the start stay in the
        // player's queue; a local song before them is a previous segment.
        let s = QueueSegments(local: [true, false, false, false], start: 2)
        XCTAssertEqual(s.current, 1..<4)
        XCTAssertEqual(s.startInCurrent, 1)
        XCTAssertEqual(try XCTUnwrap(s.previous).current, 0..<1)
        XCTAssertEqual(try XCTUnwrap(s.previous?.next).current, 1..<4)
    }
}
