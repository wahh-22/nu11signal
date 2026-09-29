import XCTest
@testable import Nu11SignalProtocol

final class RepeatTests: XCTestCase {
    func testRequestedModeIsOneOfTheWireNames() throws {
        for mode in ["off", "all", "one"] {
            XCTAssertEqual(try RepeatSetting.requested(Request(id: "1", cmd: "setRepeat", args: ["mode": mode])), mode)
        }
    }

    func testAnyOtherModeIsAnError() {
        for value: Any? in [nil, "none", "ALL", 1, true] {
            var args: JSONObject = [:]
            args["mode"] = value
            XCTAssertThrowsError(try RepeatSetting.requested(Request(id: "1", cmd: "setRepeat", args: args))) { error in
                XCTAssertEqual(String(describing: error), #"setRepeat requires "mode": "off", "all" or "one""#)
            }
        }
    }

    func testSetRepeatWaitsItsTurnWithThePlaybackCommands() {
        // It changes how the current queue plays, so it must not overtake
        // (or be overtaken by) the playSongs that replaces the queue.
        XCTAssertTrue(Request(id: "1", cmd: "setRepeat").mutatesPlayback)
    }
}
