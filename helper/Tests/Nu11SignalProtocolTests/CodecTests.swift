import Foundation
import XCTest
@testable import Nu11SignalProtocol

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

    func testReadOnlyCommandsDoNotMutatePlayback() {
        for cmd in ["authorize", "search", "searchCatalog", "playlists"] {
            XCTAssertFalse(Request(id: "1", cmd: cmd).mutatesPlayback, cmd)
        }
        XCTAssertTrue(Request(id: "1", cmd: "playSongs").mutatesPlayback)
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

    func testIntRejectsFractionalNumbersInsteadOfTruncating() throws {
        let request = try Codec.decode(#"{"id":"1","cmd":"search","limit":1.9,"startIndex":-2.5}"#).get()
        XCTAssertNil(request.int("limit"))
        XCTAssertNil(request.int("startIndex"))
    }

    func testIntRejectsValuesOutsideTheExactRange() throws {
        let request = try Codec.decode(#"{"id":"1","cmd":"search","huge":1e30,"big":9007199254740993}"#).get()
        XCTAssertNil(request.int("huge"))
        XCTAssertNil(request.int("big"))
    }

    func testIntRangeIsTheSameForIntegerAndFloatingPointNumbers() throws {
        // ±(2^53 - 1) is the largest exact range; 2^53 itself is ambiguous
        // (2^53 + 1 rounds to it), so it is rejected in either spelling.
        let request = try Codec.decode(
            #"{"id":"1","cmd":"search","maxInt":9007199254740991,"minInt":-9007199254740991,"# +
                #""limitInt":9007199254740992,"limitNegInt":-9007199254740992,"# +
                #""maxDouble":9007199254740991.0,"limitDouble":9007199254740992.0,"limitNegDouble":-9007199254740992.0}"#
        ).get()
        XCTAssertEqual(request.int("maxInt"), 9_007_199_254_740_991)
        XCTAssertEqual(request.int("minInt"), -9_007_199_254_740_991)
        XCTAssertEqual(request.int("maxDouble"), 9_007_199_254_740_991)
        XCTAssertNil(request.int("limitInt"))
        XCTAssertNil(request.int("limitNegInt"))
        XCTAssertNil(request.int("limitDouble"))
        XCTAssertNil(request.int("limitNegDouble"))
    }

    func testIntAcceptsIntegralNumbers() throws {
        let request = try Codec.decode(#"{"id":"1","cmd":"search","a":3,"b":2.0,"c":-1,"d":0}"#).get()
        XCTAssertEqual(request.int("a"), 3)
        XCTAssertEqual(request.int("b"), 2)
        XCTAssertEqual(request.int("c"), -1)
        XCTAssertEqual(request.int("d"), 0)
    }

    func testUnencodableSuccessKeepsResponseID() throws {
        let data = Codec.encode(.success(id: "42", result: ["position": Double.nan]))
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(object["id"] as? String, "42")
        XCTAssertEqual(object["ok"] as? Bool, false)
        XCTAssertEqual(object["error"] as? String, "failed to encode response")
        XCTAssertEqual(data.last, UInt8(ascii: "\n"))
        XCTAssertEqual(data.filter { $0 == UInt8(ascii: "\n") }.count, 1)
    }

    func testNonJSONValueFallsBackInsteadOfCrashing() throws {
        let data = Codec.encode(.success(id: "3", result: ["when": Date()]))
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(object["id"] as? String, "3")
        XCTAssertEqual(object["error"] as? String, "failed to encode response")
    }

    func testUnencodableEventBecomesErrorEvent() throws {
        let data = Codec.encode(.event(name: "state", fields: ["state": ["position": Double.infinity]]))
        XCTAssertEqual(String(decoding: data, as: UTF8.self),
                       #"{"event":"error","message":"failed to encode state event"}"# + "\n")
    }
}

private extension Result {
    var failureValue: Failure? {
        if case let .failure(error) = self { return error }
        return nil
    }
}
