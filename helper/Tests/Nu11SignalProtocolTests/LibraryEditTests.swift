import Foundation
import XCTest
@testable import Nu11SignalProtocol

final class LibraryEditTests: XCTestCase {
    private func request(_ cmd: String, _ args: JSONObject) -> Request {
        Request(id: "1", cmd: cmd, args: args)
    }

    private func body(_ call: MusicAPICall) -> String? {
        call.body.map { String(decoding: $0, as: UTF8.self) }
    }

    private func assertThrows(_ message: String, file: StaticString = #filePath, line: UInt = #line, _ run: () throws -> Any) {
        XCTAssertThrowsError(try run(), file: file, line: line) { error in
            XCTAssertEqual(String(describing: error), message, file: file, line: line)
        }
    }

    // MARK: song and playlist ids

    func testSongTypeFollowsTheIDKind() throws {
        XCTAssertEqual(try LibraryEdit.songType("1740944714"), "songs")
        XCTAssertEqual(try LibraryEdit.songType("i.8WdNb6YCMD45Gre"), "library-songs")
    }

    func testLocalLibraryIDsAreRejected() {
        // MusicKit on macOS names library items by 64-bit persistent ids,
        // which the Apple Music API does not know.
        for id in ["34807486897551531", "i.", "abc"] {
            assertThrows("song \"\(id)\" has no Apple Music API id") {
                try LibraryEdit.songType(id)
            }
        }
        for id in ["-6576179985726945418", ""] {
            assertThrows("\"\(id)\" is not a valid Apple Music id") {
                try LibraryEdit.songType(id)
            }
        }
    }

    func testPlaylistIDMustBeAnAPILibraryID() throws {
        XCTAssertEqual(try LibraryEdit.playlistID("p.6xZagAVsYGZAdVp"), "p.6xZagAVsYGZAdVp")
        for id in ["7193945518659293268", "pl.u-123", "p."] {
            assertThrows("playlist \"\(id)\" has no Apple Music API id") {
                try LibraryEdit.playlistID(id)
            }
        }
    }

    // MARK: createPlaylist

