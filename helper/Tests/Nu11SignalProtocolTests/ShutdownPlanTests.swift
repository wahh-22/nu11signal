import XCTest
@testable import Nu11SignalProtocol

final class ShutdownPlanTests: XCTestCase {
    private func index(_ steps: [ShutdownPlan.Step], _ match: (ShutdownPlan.Step) -> Bool,
                       file: StaticString = #filePath, line: UInt = #line) -> Int {
        guard let found = steps.firstIndex(where: match) else {
            XCTFail("step missing from \(steps)", file: file, line: line)
            return -1
        }
        return found
    }

    func testARenderingTapFadesToSilenceBeforeThePauseAndPausesBeforeTheTeardown() {
        let steps = ShutdownPlan.steps(rendering: true, playing: true)
        let fade = index(steps) { if case .fadeOut = $0 { return true }; return false }
        let pause = index(steps) { $0 == .pausePlayer }
        let paused = index(steps) { if case .awaitPaused = $0 { return true }; return false }
        let teardown = index(steps) { $0 == .teardown }
        XCTAssertLessThan(fade, pause, "the gain reaches 0 before the player is touched")
        XCTAssertLessThan(pause, paused)
        XCTAssertLessThan(paused, teardown, "the tap is released only once the player is paused")
        XCTAssertEqual(teardown, steps.count - 1, "nothing follows the teardown but the exit")
    }

    func testTheTapStaysMutedUntilTheTeardownReleasesIt() {
        // The teardown is the only step that releases the tap (and so
        // unmutes the player's process), and it happens exactly once.
        let steps = ShutdownPlan.steps(rendering: true, playing: true)
        XCTAssertEqual(steps.filter { $0 == .teardown }.count, 1)
    }

    func testTheFadeWaitCoversTheWholeShutdownRamp() {
        guard case let .fadeOut(wait) = ShutdownPlan.steps(rendering: true, playing: true).first else {
            return XCTFail("a rendering tap starts with the fade")
        }
        XCTAssertGreaterThanOrEqual(wait, GainRamp.shutdownSeconds)
        XCTAssertLessThan(wait, GainRamp.shutdownSeconds + 0.1)
    }

    func testTheSystemVolumeSkipsTheFadeButLetsThePauseSettle() {
        let steps = ShutdownPlan.steps(rendering: false, playing: true)
        XCTAssertFalse(steps.contains { if case .fadeOut = $0 { return true }; return false })
        XCTAssertEqual(steps.first, .pausePlayer)
        XCTAssertTrue(steps.contains(.settle(seconds: ShutdownPlan.settleSeconds)))
        XCTAssertGreaterThanOrEqual(ShutdownPlan.settleSeconds, 0.15)
    }

    func testNothingPlayingOnlyReleasesTheTap() {
        XCTAssertEqual(ShutdownPlan.steps(rendering: false, playing: false), [.teardown])
    }

    func testTheBackstopCoversTheLongestQuietExit() {
        let longest = [true, false].flatMap { rendering in [true, false].map { playing in
            ShutdownPlan.duration(ShutdownPlan.steps(rendering: rendering, playing: playing))
        } }.max() ?? 0
        XCTAssertGreaterThanOrEqual(longest, GainRamp.shutdownSeconds)
        XCTAssertGreaterThan(ShutdownPlan.backstopSeconds, longest + 1, "room for the teardown and exit")
    }

    func testTheShutdownRampReachesSilenceWithinItsDurationAndNotAbruptly() {
        let sampleRate = 48_000.0
        let increment = GainRamp.increment(sampleRate: sampleRate, seconds: GainRamp.shutdownSeconds)
        let frames = Int((sampleRate * GainRamp.shutdownSeconds).rounded(.up))
        XCTAssertEqual(GainRamp.gain(atFrame: frames, start: 1, target: 0, increment: increment), 0)
        // Halfway through, about half the gain is left: a fade, not a cut.
        let half = GainRamp.gain(atFrame: frames / 2, start: 1, target: 0, increment: increment)
        XCTAssertEqual(half, 0.5, accuracy: 0.01)
        // The shutdown ramp is much slower than the level-change ramp.
        XCTAssertLessThan(increment, GainRamp.increment(sampleRate: sampleRate) / 10)
    }

    func testTheShutdownRampAppliedToABufferEndsInSilence() {
        let sampleRate = 1_000.0
        let increment = GainRamp.increment(sampleRate: sampleRate, seconds: GainRamp.shutdownSeconds)
        let frames = Int(sampleRate * GainRamp.shutdownSeconds) + 10
        let source = [Float](repeating: 1, count: frames)
        var destination = [Float](repeating: 9, count: frames)
        source.withUnsafeBufferPointer { src in
            destination.withUnsafeMutableBufferPointer { dst in
                GainRamp.apply(source: src.baseAddress, sourceStride: 1, sourceFrames: frames,
                               destination: dst.baseAddress!, destinationStride: 1, frames: frames,
                               start: 1, target: 0, increment: increment)
            }
        }
        XCTAssertEqual(destination.first, 1)
        XCTAssertEqual(destination.last, 0)
        for (previous, next) in zip(destination, destination.dropFirst()) {
            XCTAssertLessThanOrEqual(next, previous)
            XCTAssertLessThanOrEqual(previous - next, increment + 1e-6, "no step larger than the ramp's")
        }
    }

    func testInvalidRatesStillJumpRatherThanTrap() {
        XCTAssertEqual(GainRamp.increment(sampleRate: 0, seconds: GainRamp.shutdownSeconds), 1)
        XCTAssertEqual(GainRamp.increment(sampleRate: 48_000, seconds: 0), 1)
    }
}
