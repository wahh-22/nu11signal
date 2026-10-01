import Combine
import Foundation
import MusicKit
import Nu11SignalProtocol

/// Publishes `state` events: every 500 ms while playing (or seeking), and immediately
/// whenever the playback status, the current queue entry, the repeat mode
/// or the volume mode changes. It also hands each status to AppVolume,
/// which starts and stops its tap with playback, and watches for the
/// queue's last song ending by itself (see `onSegmentEnd`).
@MainActor
final class StateEmitter {
    private let player = ApplicationMusicPlayer.shared
    private var stateSubscription: AnyCancellable?
    /// Only the current queue is observed; replacing it cancels the old one.
    private var queueSubscription: AnyCancellable?
    private var observedQueue: ApplicationMusicPlayer.Queue?
    private var lastSignature: String?
    private var ticker: Task<Void, Never>?

    /// The catalog id of each library copy in the queue, keyed by the
    /// copy's id (see `QueuePlan.catalogIDs`): a state names the
    /// catalog song the UI asked for, not the copy the player holds.
    /// `playSongs` replaces it with each queue it hands the player, and
    /// clears it when that fails or playback stops.
    var catalogIDs: [String: String] = [:]

    /// Called when the queue's last song ended by itself (see SegmentEnd),
    /// so `playSongs` can play the next segment of its list. Each reading
    /// of the player (on every change, and every 500 ms) is compared with
    /// the one before it; none is compared while the watch is suspended.
    var onSegmentEnd: (() -> Void)?
    private var lastObservation: SegmentEnd.Observation?
    private var segmentWatchSuspended = false
    /// Readings before this moment are ignored: the player may still show
    /// the state from before the last command for a moment.
    private var segmentWatchResumes = Date.distantPast
    /// How long after a playback command the watch ignores the player.
    private static let segmentWatchGrace: TimeInterval = 1

    /// Stops the watch while a playback command changes the player, so
    /// the change is never taken for the end of the queue.
    func suspendSegmentWatch() {
        segmentWatchSuspended = true
        lastObservation = nil
    }

    /// Resumes the watch after a playback command, from a fresh reading
    /// taken after `segmentWatchGrace`.
    func resumeSegmentWatch() {
        segmentWatchSuspended = false
        lastObservation = nil
        segmentWatchResumes = Date().addingTimeInterval(Self.segmentWatchGrace)
    }

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

    /// Emits a state event only if status, current entry, repeat mode or
    /// volume mode changed.
    func checkForChange() {
        observeQueue()
        let snapshot = self.snapshot()
        if snapshot.signature != lastSignature {
            emit(snapshot)
        }
        watchSegmentEnd(snapshot)
    }

    /// Emits a state event unconditionally (e.g. after a seek, whose new
    /// position the status/entry signature does not capture).
    func emitNow() {
        observeQueue()
        emit(snapshot())
    }

    /// The player's state now. AppVolume follows its status first: that
    /// may change the volume mode, which is read after it.
    private func snapshot() -> Snapshot {
        let status = Snapshot.name(of: player.state.playbackStatus)
        AppVolume.shared.playbackStatus(status)
        return Snapshot(player: player, status: status, catalogIDs: catalogIDs, volumeMode: AppVolume.shared.mode)
    }

    private func tick() {
        let snapshot = self.snapshot()
        if PlaybackActivity.isPlaying(snapshot.status) || snapshot.signature != lastSignature {
            emit(snapshot)
        }
        observeQueue()
        watchSegmentEnd(snapshot)
    }

    /// Compares this reading of the player with the last one and calls
    /// `onSegmentEnd` once when the queue's last song ended by itself.
    private func watchSegmentEnd(_ snapshot: Snapshot) {
        guard !segmentWatchSuspended, Date() >= segmentWatchResumes else { return }
        let entries = player.queue.entries
        let current = player.queue.currentEntry
        let now = SegmentEnd.Observation(
            entry: current.flatMap { current in entries.firstIndex { $0.id == current.id } },
            entryCount: entries.count, playing: snapshot.status == "playing",
            position: snapshot.position, duration: snapshot.duration)
        defer { lastObservation = now }
        guard let before = lastObservation,
              SegmentEnd.ended(before: before, now: now, repeatMode: snapshot.repeatMode)
        else { return }
        onSegmentEnd?()
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
struct Snapshot {
    var status: String
    var title = ""
    var artist = ""
    var album = ""
    var songId = ""
    var duration: Double = 0
    var position: Double
    /// "off", "all" or "one" (see RepeatSetting); a player without a mode
    /// reports "off".
    var repeatMode: String
    /// "app" or "system": which volume `volume` and `setVolume` drive.
    var volumeMode: String

    @MainActor
    init(player: ApplicationMusicPlayer, status: String, catalogIDs: [String: String], volumeMode: VolumeMode) {
        self.volumeMode = volumeMode.rawValue
        self.status = status
        position = max(0, player.playbackTime)
        repeatMode = Snapshot.name(of: player.state.repeatMode)
        guard let entry = player.queue.currentEntry else { return }
        title = entry.title
        artist = entry.subtitle ?? ""
        // A library item's duration may arrive in milliseconds (see
        // LibraryDuration); catalog durations pass through unchanged.
        switch entry.item {
        case let .song(song):
            artist = song.artistName
            album = song.albumTitle ?? ""
            songId = catalogIDs[song.id.rawValue] ?? song.id.rawValue
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

    var signature: String { "\(status)|\(songId)|\(title)|\(repeatMode)|\(volumeMode)" }

    var json: JSONObject {
        [
            "status": status, "title": title, "artist": artist, "album": album,
            "songId": songId, "duration": duration, "position": position, "repeat": repeatMode,
            "volumeMode": volumeMode,
        ]
    }

    static func name(of mode: MusicPlayer.RepeatMode?) -> String {
        switch mode {
        case .all: return "all"
        case .one: return "one"
        case .some(.none), nil: return "off"
        @unknown default: return "off"
        }
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
