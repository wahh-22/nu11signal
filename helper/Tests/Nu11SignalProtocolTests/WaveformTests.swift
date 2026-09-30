import Foundation
import XCTest
@testable import Nu11SignalProtocol

final class WaveformTests: XCTestCase {
    func testASineAlternatesSignBucketByBucket() {
        // A period of 64 samples over 2048 samples and 64 points: each
        // point covers half a period, the positive or the negative lobe.
        let samples = (0..<2048).map { Float(0.8 * sin(2 * .pi * (Double($0) + 0.5) / 64)) }
        let wave = Waveform.decimated(samples, points: 64)
        XCTAssertEqual(wave.count, 64)
        for (i, point) in wave.enumerated() {
            if i % 2 == 0 {
                XCTAssertEqual(point, 0.8, accuracy: 0.01, "point \(i)")
            } else {
                XCTAssertEqual(point, -0.8, accuracy: 0.01, "point \(i)")
            }
        }
    }

    func testSilenceIsFlat() {
        let wave = Waveform.decimated([Float](repeating: 0, count: 2048), points: 64)
        XCTAssertEqual(wave, [Double](repeating: 0, count: 64))
        XCTAssertEqual(Waveform.quantized(wave), [Int](repeating: 0, count: 64))
    }

    func testEachPointKeepsItsBucketsLargestSwing() {
        // A lone spike survives decimation instead of being averaged away.
        var samples = [Float](repeating: 0.1, count: 16)
        samples[5] = -0.9
        XCTAssertEqual(Waveform.decimated(samples, points: 4), [0.1, -0.9, 0.1, 0.1].map { Double(Float($0)) })
    }

    func testOverloadedSamplesAreClipped() {
        let wave = Waveform.decimated([2, 2, -3, -3, .nan, .nan, .infinity, .infinity], points: 4)
        XCTAssertEqual(wave, [1, -1, 0, 1])
    }

    func testUnevenBucketsCoverEverySample() {
        // 10 samples on 3 points: buckets of 3, 3 and 4 samples.
        let samples: [Float] = [0, 0, 0.3, 0, 0.5, 0, 0, 0, 0, -0.7]
        XCTAssertEqual(Waveform.decimated(samples, points: 3), [Double(Float(0.3)), 0.5, Double(Float(-0.7))])
    }

    func testFewerSamplesThanPointsOrNoPoints() {
        XCTAssertEqual(Waveform.decimated([0.5], points: 4), [0, 0, 0, 0])
        XCTAssertEqual(Waveform.decimated([0.5, 0.5], points: 0), [])
    }

    func testQuantizedIsHundredthsClamped() {
        XCTAssertEqual(Waveform.quantized([0, 0.5, -0.5, 1, -1, 1.5, -2, .nan, 0.004, -0.006]),
                       [0, 50, -50, 100, -100, 100, -100, 0, 0, -1])
    }
}
