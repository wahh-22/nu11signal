import Foundation
import XCTest
@testable import Nu11SignalProtocol

final class SpectrumBandsTests: XCTestCase {
    func testBandsAreAscendingNonEmptyAndInsideTheSpectrum() {
        for rate in [44_100.0, 48_000.0] {
            let ranges = SpectrumBands.binRanges(sampleRate: rate, fftSize: 2048, bands: 24, minHz: 40, maxHz: 16_000)
            XCTAssertEqual(ranges.count, 24)
            var previous = 0
            for range in ranges {
                XCTAssertFalse(range.isEmpty)
                XCTAssertGreaterThanOrEqual(range.lowerBound, 1, "the DC bin is never used")
                XCTAssertLessThanOrEqual(range.upperBound, 1024, "the Nyquist bin is never used")
                XCTAssertGreaterThanOrEqual(range.lowerBound, previous)
                previous = range.lowerBound
            }
        }
    }

    func testANarrowLowBandTakesTheBinNearestItsCenter() {
        // At 48 kHz a 2048-point bin is 23.4 Hz wide: the 51-66 Hz band has
        // no bin of its own and reads bin 2 (46.9 Hz), nearest its 58 Hz center.
        let ranges = SpectrumBands.binRanges(sampleRate: 48_000, fftSize: 2048, bands: 24, minHz: 40, maxHz: 16_000)
        XCTAssertEqual(ranges[0], 2..<3)
        XCTAssertEqual(ranges[1], 2..<3)
    }

    func testTheTopBandEndsAtMaxHz() {
        let ranges = SpectrumBands.binRanges(sampleRate: 48_000, fftSize: 2048, bands: 24, minHz: 40, maxHz: 16_000)
        // 16 kHz is bin 682.7: the top band stops before bin 683.
        XCTAssertEqual(ranges[23].upperBound, 683)
    }

    func testBandOfAFrequency() {
        let edges = SpectrumBands.edges(bands: 24, minHz: 40, maxHz: 16_000)
        XCTAssertEqual(edges.count, 25)
        XCTAssertEqual(edges[0], 40, accuracy: 1e-9)
        XCTAssertEqual(edges[24], 16_000, accuracy: 1e-6)
        XCTAssertEqual(SpectrumBands.band(of: 1_000, bands: 24, minHz: 40, maxHz: 16_000), 12)
        XCTAssertNil(SpectrumBands.band(of: 20, bands: 24, minHz: 40, maxHz: 16_000))
    }
}

final class HannWindowTests: XCTestCase {
    func testPeriodicHannCoefficients() {
        let window = HannWindow.coefficients(8)
        XCTAssertEqual(window.count, 8)
        XCTAssertEqual(window[0], 0, accuracy: 1e-6)
        XCTAssertEqual(window[4], 1, accuracy: 1e-6)
        XCTAssertEqual(window[2], 0.5, accuracy: 1e-6)
        XCTAssertEqual(window[1], window[7], accuracy: 1e-6)
    }
}

final class LevelScaleTests: XCTestCase {
    func testPowerMapsLinearlyInDecibelsBetweenFloorAndCeiling() {
        XCTAssertEqual(LevelScale.normalized(power: 0), 0)
        XCTAssertEqual(LevelScale.normalized(power: pow(10, -60 / 10)), 0)
        XCTAssertEqual(LevelScale.normalized(power: pow(10, -50 / 10)), 0, accuracy: 1e-9)
        XCTAssertEqual(LevelScale.normalized(power: pow(10, -30 / 10)), 0.5, accuracy: 1e-9)
        XCTAssertEqual(LevelScale.normalized(power: pow(10, -10 / 10)), 1, accuracy: 1e-9)
        XCTAssertEqual(LevelScale.normalized(power: 1), 1)
        XCTAssertEqual(LevelScale.normalized(power: .nan), 0)
    }

    func testQuantizedToPercentages() {
        XCTAssertEqual(LevelScale.quantized([0, 0.5, 0.994, 1, 1.2, -1, .nan]), [0, 50, 99, 100, 100, 0, 0])
    }
}

final class LevelSmootherTests: XCTestCase {
    func testAttackIsFastAndReleaseIsSlow() {
        var smoother = LevelSmoother(count: 1)
        let risen = smoother.update([1])[0]
        XCTAssertGreaterThanOrEqual(risen, 0.6, "one frame gets most of the way up")
        _ = smoother.update([1])
        _ = smoother.update([1])
        XCTAssertGreaterThan(smoother.update([1])[0], 0.95)

        let fallen = smoother.update([0])[0]
        XCTAssertGreaterThan(fallen, 0.7, "one frame falls only a little")
        var level = fallen
        for _ in 0..<15 { level = smoother.update([0])[0] }
        XCTAssertLessThan(level, 0.05, "about a second at 15 Hz reaches the floor")
    }

    func testResetStartsFromSilence() {
        var smoother = LevelSmoother(count: 2)
        _ = smoother.update([1, 1])
        smoother.reset()
        XCTAssertEqual(smoother.update([0, 0]), [0, 0])
    }

    func testTinyLevelsSnapToZero() {
        var smoother = LevelSmoother(count: 1)
        _ = smoother.update([0.001])
        XCTAssertEqual(smoother.update([0])[0], 0)
    }
}

final class SpectrumAnalyzerTests: XCTestCase {
    private func sine(_ hz: Double, amplitude: Double, rate: Double, count: Int = 2048) -> [Float] {
        (0..<count).map { Float(amplitude * sin(2 * .pi * hz * Double($0) / rate)) }
    }

