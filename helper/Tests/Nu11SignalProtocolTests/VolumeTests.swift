import XCTest
@testable import Nu11SignalProtocol

final class VolumeLevelTests: XCTestCase {
    private func requested(_ args: JSONObject) throws -> Float32 {
        try VolumeLevel.requested(Request(id: "1", cmd: "setVolume", args: args))
    }

    func testRequestedLevelIsClampedToTheUnitRange() throws {
        let cases: [(String, Any, Float32)] = [
            ("zero", 0, 0),
            ("middle", 0.25, 0.25),
            ("one", 1, 1),
            ("above", 1.7, 1),
            ("below", -0.3, 0),
            ("huge", 1e300, 1),
        ]
        for (name, level, want) in cases {
            XCTAssertEqual(try requested(["level": level]), want, name)
        }
    }

    func testMissingOrMistypedLevelsAreErrors() {
        let cases: [(String, JSONObject)] = [
            ("missing", [:]),
            ("string", ["level": "0.5"]),
            ("bool", ["level": true]),
            ("infinite", ["level": Double.infinity]),
            ("nan", ["level": Double.nan]),
        ]
        for (name, args) in cases {
            XCTAssertThrowsError(try requested(args), name) { error in
                XCTAssertEqual(String(describing: error), #"setVolume requires a number "level" from 0 to 1"#, name)
            }
        }
    }

    func testReportedLevelIsClampedAndRoundedToFourDecimals() {
        XCTAssertEqual(VolumeLevel.reported(0.5), 0.5)
        // 0.3 as a Float32 is 0.30000001192...; the wire carries 0.3.
        XCTAssertEqual(VolumeLevel.reported(0.3), 0.3)
        XCTAssertEqual(VolumeLevel.reported(0.43755), 0.4376)
        XCTAssertEqual(VolumeLevel.reported(1.2), 1)
        XCTAssertEqual(VolumeLevel.reported(-0.1), 0)
        XCTAssertEqual(VolumeLevel.reported(.nan), 0)
    }
}
