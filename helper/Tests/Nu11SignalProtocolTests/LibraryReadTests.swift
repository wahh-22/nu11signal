import Foundation
import XCTest
@testable import Nu11SignalProtocol

/// Fixtures follow the documented shapes of LibraryPlaylistsResponse and
/// LibraryPlaylistsTracksRelationshipResponse
/// (https://developer.apple.com/documentation/applemusicapi).
private enum Fixture {
    static let playlistsPage1 = #"""
    {"next":"/v1/me/library/playlists?offset=2","data":[
      {"id":"p.A1","type":"library-playlists","href":"/v1/me/library/playlists/p.A1",
       "attributes":{"name":"Road Trip","canEdit":true,"isPublic":false,"hasCatalog":false,
                     "playParams":{"id":"p.A1","kind":"playlist","isLibrary":true},
                     "description":{"standard":"Songs for the road"}}},
      {"id":"p.B2","type":"library-playlists","href":"/v1/me/library/playlists/p.B2",
       "attributes":{"name":"Today's Hits","canEdit":false,"hasCatalog":true}}
    ]}
    """#
    static let playlistsPage2 = #"""
    {"data":[{"id":"p.C3","type":"library-playlists","attributes":{"name":"Chill"}}],"meta":{"total":3}}
    """#
    static let tracks = #"""
    {"next":"/v1/me/library/playlists/p.A1/tracks?offset=100","meta":{"total":104},"data":[
      {"id":"i.Song1","type":"library-songs","attributes":{"name":"Akureyri","artistName":"Aitana",
        "albumName":"Cuarto Azul","durationInMillis":201000,"trackNumber":3,
        "playParams":{"id":"i.Song1","kind":"song","isLibrary":true,"reporting":true,"catalogId":"1740944714"}}},
      {"id":"i.Upload","type":"library-songs","attributes":{"name":"Demo Take","artistName":"Me",
        "durationInMillis":95500,"playParams":{"id":"i.Upload","kind":"song","isLibrary":true}}},
      {"id":"i.Video","type":"library-music-videos","attributes":{"name":"A Video","artistName":"X",
        "playParams":{"id":"i.Video","kind":"musicVideo","isLibrary":true,"catalogId":"555"}}},
      {"id":"1440857781","type":"songs","attributes":{"name":"Catalog Song","artistName":"Y",
        "albumName":"Z","durationInMillis":180000,"playParams":{"id":"1440857781","kind":"song"}}},
      {"id":"i.Gone","type":"library-songs","attributes":{"name":"Unavailable","artistName":"W"}}
    ]}
    """#
    static let playlist = #"""
    {"data":[{"id":"p.A1","type":"library-playlists","attributes":{"name":"Road Trip","canEdit":true,
      "description":{"standard":"Songs <b>for</b> the road","short":"Road"}}}]}
    """#
}

final class APIPathIDTests: XCTestCase {
    func testSafeIDsPass() throws {
        for id in ["1740944714", "p.6xZagAVsYGZAdVp", "i.8WdNb6YCMD45Gre", "a.b.c"] {
            XCTAssertEqual(try APIPathID.checked(id), id)
        }
    }

    func testIDsThatCouldChangeThePathAreRejected() {
        for id in ["", "p./x", "../me", "p..x", "a b", "p.x?y=1", "p.x#f", "i.%2F", "ü", "p.x/..", "."] {
            XCTAssertThrowsError(try APIPathID.checked(id), id) { error in
                XCTAssertEqual(String(describing: error), "\"\(id)\" is not a valid Apple Music id")
            }
        }
    }

    func testLibraryEditsValidateIDsBeforeBuildingPaths() {
        XCTAssertThrowsError(try LibraryEdit.songType("i.a/../b"))
        XCTAssertThrowsError(try LibraryEdit.playlistID("p.a/../../catalog"))
        XCTAssertThrowsError(try LibraryEdit.playlistID("p..."))
    }
}

final class LibraryReadTests: XCTestCase {
    private func request(_ cmd: String, _ args: JSONObject) -> Request {
        Request(id: "1", cmd: cmd, args: args)
    }

    // MARK: calls

    func testPlaylistsCallAsksForFullPages() {
        XCTAssertEqual(LibraryRead.playlistsCall, MusicAPICall(
            method: "GET", path: "/v1/me/library/playlists",
            query: [URLQueryItem(name: "limit", value: "100")], body: nil))
    }

    func testPlaylistCallsNeedAnAPILibraryID() throws {
        let req = request("libraryPlaylist", ["playlistId": "p.A1"])
        XCTAssertEqual(try LibraryRead.playlistCall(req).path, "/v1/me/library/playlists/p.A1")
        let tracks = try LibraryRead.tracksCall(req)
        XCTAssertEqual(tracks.path, "/v1/me/library/playlists/p.A1/tracks")
        XCTAssertEqual(tracks.query, [URLQueryItem(name: "limit", value: "100")])
        for id in ["7193945518659293268", "p.a/../x"] {
            XCTAssertThrowsError(try LibraryRead.tracksCall(request("libraryPlaylist", ["playlistId": id])), id)
        }
        XCTAssertThrowsError(try LibraryRead.playlistCall(request("libraryPlaylist", [:])))
    }

    // MARK: pages

    func testPlaylistsPageReadsIDNameAndEditable() throws {
        let page = try LibraryRead.playlistsPage(Data(Fixture.playlistsPage1.utf8), of: LibraryRead.playlistsCall)
        XCTAssertEqual(page.items, [
            LibraryPlaylistSummary(id: "p.A1", name: "Road Trip", editable: true),
            LibraryPlaylistSummary(id: "p.B2", name: "Today's Hits", editable: false),
        ])
        XCTAssertEqual(page.next, MusicAPICall(
            method: "GET", path: "/v1/me/library/playlists",
            query: [URLQueryItem(name: "offset", value: "2")], body: nil))

        let last = try LibraryRead.playlistsPage(Data(Fixture.playlistsPage2.utf8), of: LibraryRead.playlistsCall)
        XCTAssertEqual(last.items, [LibraryPlaylistSummary(id: "p.C3", name: "Chill", editable: false)])
        XCTAssertNil(last.next)
    }

    func testPlaylistsWithUnsafeIDsAreLeftOut() throws {
        let json = #"{"data":[{"id":"p.a/b","attributes":{"name":"x"}},{"id":"p.ok","attributes":{"name":"ok"}},{"attributes":{}}]}"#
        let page = try LibraryRead.playlistsPage(Data(json.utf8), of: LibraryRead.playlistsCall)
        XCTAssertEqual(page.items.map(\.id), ["p.ok"])
    }

    func testUnreadablePagesAreErrors() {
        for bad in ["", "nope", #"{"errors":[]}"#] {
            XCTAssertThrowsError(try LibraryRead.playlistsPage(Data(bad.utf8), of: LibraryRead.playlistsCall), bad) { error in
                XCTAssertEqual(String(describing: error), "the Apple Music API answered with an unreadable page")
            }
        }
    }

    func testTracksMapToCatalogIDsWhenTheyHaveOne() throws {
        let call = try LibraryRead.tracksCall(request("libraryPlaylist", ["playlistId": "p.A1"]))
        let page = try LibraryRead.tracksPage(Data(Fixture.tracks.utf8), of: call)
        XCTAssertEqual(page.items, [
            LibraryTrack(libraryID: "i.Song1", catalogID: "1740944714", title: "Akureyri", artist: "Aitana",
                         album: "Cuarto Azul", duration: 201),
            LibraryTrack(libraryID: "i.Upload", catalogID: nil, title: "Demo Take", artist: "Me",
                         album: "", duration: 95.5),
            LibraryTrack(libraryID: "1440857781", catalogID: "1440857781", title: "Catalog Song", artist: "Y",
                         album: "Z", duration: 180),
            LibraryTrack(libraryID: "i.Gone", catalogID: nil, title: "Unavailable", artist: "W",
                         album: "", duration: 0),
        ])
        XCTAssertEqual(page.items.map(\.id), ["1740944714", "i.Upload", "1440857781", "i.Gone"])
        XCTAssertEqual(page.items.map(\.playable), [true, false, true, false])
        XCTAssertEqual(page.next?.path, "/v1/me/library/playlists/p.A1/tracks")
        XCTAssertEqual(page.next?.query, [URLQueryItem(name: "offset", value: "100")])
    }

    func testTrackJSONMarksLibraryOnlySongs() {
        let only = LibraryTrack(libraryID: "i.U", catalogID: nil, title: "t", artist: "a", album: "", duration: 1)
        XCTAssertEqual(only.json["id"] as? String, "i.U")
        XCTAssertEqual(only.json["libraryOnly"] as? Bool, true)
        let catalog = LibraryTrack(libraryID: "i.S", catalogID: "1", title: "t", artist: "a", album: "b", duration: 2)
        XCTAssertEqual(catalog.json["id"] as? String, "1")
        XCTAssertNil(catalog.json["libraryOnly"])
        XCTAssertEqual(catalog.json["duration"] as? Double, 2)
    }

    func testUnsafeCatalogIDsAreTreatedAsMissing() throws {
        let json = #"{"data":[{"id":"i.S","type":"library-songs","attributes":{"name":"n","playParams":{"catalogId":"1/../2"}}}]}"#
        let call = try LibraryRead.tracksCall(request("libraryPlaylist", ["playlistId": "p.A1"]))
        let page = try LibraryRead.tracksPage(Data(json.utf8), of: call)
        XCTAssertEqual(page.items.first?.catalogID, nil)
    }

    func testPlaylistInfoReadsNameAndDescription() throws {
        let info = try LibraryRead.playlistInfo(Data(Fixture.playlist.utf8))
        XCTAssertEqual(info.name, "Road Trip")
        XCTAssertEqual(info.editable, true)
        XCTAssertEqual(info.notes, "Songs <b>for</b> the road")
        let short = try LibraryRead.playlistInfo(Data(#"{"data":[{"id":"p.x","attributes":{"name":"n","description":{"short":"s"}}}]}"#.utf8))
        XCTAssertEqual(short.notes, "s")
        XCTAssertThrowsError(try LibraryRead.playlistInfo(Data(#"{"data":[]}"#.utf8)))
    }

    // MARK: next links

    func testNextLinksMustStayUnderTheSameResource() {
        let base = LibraryRead.playlistsCall
        for next in [
            "https://evil.example/v1/me/library/playlists?offset=2",
            "//evil.example/v1/me/library/playlists",
            "/v1/catalog/us/songs?offset=2",
            "/v1/me/library/playlists/../../catalog?offset=2",
            "/v1/me/library/playlistsX?offset=2",
            "relative?offset=2",
        ] {
            let json = #"{"next":"\#(next)","data":[{"id":"p.a","attributes":{"name":"a"}}]}"#
            XCTAssertThrowsError(try LibraryRead.playlistsPage(Data(json.utf8), of: base), next) { error in
                XCTAssertEqual(String(describing: error), "the Apple Music API answered with an unexpected next page")
            }
        }
    }

    // MARK: pagination

    func testCollectFollowsNextPagesUpToTheCap() async throws {
        var fetched: [MusicAPICall] = []
        let pages = [Fixture.playlistsPage1, Fixture.playlistsPage2]
        let all = try await LibraryRead.collect(from: LibraryRead.playlistsCall, cap: 10, fetch: { call in
            fetched.append(call)
            return Data(pages[fetched.count - 1].utf8)
        }, page: LibraryRead.playlistsPage)
        XCTAssertEqual(all.map(\.id), ["p.A1", "p.B2", "p.C3"])
        XCTAssertEqual(fetched.count, 2)

        fetched = []
        let capped = try await LibraryRead.collect(from: LibraryRead.playlistsCall, cap: 2, fetch: { call in
            fetched.append(call)
            return Data(pages[fetched.count - 1].utf8)
        }, page: LibraryRead.playlistsPage)
        XCTAssertEqual(capped.map(\.id), ["p.A1", "p.B2"])
        XCTAssertEqual(fetched.count, 1, "no page is fetched past the cap")
    }

    func testCollectStopsAtAnEmptyPageThatStillLinksOn() async throws {
        var count = 0
        let loop = #"{"next":"/v1/me/library/playlists?offset=0","data":[]}"#
        let items = try await LibraryRead.collect(from: LibraryRead.playlistsCall, cap: 500, fetch: { _ in
            count += 1
            return Data(loop.utf8)
        }, page: LibraryRead.playlistsPage)
        XCTAssertEqual(items, [])
        XCTAssertEqual(count, 1)
    }

    func testCollectPassesFetchErrorsOn() async {
        struct Boom: Error {}
        do {
            _ = try await LibraryRead.collect(from: LibraryRead.playlistsCall, cap: 5, fetch: { _ in throw Boom() },
                                              page: LibraryRead.playlistsPage)
            XCTFail("expected an error")
        } catch {
            XCTAssertTrue(error is Boom)
        }
    }

    // MARK: queue plan

    private let tracks = [
        LibraryTrack(libraryID: "i.1", catalogID: "10", title: "One", artist: "", album: "", duration: 0),
        LibraryTrack(libraryID: "i.2", catalogID: nil, title: "Two", artist: "", album: "", duration: 0),
        LibraryTrack(libraryID: "i.3", catalogID: "30", title: "Three", artist: "", album: "", duration: 0),
        LibraryTrack(libraryID: "i.4", catalogID: "10", title: "One again", artist: "", album: "", duration: 0),
    ]

    func testQueuePlanSkipsLibraryOnlySongsAndKeepsTheStartSong() throws {
        XCTAssertEqual(try LibraryQueuePlan(tracks, startIndex: nil), LibraryQueuePlan(ids: ["10", "30", "10"], start: 0))
        XCTAssertEqual(try LibraryQueuePlan(tracks, startIndex: 0).start, 0)
        XCTAssertEqual(try LibraryQueuePlan(tracks, startIndex: 2), LibraryQueuePlan(ids: ["10", "30", "10"], start: 1))
        XCTAssertEqual(try LibraryQueuePlan(tracks, startIndex: 3).start, 2, "a repeated song starts at its own copy")
    }

    func testQueuePlanErrors() {
        XCTAssertThrowsError(try LibraryQueuePlan(tracks, startIndex: 1)) { error in
            XCTAssertEqual(String(describing: error),
                           "\"Two\" is not in the Apple Music catalog, so it cannot be played here")
        }
        XCTAssertThrowsError(try LibraryQueuePlan(tracks, startIndex: 4)) { error in
            XCTAssertEqual(String(describing: error), "startIndex 4 is out of range: the playlist has 4 songs")
        }
        XCTAssertThrowsError(try LibraryQueuePlan([tracks[1]], startIndex: nil)) { error in
            XCTAssertEqual(String(describing: error), "none of the playlist's songs are in the Apple Music catalog")
        }
        XCTAssertThrowsError(try LibraryQueuePlan([], startIndex: nil)) { error in
            XCTAssertEqual(String(describing: error), "the playlist has no songs")
        }
    }

    // MARK: catalog batches

    func testCatalogBatchesSplitLongIDLists() {
        XCTAssertEqual(CatalogBatches.split(["1", "2", "3"], size: 2), [["1", "2"], ["3"]])
        XCTAssertEqual(CatalogBatches.split([], size: 2), [])
        XCTAssertEqual(CatalogBatches.split(Array(repeating: "1", count: 600), size: CatalogBatches.size).map(\.count),
                       [300, 300])
    }
}

final class MusicAPIFailureTests: XCTestCase {
    func testFailureUsesTheDetailOrElseTheTitle() {
        XCTAssertEqual(MusicAPIFailure(command: "addToPlaylist", status: 403, title: "Forbidden", detail: "").description,
                       "playlist is not editable (Forbidden)")
        XCTAssertEqual(MusicAPIFailure(command: "favorite", status: 400, title: "Bad", detail: "Ratings not allowed").description,
                       "Apple Music rejected the request (HTTP 400: Ratings not allowed)")
    }

    func testFavoriteReadsA404AsNotFavorite() throws {
        let love = Data(#"{"data":[{"attributes":{"value":1}}]}"#.utf8)
        XCTAssertTrue(try LibraryEdit.favoriteAnswer(.success(love)))
        let unrated = MusicAPIFailure(command: "favorite", status: 404, title: "Not Found", detail: "")
        XCTAssertFalse(try LibraryEdit.favoriteAnswer(.failure(unrated)))
        let refused = MusicAPIFailure(command: "favorite", status: 403, title: "Forbidden", detail: "")
        XCTAssertThrowsError(try LibraryEdit.favoriteAnswer(.failure(refused)))
    }

    func testUnfavoritingAnUnratedSongSucceeds() {
        let unrated = MusicAPIFailure(command: "setFavorite", status: 404, title: "Not Found", detail: "")
        XCTAssertNoThrow(try LibraryEdit.setFavoriteAnswer(on: false, .failure(unrated)))
        XCTAssertThrowsError(try LibraryEdit.setFavoriteAnswer(on: true, .failure(unrated)))
        XCTAssertNoThrow(try LibraryEdit.setFavoriteAnswer(on: true, .success(Data())))
        let refused = MusicAPIFailure(command: "setFavorite", status: 401, title: "Unauthorized", detail: "")
        XCTAssertThrowsError(try LibraryEdit.setFavoriteAnswer(on: false, .failure(refused)))
    }

    func testTimedOutWritesHaveAnUnknownOutcome() {
        let timeout = Deadline.TimedOut(seconds: 5)
        XCTAssertEqual(String(describing: LibraryEdit.settled("createPlaylist", timeout)),
                       "createPlaylist timed out after 5.0s: the playlist may or may not have been created; "
                       + "check the library before trying again")
        XCTAssertEqual(String(describing: LibraryEdit.settled("addToPlaylist", timeout)),
                       "addToPlaylist timed out after 5.0s: the songs may or may not have been added; "
                       + "check the playlist before trying again")
        XCTAssertEqual(String(describing: LibraryEdit.settled("favorite", timeout)), "timed out after 5.0s")
        struct Other: Error, CustomStringConvertible { var description: String { "other" } }
        XCTAssertEqual(String(describing: LibraryEdit.settled("createPlaylist", Other())), "other")
    }
}