    func testCreatePlaylistWithSongsAndDescription() throws {
        let call = try LibraryEdit.createPlaylist(request("createPlaylist", [
            "name": "Night Drive", "description": "After hours", "songIds": ["1740944714", "i.8WdNb6YCMD45Gre"],
        ]))
        XCTAssertEqual(call.method, "POST")
        XCTAssertEqual(call.path, "/v1/me/library/playlists")
        XCTAssertEqual(body(call), #"{"attributes":{"description":"After hours","name":"Night Drive"},"#
            + #""relationships":{"tracks":{"data":[{"id":"1740944714","type":"songs"},{"id":"i.8WdNb6YCMD45Gre","type":"library-songs"}]}}}"#)
    }

    func testCreatePlaylistWithoutSongsOrDescription() throws {
        for args: JSONObject in [["name": "Empty"], ["name": "Empty", "songIds": [String](), "description": ""]] {
            let call = try LibraryEdit.createPlaylist(request("createPlaylist", args))
            XCTAssertEqual(body(call), #"{"attributes":{"name":"Empty"}}"#)
        }
    }

    func testCreatePlaylistArgumentErrors() {
        assertThrows(#"createPlaylist requires a non-empty "name""#) {
            try LibraryEdit.createPlaylist(self.request("createPlaylist", ["name": "  "]))
        }
        assertThrows(#""songIds" must be an array of strings"#) {
            try LibraryEdit.createPlaylist(self.request("createPlaylist", ["name": "x", "songIds": "1"]))
        }
        assertThrows(#""description" must be a string"#) {
            try LibraryEdit.createPlaylist(self.request("createPlaylist", ["name": "x", "description": 3]))
        }
        assertThrows("song \"123456789012345678\" has no Apple Music API id") {
            try LibraryEdit.createPlaylist(self.request("createPlaylist", ["name": "x", "songIds": ["123456789012345678"]]))
        }
    }

    func testCreatedPlaylistIsReadFromTheResponse() throws {
        let json = #"{"data":[{"id":"p.new1","type":"library-playlists","attributes":{"name":"Night Drive","canEdit":true}}]}"#
        XCTAssertEqual(try LibraryEdit.createdPlaylist(Data(json.utf8), requestedName: "x"),
                       CreatedPlaylist(id: "p.new1", name: "Night Drive"))
        let unnamed = #"{"data":[{"id":"p.new2","type":"library-playlists"}]}"#
        XCTAssertEqual(try LibraryEdit.createdPlaylist(Data(unnamed.utf8), requestedName: "Asked"),
                       CreatedPlaylist(id: "p.new2", name: "Asked"))
        for bad in [#"{"data":[]}"#, #"{"data":[{"type":"library-playlists"}]}"#, "", "nope"] {
            assertThrows("the Apple Music API did not return the new playlist") {
                try LibraryEdit.createdPlaylist(Data(bad.utf8), requestedName: "x")
            }
        }
    }

    // MARK: addToPlaylist

    func testAddToPlaylist() throws {
        let call = try LibraryEdit.addToPlaylist(request("addToPlaylist", [
            "playlistId": "p.6xZagAVsYGZAdVp", "songIds": ["i.a", "1740944714"],
        ]))
        XCTAssertEqual(call.method, "POST")
        XCTAssertEqual(call.path, "/v1/me/library/playlists/p.6xZagAVsYGZAdVp/tracks")
        XCTAssertEqual(body(call), #"{"data":[{"id":"i.a","type":"library-songs"},{"id":"1740944714","type":"songs"}]}"#)
    }

    func testAddToPlaylistArgumentErrors() {
        assertThrows(#"addToPlaylist requires a non-empty "playlistId""#) {
            try LibraryEdit.addToPlaylist(self.request("addToPlaylist", ["songIds": ["1"]]))
        }
        assertThrows(#"addToPlaylist requires a non-empty "songIds" array"#) {
            try LibraryEdit.addToPlaylist(self.request("addToPlaylist", ["playlistId": "p.a", "songIds": [String]()]))
        }
        assertThrows(#"addToPlaylist requires a non-empty "songIds" array"#) {
            try LibraryEdit.addToPlaylist(self.request("addToPlaylist", ["playlistId": "p.a"]))
        }
    }

    // MARK: favorite and setFavorite

    func testFavoriteReadsTheSongRating() throws {
        let catalog = try LibraryEdit.favorite(request("favorite", ["songId": "1740944714"]))
        XCTAssertEqual(catalog, MusicAPICall(method: "GET", path: "/v1/me/ratings/songs/1740944714", body: nil))
        let library = try LibraryEdit.favorite(request("favorite", ["songId": "i.a"]))
        XCTAssertEqual(library.path, "/v1/me/ratings/library-songs/i.a")
        assertThrows(#"favorite requires a non-empty "songId""#) {
            try LibraryEdit.favorite(self.request("favorite", [:]))
        }
    }

    func testSetFavoriteLovesOrClearsTheRating() throws {
        let on = try LibraryEdit.setFavorite(request("setFavorite", ["songId": "1740944714", "on": true]))
        XCTAssertEqual(on.method, "PUT")
        XCTAssertEqual(on.path, "/v1/me/ratings/songs/1740944714")
        XCTAssertEqual(body(on), #"{"attributes":{"value":1},"type":"rating"}"#)

        let off = try LibraryEdit.setFavorite(request("setFavorite", ["songId": "i.a", "on": false]))
        XCTAssertEqual(off, MusicAPICall(method: "DELETE", path: "/v1/me/ratings/library-songs/i.a", body: nil))
    }

    func testSetFavoriteRequiresABoolean() {
        for value: Any? in [nil, 1, "true"] {
            var args: JSONObject = ["songId": "1"]
            args["on"] = value
            assertThrows(#"setFavorite requires a boolean "on""#) {
                try LibraryEdit.setFavorite(self.request("setFavorite", args))
            }
        }
    }

    func testIsFavoriteOnlyForALoveRating() {
        XCTAssertTrue(LibraryEdit.isFavorite(Data(#"{"data":[{"id":"1","type":"ratings","attributes":{"value":1}}]}"#.utf8)))
        for json in [
            #"{"data":[{"attributes":{"value":-1}}]}"#,
            #"{"data":[{"attributes":{}}]}"#,
            #"{"data":[]}"#,
            "",
        ] {
            XCTAssertFalse(LibraryEdit.isFavorite(Data(json.utf8)), json)
        }
    }

    func testLibraryEditsDoNotWaitBehindPlayback() {
        for cmd in ["createPlaylist", "addToPlaylist", "favorite", "setFavorite"] {
            XCTAssertFalse(Request(id: "1", cmd: cmd).mutatesPlayback, cmd)
        }
    }

    // MARK: failures

    func testFailureMessagesNameTheCause() {
        let cases: [(String, Int, String, String)] = [
            ("addToPlaylist", 403, "Forbidden", "playlist is not editable (Forbidden)"),
            ("createPlaylist", 403, "", "Apple Music refused the request: check the subscription and that the app may access your library (HTTP 403)"),
            ("setFavorite", 401, "Unauthorized", "Apple Music did not accept the credentials: check the subscription and sign-in (Unauthorized)"),
            ("addToPlaylist", 404, "Resource with requested id was not found", "not found in the Apple Music library (Resource with requested id was not found)"),
            ("setFavorite", 400, "Ratings not allowed", "Apple Music rejected the request (HTTP 400: Ratings not allowed)"),
            ("createPlaylist", 500, "", "Apple Music failed (HTTP 500)"),
        ]
        for (command, status, detail, want) in cases {
            XCTAssertEqual(LibraryEdit.failureMessage(command: command, status: status, detail: detail), want, "\(command) \(status)")
        }
    }
}

final class RequestBoolTests: XCTestCase {
    func testBoolAcceptsOnlyJSONBooleans() throws {
        let decoded = try Codec.decode(#"{"id":"1","cmd":"x","t":true,"f":false,"n":1,"s":"true"}"#).get()
        XCTAssertEqual(decoded.bool("t"), true)
        XCTAssertEqual(decoded.bool("f"), false)
        XCTAssertNil(decoded.bool("n"))
        XCTAssertNil(decoded.bool("s"))
        XCTAssertNil(decoded.bool("missing"))
    }
}
