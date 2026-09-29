import Foundation
import XCTest
@testable import Nu11SignalProtocol

final class CatalogBudgetTests: XCTestCase {
    /// Every catalog command must answer before the Go client gives up on
    /// it, so the helper's own timeout error reaches the UI.
    func testCommandsFinishBeforeTheGoClientTimesOut() {
        XCTAssertLessThan(CatalogBudget.artist, CatalogBudget.goArtistCallTimeout)
        XCTAssertLessThan(CatalogBudget.album, CatalogBudget.goDetailCallTimeout)
        XCTAssertLessThan(CatalogBudget.songAlbum, CatalogBudget.goDetailCallTimeout)
        XCTAssertLessThan(CatalogBudget.catalogPlaylist, CatalogBudget.goDetailCallTimeout)
    }

    func testCommandBudgetsAddUpTheirSteps() {
        XCTAssertEqual(CatalogBudget.artist, CatalogBudget.lookup + CatalogBudget.section)
        XCTAssertEqual(CatalogBudget.album, CatalogBudget.lookup)
        XCTAssertEqual(CatalogBudget.songAlbum, 2 * CatalogBudget.lookup)
        XCTAssertEqual(CatalogBudget.catalogPlaylist, CatalogBudget.lookup)
    }
}

final class RequiredIDTests: XCTestCase {
    func testReturnsTheID() throws {
        let request = Request(id: "1", cmd: "album", args: ["albumId": "1440857781"])
        XCTAssertEqual(try request.requiredID("albumId"), "1440857781")
    }

    func testMissingBlankOrMistypedIDsAreErrors() {
        let cases: [(String, JSONObject)] = [
            ("missing", [:]),
            ("empty", ["songId": ""]),
            ("blank", ["songId": "  \t"]),
            ("number", ["songId": 42]),
            ("other key", ["id": "s1"]),
        ]
        for (name, args) in cases {
            let request = Request(id: "1", cmd: "songAlbum", args: args)
            XCTAssertThrowsError(try request.requiredID("songId"), name) { error in
                XCTAssertEqual(String(describing: error), #"songAlbum requires a non-empty "songId""#, name)
            }
        }
    }
}

final class CatalogDateTests: XCTestCase {
    func testFormatsTheCalendarDateInUTC() {
        // 1975-06-27T00:00:00Z and 23:30Z: neither a zone west nor one east
        // of UTC may move them a day.
        XCTAssertEqual(CatalogDate.iso(Date(timeIntervalSince1970: 173_059_200)), "1975-06-27")
        XCTAssertEqual(CatalogDate.iso(Date(timeIntervalSince1970: 173_059_200 + 84_600)), "1975-06-27")
        XCTAssertEqual(CatalogDate.iso(Date(timeIntervalSince1970: 0)), "1970-01-01")
    }
}
