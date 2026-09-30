// Spike (odd/tasks/local-queue.md Q2): which queue construction lets the
// player prepare "Canciones favoritas" with every song, and advance with NEXT?
//
// usage: nu11signal-helper list
//        nu11signal-helper <case> <startIndex>
// cases: a  (library Songs of the library playlist, Queue(for:startingAt:))
//        ae (same songs, one Entry each, start named as its entry)
//        b  (the library playlist itself, Queue(playlist:startingAt:))
//        c  (catalog songs, library copy for songs with one, Entry each)
//        d  (current helper: PreparedQueue replica on catalog songs)
//        e  (all catalog songs, nothing dropped: the original Code=6 case)
// Playback is brief: ~1.5 s after play, then paused; skips run paused.
import Foundation
import MusicKit

let playlistName = "Canciones favoritas"
let player = ApplicationMusicPlayer.shared

func out(_ s: String) { print(s); fflush(stdout) }

func errText(_ error: Error) -> String {
    let e = error as NSError
    return "\(e.domain) Code=\(e.code) \(e.userInfo[NSDebugDescriptionErrorKey] as? String ?? e.localizedDescription)"
}

func catalogID(_ song: Song) -> String? {
    guard let p = song.playParameters, let data = try? JSONEncoder().encode(p),
          let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return nil }
    return obj["catalogId"] as? String
}

func paramsJSON(_ song: Song) -> String {
    guard let p = song.playParameters, let data = try? JSONEncoder().encode(p) else { return "nil" }
    return String(decoding: data, as: UTF8.self)
}

struct Loaded {
    let playlist: Playlist
    let tracks: [Track]
    let librarySongs: [Song]          // playlist order
    let catalogIDs: [String]          // playlist order
    let catalog: [String: Song]       // catalog id -> catalog song
    let copies: [String: Song]        // catalog id -> MusicLibraryRequest<Song> hit
}

func load() async throws -> Loaded {
    let all = try await MusicLibraryRequest<Playlist>().response().items
    guard let found = all.first(where: { $0.name == playlistName || $0.name == "Favourite Songs" }) else {
        out("library playlists: \(all.map(\.name))")
        throw NSError(domain: "spike", code: 1)
    }
    let playlist = try await found.with([.tracks, .entries])
    let tracks = Array(playlist.tracks ?? [])
    let songs: [Song] = tracks.compactMap { if case let .song(s) = $0 { return s }; return nil }
    out("device library playlist \"\(playlist.name)\" songs: \(songs.map(\.title)) params0=\(songs.first.map(paramsJSON) ?? "-")")
    // The catalog ids the TUI sends for the Apple Music API playlist
    // p.81UMD45Gre (helper libraryPlaylist, 2026-09-30), in its order.
    let ids = ["1825270819", "1617910506", "1458486562", "1808547032", "1104441831", "1825270816"]
    let lookup = MusicCatalogResourceRequest<Song>(matching: \.id, memberOf: ids.map { MusicItemID($0) })
    var catalog: [String: Song] = [:]
    for s in try await lookup.response().items { catalog[s.id.rawValue] = s }
    // Same lookup as the helper's libraryCopies.
    var lr = MusicLibraryRequest<Song>()
    lr.filter(matching: \.id, memberOf: ids.map { MusicItemID($0) })
    var copies: [String: Song] = [:]
    for s in try await lr.response().items {
        out("  library hit \"\(s.title)\" id=\(s.id) params=\(paramsJSON(s))")
        if let c = catalogID(s) { copies[c] = copies[c] ?? s }
    }
    return Loaded(playlist: playlist, tracks: tracks, librarySongs: songs, catalogIDs: ids, catalog: catalog, copies: copies)
}

func report(_ label: String) {
    let q = player.queue
    let cur = q.currentEntry?.title ?? "nil"
    let idx = q.entries.firstIndex { $0.id == q.currentEntry?.id }.map(String.init) ?? "?"
    out("  [\(label)] status=\(player.state.playbackStatus) current=\"\(cur)\" index=\(idx)/\(q.entries.count) entries=\(q.entries.map(\.title))")
}

func run(_ queue: ApplicationMusicPlayer.Queue, lastTitle: String, append: [MusicPlayer.Queue.Entry] = []) async {
    let t0 = Date()
    player.queue = queue
    do { try await player.prepareToPlay(); out("  prepareToPlay: OK") } catch { out("  prepareToPlay: THROWS \(errText(error))") }
    do { try await player.play(); out("  play: OK") } catch { out("  play: THROWS \(errText(error))") }
    if !append.isEmpty {
        do { try await player.queue.insert(append, position: .tail); out("  insert tail: OK") } catch { out("  insert tail: THROWS \(errText(error))") }
    }
    try? await Task.sleep(for: .milliseconds(1500))
    report("after play")
    player.pause()
    for step in 1...6 {
        do { try await player.skipToNextEntry() } catch { out("  skip \(step): THROWS \(errText(error))"); break }
        try? await Task.sleep(for: .milliseconds(500))
        report("skip \(step)")
        if player.queue.currentEntry?.title == lastTitle { out("  reached last after \(step) skips"); break }
    }
    player.pause()
    player.stop()
    out("  case seconds (wall, mostly paused): \(String(format: "%.1f", Date().timeIntervalSince(t0)))")
}

