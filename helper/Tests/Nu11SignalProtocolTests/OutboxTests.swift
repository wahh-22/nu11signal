import Foundation
import XCTest
@testable import Nu11SignalProtocol

final class RoutingTests: XCTestCase {
    func testPlaybackCommandsAreSerialized() {
        for cmd in ["playSongs", "playPlaylist", "pause", "resume", "next", "previous", "stop", "seek"] {
            XCTAssertTrue(Request(id: "1", cmd: cmd).mutatesPlayback, cmd)
        }
    }

    func testReadOnlyAndUnknownCommandsRunConcurrently() {
        for cmd in ["authorize", "searchCatalog", "playlists", "bogus"] {
            XCTAssertFalse(Request(id: "1", cmd: cmd).mutatesPlayback, cmd)
        }
    }

    func testOnlyStateEventsAreDroppable() {
        XCTAssertTrue(Message.event(name: "state", fields: [:]).isDroppable)
        XCTAssertFalse(Message.event(name: "ready", fields: [:]).isDroppable)
        XCTAssertFalse(Message.event(name: "error", fields: [:]).isDroppable)
        XCTAssertFalse(Message.success(id: "1", result: [:]).isDroppable)
        XCTAssertFalse(Message.failure(id: "1", error: "x").isDroppable)
    }
}

final class OutboxTests: XCTestCase {
    private func line(_ text: String) -> Data { Data(text.utf8) }

    private func drain(_ outbox: Outbox) -> [String] {
        var lines: [String] = []
        while outbox.count > 0, let data = outbox.pop() {
            lines.append(String(decoding: data, as: UTF8.self))
            outbox.finishWrite()
        }
        return lines
    }

    func testDeliversInArrivalOrder() {
        let outbox = Outbox(capacity: 4)
        for text in ["a", "b", "c"] { XCTAssertTrue(outbox.push(line(text), droppable: false)) }
        XCTAssertEqual(drain(outbox), ["a", "b", "c"])
    }

    func testFullQueueEvictsOldestDroppableForANewerOne() {
        let outbox = Outbox(capacity: 2)
        XCTAssertTrue(outbox.push(line("s1"), droppable: true))
        XCTAssertTrue(outbox.push(line("r1"), droppable: false))
        XCTAssertTrue(outbox.push(line("s2"), droppable: true))
        XCTAssertEqual(outbox.dropped, 1)
        XCTAssertEqual(drain(outbox), ["r1", "s2"])
    }

    func testFullQueueOfResponsesDropsNewDroppableLine() {
        let outbox = Outbox(capacity: 2)
        outbox.push(line("r1"), droppable: false)
        outbox.push(line("r2"), droppable: false)
        XCTAssertFalse(outbox.push(line("s1"), droppable: true))
        XCTAssertEqual(outbox.dropped, 1)
        XCTAssertEqual(drain(outbox), ["r1", "r2"])
    }

    func testResponsesAreNeverDroppedWhenFull() {
        let outbox = Outbox(capacity: 1)
        outbox.push(line("s1"), droppable: true)
        XCTAssertTrue(outbox.push(line("r1"), droppable: false))
        XCTAssertTrue(outbox.push(line("r2"), droppable: false))
        XCTAssertEqual(outbox.dropped, 0)
        XCTAssertEqual(drain(outbox), ["s1", "r1", "r2"])
    }

    func testClosedOutboxRejectsPushesAndReleasesTheWriter() {
        let outbox = Outbox(capacity: 2)
        let popped = expectation(description: "pop returns nil once closed")
        Thread.detachNewThread {
            if outbox.pop() == nil { popped.fulfill() }
        }
        outbox.close()
        wait(for: [popped], timeout: 2)
        XCTAssertFalse(outbox.push(line("r1"), droppable: false))
    }

    func testWaitUntilFlushedTimesOutWhileTheWriterIsStuck() {
        let outbox = Outbox(capacity: 2)
        outbox.push(line("r1"), droppable: false)
        _ = outbox.pop() // taken by a writer that never finishes
        let start = Date()
        XCTAssertFalse(outbox.waitUntilFlushed(before: start.addingTimeInterval(0.1)))
        XCTAssertLessThan(Date().timeIntervalSince(start), 1)
    }

    func testWaitUntilFlushedReturnsOnceWritten() {
        let outbox = Outbox(capacity: 2)
        outbox.push(line("r1"), droppable: false)
        Thread.detachNewThread {
            _ = outbox.pop()
            Thread.sleep(forTimeInterval: 0.05)
            outbox.finishWrite()
        }
        XCTAssertTrue(outbox.waitUntilFlushed(before: Date().addingTimeInterval(2)))
    }

    func testWriterStopsAndClosesTheOutboxOnAWriteFailure() {
        let outbox = Outbox(capacity: 4)
        outbox.push(line("r1"), droppable: false)
        outbox.push(line("r2"), droppable: false)
        outbox.push(line("r3"), droppable: false)
        var written: [String] = []
        let failure = outbox.drain { data in
            written.append(String(decoding: data, as: UTF8.self))
            return written.count == 2 ? EPIPE : nil
        }
        XCTAssertEqual(failure, EPIPE)
        XCTAssertEqual(written, ["r1", "r2"], "the writer kept writing after a failure")
        XCTAssertFalse(outbox.push(line("r4"), droppable: false), "a failed writer left the outbox open")
        XCTAssertFalse(outbox.waitUntilFlushed(before: Date().addingTimeInterval(1)))
    }

    func testWriterReturnsWithoutFailureOnceClosed() {
        let outbox = Outbox(capacity: 4)
        outbox.push(line("r1"), droppable: false)
        let finished = expectation(description: "drain returns once the outbox is closed")
        let lines = LineLog()
        Thread.detachNewThread {
            let failure = outbox.drain { lines.append($0); return nil }
            XCTAssertNil(failure)
            finished.fulfill()
        }
        XCTAssertTrue(outbox.waitUntilFlushed(before: Date().addingTimeInterval(2)))
        outbox.close()
        wait(for: [finished], timeout: 2)
        XCTAssertEqual(lines.values, ["r1"])
    }
}

private final class LineLog: @unchecked Sendable {
    private let lock = NSLock()
    private var lines: [String] = []
    var values: [String] { lock.lock(); defer { lock.unlock() }; return lines }
    func append(_ data: Data) {
        lock.lock(); defer { lock.unlock() }
        lines.append(String(decoding: data, as: UTF8.self))
    }
}
