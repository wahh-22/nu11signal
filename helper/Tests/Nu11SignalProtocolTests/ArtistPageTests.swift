import Foundation
import XCTest
@testable import Nu11SignalProtocol

final class EditorialTextTests: XCTestCase {
    func testStripsTagsAndDecodesEntities() {
        let html = "<p>Daft Punk&#39;s <i>Discovery</i> &amp; <b>Homework</b> &lt;3</p>"
        XCTAssertEqual(EditorialText.plain(html), "Daft Punk's Discovery & Homework <3")
    }

    func testBreaksAndParagraphsBecomeNewlines() {
        let html = "<p>First.</p><p>Second<br/>line.</p>"
        XCTAssertEqual(EditorialText.plain(html), "First.\nSecond\nline.")
    }

    func testDecodesNumericAndNamedEntities() {
        XCTAssertEqual(EditorialText.plain("caf&#233; &#x2014; &quot;x&quot;&nbsp;&apos;y&apos;"),
                       "café — \"x\" 'y'")
    }

    func testCollapsesWhitespaceAndKeepsUnknownEntities() {
        XCTAssertEqual(EditorialText.plain("  a \n\n\n  b\t\tc &bogus; &#xZZ; "), "a\nb c &bogus; &#xZZ;")
    }

    func testDropsControlCharactersButKeepsNewlines() {
        // A decoded ESC would let catalog text drive the terminal.
        XCTAssertEqual(EditorialText.plain("a&#27;[31mb&#x1B;c"), "a[31mbc")
        XCTAssertEqual(EditorialText.plain("bell\u{7}\u{7F}&#128;&#x9F;!"), "bell!")
        XCTAssertEqual(EditorialText.plain("one&#10;two"), "one\ntwo")
    }

    func testControlWhitespaceBecomesSpaces() {
        XCTAssertEqual(EditorialText.plain("tab&#9;bed\tx&#133;y"), "tab bed x y")
        XCTAssertEqual(EditorialText.plain("crlf\r\nline"), "crlf\nline")
    }

    func testPlainTextPassesThrough() {
        XCTAssertEqual(EditorialText.plain("Rock & roll > pop"), "Rock & roll > pop")
        XCTAssertEqual(EditorialText.plain(""), "")
    }
}

final class ArtistFactsTests: XCTestCase {
    func testParsesOriginAndBornOrFormed() {
        let json = #"{"data":[{"id":"5468295","attributes":{"name":"Daft Punk","origin":"Paris, France","bornOrFormed":"1993"}}]}"#
        XCTAssertEqual(ArtistFacts.parse(Data(json.utf8)), ArtistFacts(origin: "Paris, France", formed: "1993"))
    }

    func testMissingOrMalformedFieldsAreEmpty() {
        let cases = [
            #"{"data":[{"attributes":{"name":"x"}}]}"#,
            #"{"data":[]}"#,
            #"{"data":[{"attributes":{"origin":7,"bornOrFormed":null}}]}"#,
            #"{"errors":[{"status":"404"}]}"#,
            "not json",
        ]
        for json in cases {
            XCTAssertEqual(ArtistFacts.parse(Data(json.utf8)), ArtistFacts(origin: "", formed: ""), json)
        }
    }

    func testFormedKeepsOnlyTheYearOfAFullDate() {
        let json = #"{"data":[{"attributes":{"bornOrFormed":"1993-01-01"}}]}"#
        XCTAssertEqual(ArtistFacts.parse(Data(json.utf8)).formed, "1993")
    }
}
