import XCTest
@testable import Nu11SignalProtocol

final class PlaylistStartTests: XCTestCase {
    private func index(_ args: JSONObject) throws -> Int? {
        try PlaylistStart.index(Request(id: "1", cmd: "playPlaylist", args: args))
    }

    func testStartIndexIsOptional() throws {
        XCTAssertNil(try index(["playlistId": "p1"]))
        XCTAssertEqual(try index(["playlistId": "p1", "startIndex": 0]), 0)
        XCTAssertEqual(try index(["playlistId": "p1", "startIndex": 3]), 3)
    }

    func testNegativeOrNonIntegerStartIndexesAreErrors() {
        let cases: [(String, Any)] = [("negative", -1), ("fraction", 1.5), ("string", "2"), ("bool", true)]
        for (name, value) in cases {
            XCTAssertThrowsError(try index(["startIndex": value]), name) { error in
                XCTAssertEqual(String(describing: error), #""startIndex" must be a non-negative integer"#, name)
            }
        }
    }

    func testItemPicksTheSongAtTheIndex() throws {
        XCTAssertEqual(try PlaylistStart.item(in: ["a", "b", "c"], at: 0), "a")
        XCTAssertEqual(try PlaylistStart.item(in: ["a", "b", "c"], at: 2), "c")
        XCTAssertThrowsError(try PlaylistStart.item(in: ["a", "b", "c"], at: 3)) { error in
            XCTAssertEqual(String(describing: error), "startIndex 3 is out of range: the playlist has 3 songs")
        }
        XCTAssertThrowsError(try PlaylistStart.item(in: [String](), at: 0)) { error in
            XCTAssertEqual(String(describing: error), "startIndex 0 is out of range: the playlist has 0 songs")
        }
    }
}

final class LibraryDurationTests: XCTestCase {
    func testMillisecondsBecomeSeconds() {
        // As observed on macOS for library songs: 3:52.827 reported as ms.
        XCTAssertEqual(LibraryDuration.seconds(232_827), 232.827, accuracy: 1e-9)
        XCTAssertEqual(LibraryDuration.seconds(7_201), 7.201, accuracy: 1e-9)
    }

    func testSecondsStaySeconds() {
        XCTAssertEqual(LibraryDuration.seconds(232.827), 232.827)
        XCTAssertEqual(LibraryDuration.seconds(LibraryDuration.millisecondsAbove), 7200)
    }

    func testUnknownDurationsAreZero() {
        XCTAssertEqual(LibraryDuration.seconds(nil), 0)
        XCTAssertEqual(LibraryDuration.seconds(-5), 0)
        XCTAssertEqual(LibraryDuration.seconds(.nan), 0)
        XCTAssertEqual(LibraryDuration.seconds(.infinity), 0)
    }
}