@main
struct Spike {
    static func main() async {
        let args = CommandLine.arguments
        guard await MusicAuthorization.request() == .authorized else { out("not authorized"); exit(1) }
        do {
            let l = try await load()
            let n = l.catalogIDs.count
            for (i, c) in l.catalogIDs.enumerated() {
                out("  \(i) \(c) \"\(l.catalog[c]?.title ?? "NOT FOUND")\" libraryCopy=\(l.copies[c].map { "\($0.id)" } ?? "none")")
            }
            guard args.count >= 3, let start = Int(args[2]) else { exit(0) }
            let lastTitle = (args[1] == "a" || args[1] == "ae" || args[1] == "b") ? l.librarySongs.last!.title : l.catalog[l.catalogIDs[n - 1]]!.title
            out("case \(args[1]) start=\(start) lastTitle=\"\(lastTitle)\"")
            let queue: ApplicationMusicPlayer.Queue
            var append: [MusicPlayer.Queue.Entry] = []
            switch args[1] {
            case "f", "g":
                // Start item alone (library copy when local), then append the
                // rest (each copy-or-catalog) at the tail once it plays.
                let order = args.count > 3 ? args[3].split(separator: ",").map { Int($0)! } : Array(0..<n)
                let songs = order.map { l.copies[l.catalogIDs[$0]] ?? l.catalog[l.catalogIDs[$0]]! }
                out("  start alone: \(songs[start].title); append: \(songs[(start + 1)...].map(\.title))")
                // g: the start is always its catalog song, even when local.
                let first = MusicPlayer.Queue.Entry(args[1] == "g" ? l.catalog[l.catalogIDs[order[start]]]! : songs[start])
                out("  start item: \(args[1] == "g" ? "catalog" : "copy-or-catalog")")
                queue = ApplicationMusicPlayer.Queue([first], startingAt: first)
                append = songs[(start + 1)...].map { MusicPlayer.Queue.Entry($0) }
            case "a":
                queue = ApplicationMusicPlayer.Queue(for: l.librarySongs, startingAt: l.librarySongs[start])
            case "ae":
                let e = l.librarySongs.map { MusicPlayer.Queue.Entry($0) }
                queue = ApplicationMusicPlayer.Queue(e, startingAt: e[start])
            case "b":
                let entries = Array(l.playlist.entries ?? [])
                out("  playlist entries: \(entries.count)")
                queue = ApplicationMusicPlayer.Queue(playlist: l.playlist, startingAt: entries[start])
            case "c":
                // Optional 3rd argument: a comma-separated order of playlist
                // indices (e.g. 1,0,2,3,4,5), to place the catalog song elsewhere.
                let order = args.count > 3 ? args[3].split(separator: ",").map { Int($0)! } : Array(0..<n)
                let songs = order.map { l.copies[l.catalogIDs[$0]] ?? l.catalog[l.catalogIDs[$0]]! }
                out("  items: \(songs.map { "\($0.title)=\(l.copies[catalogID($0) ?? $0.id.rawValue] != nil && catalogID($0) != nil ? "library" : "catalog")" })")
                let e = songs.map { MusicPlayer.Queue.Entry($0) }
                queue = ApplicationMusicPlayer.Queue(e, startingAt: e[start])
            case "d", "e":
                var songs = l.catalogIDs.map { l.catalog[$0]! }
                var s = start
                if args[1] == "d" {
                    if l.copies[l.catalogIDs[start]] != nil {
                        songs = l.catalogIDs.map { l.copies[$0] ?? l.catalog[$0]! }
                    } else {
                        var kept: [Song] = []
                        for (i, song) in songs.enumerated() {
                            if i == start { s = kept.count } else if l.copies[l.catalogIDs[i]] != nil { continue }
                            kept.append(song)
                        }
                        songs = kept
                    }
                }
                out("  items: \(songs.map(\.title)) start=\(s)")
                let e = songs.map { MusicPlayer.Queue.Entry($0) }
                queue = ApplicationMusicPlayer.Queue(e, startingAt: e[s])
            default:
                out("unknown case"); exit(2)
            }
            await run(queue, lastTitle: lastTitle, append: append)
        } catch {
            out("error: \(errText(error))")
        }
        exit(0)
    }
}
