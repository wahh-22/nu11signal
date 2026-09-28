import Foundation
import XCTest
@testable import SoulKingProtocol

final class CodecTests: XCTestCase {
    func testDecodesRequestWithTypedArgs() throws {
        let line = #"{"id":"7","cmd":"playSongs","ids":["a","b"],"startIndex":1,"seconds":12.5}"#
        let request = try Codec.decode(line).get()
        XCTAssertEqual(request.id, "7")
        XCTAssertEqual(request.cmd, "playSongs")
        XCTAssertEqual(request.strings("ids"), ["a", "b"])
        XCTAssertEqual(request.int("startIndex"), 1)
        XCTAssertEqual(request.double("seconds"), 12.5)
        XCTAssertNil(request.args["id"])
        XCTAssertNil(request.args["cmd"])
    }

    func testBooleansAreNotNumbers() throws {
        let request = try Codec.decode(#"{"id":"1","cmd":"search","limit":true}"#).get()
        XCTAssertNil(request.int("limit"))
    }

    func testMalformedJSONFailsWithEmptyID() {
        XCTAssertEqual(Codec.decode("not json").failureValue,
                       DecodeFailure(id: "", message: "malformed JSON request"))
    }

    func testMissingCmdKeepsID() {
        XCTAssertEqual(Codec.decode(#"{"id":"9"}"#).failureValue?.id, "9")
    }

    func testMissingIDFailsWithEmptyID() {
        XCTAssertEqual(Codec.decode(#"{"cmd":"pause"}"#).failureValue?.id, "")
    }

    func testEncodesSuccessAsSingleLine() throws {
        let data = Codec.encode(.success(id: "1", result: ["status": "authorized"]))
        XCTAssertEqual(String(decoding: data, as: UTF8.self),
                       #"{"id":"1","ok":true,"result":{"status":"authorized"}}"# + "\n")
    }

    func testEncodesFailure() {
        let data = Codec.encode(.failure(id: "", error: "boom"))
        XCTAssertEqual(String(decoding: data, as: UTF8.self),
                       #"{"error":"boom","id":"","ok":false}"# + "\n")
    }

    func testEncodesEventWithoutID() {
        let data = Codec.encode(.event(name: "ready", fields: [:]))
        XCTAssertEqual(String(decoding: data, as: UTF8.self), #"{"event":"ready"}"# + "\n")
    }
}

private extension Result {
    var failureValue: Failure? {
        if case let .failure(error) = self { return error }
        return nil
    }
}
