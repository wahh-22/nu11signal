import XCTest
@testable import Nu11SignalProtocol

final class CatalogSearchQueryTests: XCTestCase {
    private func query(_ args: JSONObject) throws -> CatalogSearchQuery {
        try CatalogSearchQuery(Request(id: "1", cmd: "searchCatalog", args: args))
    }

    func testKeepsTheTermAndLimit() throws {
        let q = try query(["term": "queen", "limit": 10])
        XCTAssertEqual(q.term, "queen")
        XCTAssertEqual(q.limit, 10)
        XCTAssertEqual(q.suggestionLimit, 10)
    }

    func testMissingBlankOrMistypedTermsAreErrors() {
        let cases: [(String, JSONObject)] = [
            ("missing", [:]),
            ("empty", ["term": ""]),
            ("blank", ["term": " \t\n"]),
            ("number", ["term": 7]),
        ]
        for (name, args) in cases {
            XCTAssertThrowsError(try query(args), name) { error in
                XCTAssertEqual(String(describing: error), #"searchCatalog requires a non-empty "term""#, name)
            }
        }
    }

    func testLimitDefaultsAndClampsToThePageSize() throws {
        let cases: [(String, JSONObject, Int)] = [
            ("default", ["term": "q"], 25),
            ("zero", ["term": "q", "limit": 0], 1),
            ("negative", ["term": "q", "limit": -3], 1),
            ("one", ["term": "q", "limit": 1], 1),
            ("max", ["term": "q", "limit": 25], 25),
            ("above", ["term": "q", "limit": 1000], 25),
            ("integral double", ["term": "q", "limit": 5.0], 5),
        ]
        for (name, args, want) in cases {
            XCTAssertEqual(try query(args).limit, want, name)
        }
    }

    func testSuggestionsStayWithinTheirCap() throws {
        XCTAssertEqual(try query(["term": "q", "limit": 3]).suggestionLimit, 3)
        XCTAssertEqual(try query(["term": "q", "limit": 10]).suggestionLimit, 10)
        XCTAssertEqual(try query(["term": "q", "limit": 11]).suggestionLimit, 10)
        XCTAssertEqual(try query(["term": "q"]).suggestionLimit, CatalogSearchQuery.maxSuggestions)
    }

    func testTopResultsStayWithinTheirCap() throws {
        XCTAssertEqual(try query(["term": "q", "limit": 3]).topResultLimit, 3)
        XCTAssertEqual(try query(["term": "q", "limit": 6]).topResultLimit, 6)
        XCTAssertEqual(try query(["term": "q", "limit": 25]).topResultLimit, 6)
        XCTAssertEqual(try query(["term": "q"]).topResultLimit, CatalogSearchQuery.maxTopResults)
    }

    func testNonIntegerLimitsAreErrors() {
        for (name, limit) in [("fraction", 2.5 as Any), ("string", "10"), ("bool", true)] {
            XCTAssertThrowsError(try query(["term": "q", "limit": limit]), name) { error in
                XCTAssertEqual(String(describing: error), #""limit" must be an integer"#, name)
            }
        }
    }
}
