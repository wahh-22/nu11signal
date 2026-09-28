import Foundation
import XCTest
@testable import SoulKingProtocol

final class DeadlineTests: XCTestCase {
    func testReturnsTheResultOfAnOperationThatFinishesInTime() async throws {
        let value = try await Deadline.run(seconds: 2) { 42 }
        XCTAssertEqual(value, 42)
    }

    func testPropagatesTheOperationsError() async {
        struct Boom: Error {}
        do {
            _ = try await Deadline.run(seconds: 2) { () async throws -> Int in throw Boom() }
            XCTFail("expected the operation's error")
        } catch {
            XCTAssertTrue(error is Boom, "got \(error)")
        }
    }

    func testTimesOutAnOperationThatNeverFinishes() async {
        let start = Date()
        do {
            _ = try await Deadline.run(seconds: 0.05) { () async throws -> Int in
                // Ignores cancellation and never returns, like a hung call.
                await withCheckedContinuation { (_: CheckedContinuation<Void, Never>) in }
                return 1
            }
            XCTFail("expected a timeout")
        } catch let error as Deadline.TimedOut {
            XCTAssertEqual(error.seconds, 0.05)
        } catch {
            XCTFail("unexpected error \(error)")
        }
        XCTAssertLessThan(Date().timeIntervalSince(start), 2)
    }

    func testExtremeOrNonFiniteTimeoutsDoNotTrap() async throws {
        for seconds in [TimeInterval.infinity, 1e30, .nan, -1] {
            let value = try await Deadline.run(seconds: seconds) { 7 }
            XCTAssertEqual(value, 7, "seconds: \(seconds)")
        }
    }

    func testALateResultAfterATimeoutIsIgnored() async throws {
        do {
            _ = try await Deadline.run(seconds: 0.02) { () async throws -> Int in
                // Finishes (by throwing CancellationError) after the timeout.
                try await Task.sleep(nanoseconds: 5_000_000_000)
                return 1
            }
            XCTFail("expected a timeout")
        } catch is Deadline.TimedOut {}
        // A second resume of the same continuation would trap here.
        try await Task.sleep(nanoseconds: 100_000_000)
    }
}
