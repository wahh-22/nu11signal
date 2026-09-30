import Foundation
import Nu11SignalProtocol

/// Emits `levels` events (the spectrum the TUI's equalizer draws) while the
/// app-volume tap renders the music: AppVolume's IOProc writes each buffer,
/// before its gain, into `ring`; a timer on a utility queue reads the newest
/// window LevelsEvent.interval apart, analyzes it and emits the bands.
/// Measuring before the gain keeps the bars independent of the volume.
///
/// AppVolume starts the timer with its IOProc (so only while playing in app
/// mode) and stops it with the IOProc: on pause, stop, rebuild or fallback.
/// Nothing is emitted in system mode. SampleRing's atomics need macOS 15;
/// on macOS 14 there is no meter and the TUI keeps its decorative bars.
@available(macOS 15.0, *)
final class LevelMeter: @unchecked Sendable {
    let ring = SampleRing(capacity: 8192)
    private let queue = DispatchQueue(label: "nu11signal-helper.levels", qos: .utility)
    /// Touched on the main actor only.
    private var timer: DispatchSourceTimer?

    // Touched on `queue` only.
    private var analyzer: SpectrumAnalyzer?
    private var analyzerRate = 0.0
    private var smoother = LevelSmoother(count: LevelsEvent.bands)
    private let window = UnsafeMutablePointer<Float>.allocate(capacity: LevelsEvent.fftSize)

    /// The ring as the IOProc captures it: a raw pointer, so the real-time
    /// thread does no reference counting. `self` outlives every IOProc.
    var ringPointer: UnsafeMutableRawPointer { Unmanaged.passUnretained(ring).toOpaque() }

    init() {
        window.initialize(repeating: 0, count: LevelsEvent.fftSize)
    }

    /// Starts measuring audio at sampleRate; call after the IOProc starts.
    @MainActor
    func start(sampleRate: Double) {
        stop()
        queue.async { [self] in
            if analyzer == nil || sampleRate != analyzerRate {
                analyzer = SpectrumAnalyzer(sampleRate: sampleRate)
                analyzerRate = sampleRate
            }
            smoother.reset()
        }
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.setEventHandler { [self] in measure() }
        // The leeway lets the system coalesce these wakeups with others.
        timer.schedule(deadline: .now() + LevelsEvent.interval, repeating: LevelsEvent.interval, leeway: .milliseconds(10))
        timer.resume()
        self.timer = timer
    }

    /// Stops measuring; a measurement already running may still emit.
    @MainActor
    func stop() {
        timer?.cancel()
        timer = nil
    }

    private func measure() {
        guard let analyzer, ring.readLatest(into: window, count: LevelsEvent.fftSize) else { return }
        let levels = smoother.update(analyzer.levels(window))
        Output.shared.event(LevelsEvent.name, ["bands": LevelScale.quantized(levels)])
    }
}
