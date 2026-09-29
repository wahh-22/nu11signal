import Combine
import Foundation
import MusicKit
import Nu11SignalProtocol

/// Publishes `state` events: every 500 ms while playing, and immediately
/// whenever the playback status or the current queue entry changes.
@MainActor
final class StateEmitter {
    private let player = ApplicationMusicPlayer.shared
    private var stateSubscription: AnyCancellable?
    /// Only the current queue is observed; replacing it cancels the old one.
    private var queueSubscription: AnyCancellable?
    private var observedQueue: ApplicationMusicPlayer.Queue?
    private var lastSignature: String?
    private var ticker: Task<Void, Never>?

    func start() {
        stateSubscription = player.state.objectWillChange
            .sink { [weak self] in self?.scheduleCheck() }
        observeQueue()
        ticker = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .milliseconds(500))
                self?.tick()
            }
        }
    }

    /// Emits a state event only if status or current entry changed.
    func checkForChange() {
        observeQueue()
        let snapshot = Snapshot(player: player)
        if snapshot.signature != lastSignature {
            emit(snapshot)
        }
    }

    /// Emits a state event unconditionally (e.g. after a seek, whose new
    /// position the status/entry signature does not capture).
    func emitNow() {
        observeQueue()
        emit(Snapshot(player: player))
    }

    private func tick() {
        let snapshot = Snapshot(player: player)
        if snapshot.status == "playing" || snapshot.signature != lastSignature {
            emit(snapshot)
        }
        observeQueue()
    }

    private func emit(_ snapshot: Snapshot) {
        lastSignature = snapshot.signature
        Output.shared.event("state", ["state": snapshot.json])
    }

    /// objectWillChange fires before the value changes, so read it on the next turn.
    private nonisolated func scheduleCheck() {
        DispatchQueue.main.async {
            MainActor.assumeIsolated { self.checkForChange() }
        }
    }

    /// Assigning a new queue replaces the object, so re-subscribe when it does.
    private func observeQueue() {
        let queue = player.queue
        guard queue !== observedQueue else { return }
        observedQueue = queue
        queueSubscription?.cancel()
        queueSubscription = queue.objectWillChange.sink { [weak self] in self?.scheduleCheck() }
    }
}

/// A point-in-time view of the player in protocol shape.
private struct Snapshot {
    var status: String
    var title = ""
    var artist = ""
    var album = ""
    var songId = ""
    var duration: Double = 0
    var position: Double

    @MainActor
    init(player: ApplicationMusicPlayer) {
        status = Snapshot.name(of: player.state.playbackStatus)
        position = max(0, player.playbackTime)
        guard let entry = player.queue.currentEntry else { return }
        title = entry.title
        artist = entry.subtitle ?? ""
        // A library item's duration may arrive in milliseconds (see
        // LibraryDuration); catalog durations pass through unchanged.
        switch entry.item {
        case let .song(song):
            artist = song.artistName
            album = song.albumTitle ?? ""
            songId = song.id.rawValue
            duration = LibraryDuration.seconds(song.duration)
        case let .musicVideo(video):
            artist = video.artistName
            album = video.albumTitle ?? ""
            songId = video.id.rawValue
            duration = LibraryDuration.seconds(video.duration)
        case .none:
            break
        @unknown default:
            break
        }
    }

    var signature: String { "\(status)|\(songId)|\(title)" }

    var json: JSONObject {
        [
            "status": status, "title": title, "artist": artist, "album": album,
            "songId": songId, "duration": duration, "position": position,
        ]
    }

    static func name(of status: MusicPlayer.PlaybackStatus) -> String {
        switch status {
        case .playing: return "playing"
        case .paused: return "paused"
        case .stopped: return "stopped"
        case .interrupted: return "interrupted"
        case .seekingForward, .seekingBackward: return "seeking"
        @unknown default: return "stopped"
        }
    }
}