    func testSilenceIsZero() {
        let analyzer = SpectrumAnalyzer(sampleRate: 48_000)
        XCTAssertEqual(analyzer.levels([Float](repeating: 0, count: 2048)), [Double](repeating: 0, count: 24))
    }

    func testASineLandsInItsBand() {
        for (hz, rate) in [(1_000.0, 48_000.0), (200.0, 44_100.0), (5_000.0, 44_100.0), (100.0, 48_000.0)] {
            let analyzer = SpectrumAnalyzer(sampleRate: rate)
            let levels = analyzer.levels(sine(hz, amplitude: 0.25, rate: rate))
            let expected = SpectrumBands.band(of: hz, bands: 24, minHz: 40, maxHz: 16_000)!
            let loudest = levels.indices.max(by: { levels[$0] < levels[$1] })!
            XCTAssertEqual(loudest, expected, "\(hz) Hz at \(rate) Hz")
            XCTAssertGreaterThan(levels[expected], 0.8, "a -12 dBFS sine is near the top")
            // Bands well away from the tone stay low (the Hann skirts).
            for band in levels.indices where abs(band - expected) >= 4 {
                XCTAssertLessThan(levels[band], levels[expected] - 0.4, "\(hz) Hz band \(band)")
            }
        }
    }

    func testAQuietSineReadsLowerThanALoudOne() {
        let analyzer = SpectrumAnalyzer(sampleRate: 48_000)
        let loud = analyzer.levels(sine(1_000, amplitude: 0.25, rate: 48_000))[12]
        let quiet = analyzer.levels(sine(1_000, amplitude: 0.025, rate: 48_000))[12]
        // 20 dB quieter is half of the 40 dB range lower.
        XCTAssertEqual(loud - quiet, 20.0 / 40.0, accuracy: 0.05)
    }

    func testAFullScaleToneIsClampedToOne() {
        let analyzer = SpectrumAnalyzer(sampleRate: 48_000)
        XCTAssertEqual(analyzer.levels(sine(1_000, amplitude: 1, rate: 48_000))[12], 1)
    }
}

@available(macOS 15.0, *)
final class SampleRingTests: XCTestCase {
    func testReadsNothingUntilEnoughIsWritten() {
        let ring = SampleRing(capacity: 16)
        var out = [Float](repeating: -1, count: 8)
        var input: [Float] = [1, 2, 3]
        input.withUnsafeBufferPointer { ring.write(source: $0.baseAddress!, stride: 1, channels: 1, frames: 3) }
        XCTAssertFalse(out.withUnsafeMutableBufferPointer { ring.readLatest(into: $0.baseAddress!, count: 8) })
        input = [4, 5, 6, 7, 8]
        input.withUnsafeBufferPointer { ring.write(source: $0.baseAddress!, stride: 1, channels: 1, frames: 5) }
        XCTAssertTrue(out.withUnsafeMutableBufferPointer { ring.readLatest(into: $0.baseAddress!, count: 8) })
        XCTAssertEqual(out, [1, 2, 3, 4, 5, 6, 7, 8])
    }

    func testMixesInterleavedChannelsToMonoAcrossTheWrap() {
        let ring = SampleRing(capacity: 8)
        // Three writes of four stereo frames: frame n is (n, n + 1), mono n + 0.5.
        for block in 0..<3 {
            var interleaved: [Float] = []
            for frame in 0..<4 {
                let n = Float(block * 4 + frame)
                interleaved += [n, n + 1]
            }
            interleaved.withUnsafeBufferPointer { ring.write(source: $0.baseAddress!, stride: 2, channels: 2, frames: 4) }
        }
        var out = [Float](repeating: 0, count: 6)
        XCTAssertTrue(out.withUnsafeMutableBufferPointer { ring.readLatest(into: $0.baseAddress!, count: 6) })
        XCTAssertEqual(out, [6.5, 7.5, 8.5, 9.5, 10.5, 11.5])
    }

    func testTakesOnlyTheTapChannelsOfAWiderBuffer() {
        let ring = SampleRing(capacity: 8)
        // Three channels per frame; the tap is the first two of them.
        let interleaved: [Float] = [1, 3, 100, 5, 7, 100]
        interleaved.withUnsafeBufferPointer { ring.write(source: $0.baseAddress!, stride: 3, channels: 2, frames: 2) }
        var out = [Float](repeating: 0, count: 2)
        XCTAssertTrue(out.withUnsafeMutableBufferPointer { ring.readLatest(into: $0.baseAddress!, count: 2) })
        XCTAssertEqual(out, [2, 6])
    }

    func testResetForgetsWhatWasWritten() {
        let ring = SampleRing(capacity: 8)
        let input: [Float] = [1, 2, 3, 4]
        input.withUnsafeBufferPointer { ring.write(source: $0.baseAddress!, stride: 1, channels: 1, frames: 4) }
        ring.reset()
        var out = [Float](repeating: 0, count: 2)
        XCTAssertFalse(out.withUnsafeMutableBufferPointer { ring.readLatest(into: $0.baseAddress!, count: 2) })
    }

    func testAWriteLongerThanTheRingKeepsItsNewestSamples() {
        let ring = SampleRing(capacity: 4)
        let input: [Float] = [1, 2, 3, 4, 5, 6]
        input.withUnsafeBufferPointer { ring.write(source: $0.baseAddress!, stride: 1, channels: 1, frames: 6) }
        var out = [Float](repeating: 0, count: 4)
        XCTAssertTrue(out.withUnsafeMutableBufferPointer { ring.readLatest(into: $0.baseAddress!, count: 4) })
        XCTAssertEqual(out, [3, 4, 5, 6])
    }
}
