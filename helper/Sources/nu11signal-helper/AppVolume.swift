import AudioToolbox
import CoreAudio
import Darwin
import Foundation
import Nu11SignalProtocol

/// The volume the `volume` and `setVolume` commands drive: the helper's
/// own gain (app mode) or, as a fallback, the system volume.
///
/// MusicKit renders the helper's audio in a separate process, a per-client
/// copy of MediaPlayer's RemotePlayerService, so the helper cannot scale
/// its own output. In app mode it creates a Core Audio process tap on that
/// process with `muteBehavior = .muted` (the process's own output goes
/// silent), routes the tap into a private aggregate device on the default
/// output device, and renders it there scaled by the app gain. Only a copy
/// macOS holds the helper responsible for is tapped (RemotePlayerTarget):
/// muting another client's copy would silence that client.
///
/// App mode needs the helper to be its own responsible process (see
/// Relaunch), macOS 14.2+ and the audio capture permission; the permission
/// is asked for on the first playback, and until it is granted (or when it
/// is denied) the system volume is used. Nothing muted is ever created
/// before the permission is known to be granted.
///
/// When the tap happens is TapLifecycle's decision; this class performs
/// its actions on the HAL. The tap is built right before a play (so the
/// audio never starts loud), its IOProc stops when playback pauses (the
/// muted tap stays, so resuming is never briefly loud) and everything is
/// destroyed when playback stops and on exit (where a rendering tap first
/// fades the gain to 0, see ShutdownPlan). A rebuild (a new default
/// output device, or a new RemotePlayerService copy) builds the new tap
/// first and destroys the old one once the new one starts, so a failed
/// rebuild keeps the old muted tap, its IOProc stopped, while a transient
/// failure is retried a few times over about five seconds. When no kept
/// tap mutes the player and the app level is below the system volume, the
/// player is paused for the retries (`pausePlayer`) and resumed once a new
/// tap is built (`resumePlayer`). If it keeps failing, or cannot work, the
/// rest of the session uses the system volume. That fallback never changes
/// the system volume: if the music was playing quieter than the system
/// volume, the player is paused first, so it is never suddenly louder; it
/// resumes at the system volume when the user plays again.
///
/// While the IOProc runs it also hands the tapped audio, before the gain,
/// to the LevelMeter (macOS 15+), which emits the spectrum as `levels`.
@MainActor
final class AppVolume {
    static let shared = AppVolume()

    /// Called whenever `mode` changes (the state emitter reports it).
    var onModeChange: (() -> Void)?
    /// Called when the music would otherwise play louder than the app
    /// level (a fallback, or a retry no muted tap covers): the owner
    /// pauses the player.
    var pausePlayer: (() -> Void)?
    /// Called when a new tap is built for a player `pausePlayer` paused
    /// during a retry: the owner resumes it.
    var resumePlayer: (() -> Void)?

    private(set) var mode: VolumeMode = .system
    private let policy: VolumeModePolicy
    private var permission: CapturePermission
    private var requesting = false
    /// Set once the lifecycle fell back: the rest of the session uses the
    /// system volume.
    private var fallbackReason: String?

    /// The app level, 0...1, as reported and stored.
    private var level: Double
    /// The amplitude the IOProc ramps to; written here, read on the IO thread.
    private let target = UnsafeMutablePointer<Float>.allocate(capacity: 1)
    /// Non-zero once the helper exits: the IOProc ramps at the slow
    /// shutdown rate (GainRamp.shutdownSeconds). Written here, read on the
    /// IO thread.
    private let fading = UnsafeMutablePointer<Float>.allocate(capacity: 1)
    /// Set when the quiet exit begins: the lifecycle and the level are
    /// left alone from then on, so nothing raises the gain or rebuilds.
    private var closing = false
    /// The tap in use.
    private var tap: Tap?
    /// The tap a rebuild replaced, kept (muted, its IOProc stopped) until
    /// the new one starts.
    private var replaced: Tap?
    /// The IOProc of `tap` runs.
    private var running = false
    private var lifecycle = TapLifecycle()
    /// The last player status seen; the lifecycle hears only changes.
    private var lastStatus = "stopped"
    /// Bumped to cancel a scheduled retry.
    private var retryGeneration = 0
    private var listening = false
    /// The LevelMeter (macOS 15+) that turns the rendered audio into
    /// `levels` events while the IOProc runs; nil before macOS 15.
    private let meter: AnyObject?

    private init() {
        let environment = ProcessInfo.processInfo.environment
        var supported = false
        if #available(macOS 14.2, *) { supported = true }
        policy = VolumeModePolicy(disclaimed: Relaunch.isDisclaimed, tapsSupported: supported,
                                  forcedSystem: HelperLaunch.forcesSystemVolume(environment: environment))
        permission = policy.mode(.authorized) == .app ? CaptureAuthorization.preflight() : .unavailable
        level = AppGain.stored(UserDefaults.standard.object(forKey: AppGain.defaultsKey))
        target.initialize(to: AppGain.amplitude(level: level))
        fading.initialize(to: 0)
        if #available(macOS 15.0, *) { meter = LevelMeter() } else { meter = nil }
        updateMode()
        log("volume mode \(mode.rawValue) (responsible for itself: \(policy.disclaimed), capture permission: \(permission))")
    }

    // MARK: Commands

    /// The level the `volume` command reports, with its mode.
    func current() throws -> (level: Double, mode: VolumeMode) {
        switch mode {
        case .app: return (level, .app)
        case .system: return (try SystemVolume.level(), .system)
        }
    }

    /// Sets the level of the current mode; app levels persist.
    func set(_ requested: Float32) throws {
        switch mode {
        case .app:
            level = VolumeLevel.reported(requested)
            UserDefaults.standard.set(level, forKey: AppGain.defaultsKey)
            guard !closing else { return } // the exit's fade keeps the gain at 0
            target.pointee = AppGain.amplitude(level: level)
            _ = lifecycle.handle(.gain(quieterThanSystem: level < 1))
        case .system:
            try SystemVolume.setLevel(requested)
        }
    }

    // MARK: Playback lifecycle

    /// Called right before the helper asks the player to play: asks for
    /// the permission the first time (without waiting for the answer: the
    /// music plays at the system volume meanwhile), and in app mode has the
    /// muted tap ready when the audio starts, so it never starts loud. The
    /// IOProc starts once the player reports it plays.
    func prepareForPlayback() {
        if policy.shouldRequest(permission), !requesting {
            requesting = CaptureAuthorization.request { granted in
                DispatchQueue.main.async {
                    MainActor.assumeIsolated { AppVolume.shared.permissionAnswered(granted) }
                }
            }
            if !requesting {
                permission = .unavailable
            }
        }
        if mode == .app { send(.prepare) }
    }

    /// Called when the play prepareForPlayback preceded threw: a tap built
    /// for it must not stay behind, muted.
    func playbackFailed() {
        if mode == .app { send(.playFailed) }
    }

    /// The player's status, as the state emitter reads it (repeatedly).
    func playbackStatus(_ name: String) {
        guard name != lastStatus else { return }
        lastStatus = name
        if mode == .app { send(.playback(PlaybackActivity(status: name))) }
    }

    /// Begins the quiet exit: from now on the lifecycle is no longer fed,
    /// so nothing rebuilds, resumes or raises the gain. Returns the steps
    /// for what plays now (see ShutdownPlan); `playing` is the player's.
    func beginShutdown(playing: Bool) -> [ShutdownPlan.Step] {
        closing = true
        retryGeneration += 1
        return ShutdownPlan.steps(rendering: mode == .app && running, playing: playing)
    }

    /// Ramps the gain to 0 at the shutdown rate. The IOProc reads the two
    /// words separately: if it sees the new target before the slower rate,
    /// one buffer ramps at the level-change rate, which is click-free too.
    func fadeOut() {
        fading.pointee = 1
        target.pointee = 0
    }

    /// Releases the tap and aggregate device; called on exit. The IOProc
    /// stops before the tap, muted to the last, is destroyed.
    func shutdown() {
        closing = true
        teardown()
    }

    private func permissionAnswered(_ granted: Bool) {
        requesting = false
        guard !closing else { return }
        permission = granted ? .authorized : .denied
        log("audio capture permission \(granted ? "granted" : "denied")")
        updateMode()
    }

    /// Follows the policy (or a fallback). Entering app mode starts a fresh
    /// lifecycle told what the player is doing; leaving it releases the tap.
    private func updateMode() {
        let next: VolumeMode = fallbackReason != nil ? .system : policy.mode(permission)
        guard next != mode else { return }
        mode = next
        lifecycle = TapLifecycle()
        retryGeneration += 1
        teardown()
        if mode == .app {
            target.pointee = AppGain.amplitude(level: level)
            _ = lifecycle.handle(.gain(quieterThanSystem: level < 1))
            listen()
            let activity = PlaybackActivity(status: lastStatus)
            if activity != .stopped { send(.playback(activity)) }
        }
        onModeChange?()
    }

    // MARK: Lifecycle actions

    private func send(_ event: TapLifecycle.Event) {
        guard !closing else { return }
        perform(lifecycle.handle(event))
    }

    /// Performs the lifecycle's actions in order; results go back to it
    /// as events (their own actions run before the next one here).
    private func perform(_ actions: [TapLifecycle.Action]) {
        for action in actions {
            // A mode change (a revoked permission, a fallback) ends the
            // lifecycle that asked; only releasing still makes sense.
            guard mode == .app || action == .teardown else { return }
            switch action {
            case .build:
                switch build() {
                case .built: send(.built)
                case .noTarget: send(.noTarget)
                case let .failed(failure): send(.failed(failure))
                case .aborted: return
                }
            case .start:
                if let failure = start() { send(.failed(failure)) } else { send(.started) }
            case .stop:
                stop()
            case .releaseReplaced:
                if let replaced { destroy(replaced) }
                replaced = nil
            case .teardown:
                teardown()
            case let .scheduleRetry(delay):
                scheduleRetry(after: delay)
            case .pausePlayer:
                log("app volume: pausing while no muted tap covers the player, so it does not play louder")
                pausePlayer?()
            case .resumePlayer:
                log("app volume: resuming the player")
                resumePlayer?()
            case let .fallBack(reason):
                fallBack(reason)
            }
        }
    }

    private func scheduleRetry(after delay: Double) {
        retryGeneration += 1
        let generation = retryGeneration
        log("app volume: retrying in \(delay) s")
        DispatchQueue.main.asyncAfter(deadline: .now() + delay) {
            MainActor.assumeIsolated { AppVolume.shared.retryDue(generation) }
        }
    }

    private func retryDue(_ generation: Int) {
        guard generation == retryGeneration, mode == .app else { return }
        send(.retryDue)
    }

    /// Gives up on the app volume for this session without a jump in
    /// loudness: music playing quieter than the system volume is paused
    /// (while the muted tap still silences it) before the tap goes.
    private func fallBack(_ reason: String) {
        log("app volume off (\(reason)); using the system volume for this session")
        let quieter = PlaybackActivity(status: lastStatus) == .playing && target.pointee < 1
        fallbackReason = reason
        guard quieter, let pausePlayer else { return updateMode() }
        log("app volume: pausing, so the music does not jump to the louder system volume")
        pausePlayer()
        // Let the pause reach the player before the mute goes.
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
            MainActor.assumeIsolated { AppVolume.shared.updateMode() }
        }
    }

    // MARK: Tap

    private struct Tap {
        var tapID: AudioObjectID
        var aggregateID: AudioObjectID
        var procID: AudioDeviceIOProcID
        /// The RemotePlayerService process object tapped.
        var processObject: AudioObjectID
        /// The output device the aggregate renders to.
        var outputDevice: AudioObjectID
        /// The aggregate's sample rate, which its IOProc's input runs at.
        var sampleRate: Double
        /// This IOProc's own render state (only its IO thread touches it
        /// once started), so an old and a new tap never share one.
        var render: UnsafeMutablePointer<RenderState>
    }

    private enum BuildResult {
        case built, noTarget
        case failed(TapFailure)
        /// The mode changed under it (the permission was revoked).
        case aborted
    }

    /// Builds a tap, aggregate device and IOProc for the RemotePlayerService
    /// copy that serves the helper. On success it becomes the tap in use and
    /// the previous one is kept, stopped, until the new one starts
    /// (`releaseReplaced`); on failure only the new, partial resources are
    /// destroyed and the tap in use stays as it is.
    private func build() -> BuildResult {
        guard #available(macOS 14.2, *) else { return .failed(.permanent("process taps need macOS 14.2")) }
        // A permission revoked since startup must not leave a muted tap.
        let current = CaptureAuthorization.preflight()
        guard current == .authorized else {
            permission = current
            updateMode()
            return .aborted
        }
        guard let process = selectProcess() else { return .noTarget }
        guard let output = HAL.defaultOutputDevice(), let outputUID = HAL.string(output, kAudioDevicePropertyDeviceUID) else {
            return .failed(.transient("no default output device"))
        }

        let description = CATapDescription(stereoMixdownOfProcesses: [process.objectID])
        description.name = "nu11signal volume"
        description.isPrivate = true
        description.muteBehavior = .muted
        var tapID = AudioObjectID(kAudioObjectUnknown)
        var result = AudioHardwareCreateProcessTap(description, &tapID)
        guard result == noErr, tapID != kAudioObjectUnknown else {
            return .failed(.transient("could not create the process tap, OSStatus \(result)"))
        }
        let tapFormat = HAL.format(tapID, kAudioTapPropertyFormat, kAudioObjectPropertyScopeGlobal)
        let tapUID = HAL.string(tapID, kAudioTapPropertyUID) ?? description.uuid.uuidString
        guard let tapFormat, Self.renderable(tapFormat) else {
            AudioHardwareDestroyProcessTap(tapID)
            return .failed(.permanent("the tap's format is not 32-bit float"))
        }

        let aggregate: [String: Any] = [
            kAudioAggregateDeviceNameKey: "nu11signal volume",
            kAudioAggregateDeviceUIDKey: UUID().uuidString,
            kAudioAggregateDeviceMainSubDeviceKey: outputUID,
            kAudioAggregateDeviceIsPrivateKey: true,
            kAudioAggregateDeviceIsStackedKey: false,
            kAudioAggregateDeviceTapAutoStartKey: true,
            kAudioAggregateDeviceSubDeviceListKey: [[kAudioSubDeviceUIDKey: outputUID]],
            kAudioAggregateDeviceTapListKey: [[kAudioSubTapDriftCompensationKey: true, kAudioSubTapUIDKey: tapUID]],
        ]
        var aggregateID = AudioObjectID(kAudioObjectUnknown)
        result = AudioHardwareCreateAggregateDevice(aggregate as CFDictionary, &aggregateID)
        guard result == noErr, aggregateID != kAudioObjectUnknown else {
            AudioHardwareDestroyProcessTap(tapID)
            return .failed(.transient("could not create the aggregate device, OSStatus \(result)"))
        }
        let outputFormat = HAL.format(aggregateID, kAudioDevicePropertyStreamFormat, kAudioObjectPropertyScopeOutput)
        guard let outputFormat, Self.renderable(outputFormat) else {
            AudioHardwareDestroyAggregateDevice(aggregateID)
            AudioHardwareDestroyProcessTap(tapID)
            return .failed(.permanent("the output format is not 32-bit float"))
        }

        let render = UnsafeMutablePointer<RenderState>.allocate(capacity: 1)
        render.initialize(to: RenderState(
            current: target.pointee, increment: GainRamp.increment(sampleRate: outputFormat.mSampleRate),
            fadeIncrement: GainRamp.increment(sampleRate: outputFormat.mSampleRate, seconds: GainRamp.shutdownSeconds),
            tapChannels: Int(max(tapFormat.mChannelsPerFrame, 1))))
        let target = self.target
        let fading = self.fading
        var ring: UnsafeMutableRawPointer?
        if #available(macOS 15.0, *) { ring = levelMeter?.ringPointer }
        var procID: AudioDeviceIOProcID?
        // No dispatch queue: the block runs on the real-time IO thread.
        result = AudioDeviceCreateIOProcIDWithBlock(&procID, aggregateID, nil) { _, input, _, output, _ in
            AppVolume.renderGain(input: input, output: output, target: target, fading: fading, state: render)
            if let ring { AppVolume.capture(input: input, ring: ring, state: render) }
        }
        guard result == noErr, let procID else {
            AudioHardwareDestroyAggregateDevice(aggregateID)
            AudioHardwareDestroyProcessTap(tapID)
            render.deallocate()
            return .failed(.transient("could not create the IOProc, OSStatus \(result)"))
        }
        // Swap: the old IOProc stops (its tap still mutes) before the new
        // one can start; an older kept tap is no longer needed.
        stop()
        if let replaced { destroy(replaced) }
        replaced = tap
        tap = Tap(tapID: tapID, aggregateID: aggregateID, procID: procID, processObject: process.objectID,
                  outputDevice: output, sampleRate: outputFormat.mSampleRate, render: render)
        log("app volume: tapping RemotePlayerService pid \(process.pid) (responsible: this helper) on \(outputUID) "
            + "(\(Int(outputFormat.mSampleRate)) Hz, \(tapFormat.mChannelsPerFrame) channels, gain \(target.pointee))")
        return .built
    }

    /// Starts the IOProc; the failure, if it does not.
    private func start() -> TapFailure? {
        guard let tap else { return .transient("no tap to start") }
        guard !running else { return nil }
        // No IOProc runs yet: the ring may be emptied of the last tap's audio.
        if #available(macOS 15.0, *) { levelMeter?.ring.reset() }
        let result = AudioDeviceStart(tap.aggregateID, tap.procID)
        guard result == noErr else { return .transient("could not start the aggregate device, OSStatus \(result)") }
        running = true
        if #available(macOS 15.0, *) { levelMeter?.start(sampleRate: tap.sampleRate) }
        return nil
    }

    private func stop() {
        if #available(macOS 15.0, *) { levelMeter?.stop() }
        guard let tap, running else { return }
        AudioDeviceStop(tap.aggregateID, tap.procID)
        running = false
    }

    /// Releases every tap: the one in use and one a rebuild replaced.
    private func teardown() {
        stop()
        if let tap { destroy(tap) }
        if let replaced { destroy(replaced) }
        tap = nil
        replaced = nil
    }

    /// Destroys a tap whose IOProc is stopped (or never started); its render
    /// state goes only once the IOProc is gone.
    private func destroy(_ tap: Tap) {
        AudioDeviceDestroyIOProcID(tap.aggregateID, tap.procID)
        AudioHardwareDestroyAggregateDevice(tap.aggregateID)
        if #available(macOS 14.2, *) { AudioHardwareDestroyProcessTap(tap.tapID) }
        tap.render.deinitialize(count: 1)
        tap.render.deallocate()
    }

    @available(macOS 15.0, *)
    private var levelMeter: LevelMeter? { meter as? LevelMeter }

    private func selectProcess() -> AudioProcessRecord? {
        let records = HAL.processes().map { record -> AudioProcessRecord in
            var record = record
            if record.bundleID == RemotePlayerTarget.bundleID {
                record.responsiblePID = Relaunch.responsiblePID(of: record.pid)
            }
            return record
        }
        return RemotePlayerTarget.select(records, helperPID: getpid())
    }

    private static func renderable(_ format: AudioStreamBasicDescription) -> Bool {
        TapFormat.isFloat32(formatID: format.mFormatID, flags: format.mFormatFlags, bitsPerChannel: format.mBitsPerChannel)
    }

    // MARK: Listeners

    /// Watches the default output device and the process list, once app
    /// mode is on. The blocks run on the main queue.
    private func listen() {
        guard !listening else { return }
        listening = true
        HAL.listen(kAudioHardwarePropertyDefaultOutputDevice) {
            AppVolume.shared.outputDeviceChanged()
        }
        HAL.listen(kAudioHardwarePropertyProcessObjectList) {
            AppVolume.shared.processesChanged()
        }
    }

    private func outputDeviceChanged() {
        guard mode == .app, let tap, HAL.defaultOutputDevice() != tap.outputDevice else { return }
        log("app volume: the default output device changed; rebuilding")
        send(.rebuild(.outputDevice))
    }

    /// When the change matters is the lifecycle's rule; this compares the
    /// player's process with the tapped one.
    private func processesChanged() {
        guard mode == .app, lifecycle.watchesPlayerProcess else { return }
        let process = selectProcess()
        if let tap, process?.objectID == tap.processObject { return }
        if tap != nil { log("app volume: the RemotePlayerService process changed; rebuilding") }
        send(.rebuild(.playerProcess))
    }

    // MARK: Real-time rendering

    /// State only the IO thread touches after the IOProc starts.
    struct RenderState {
        var current: Float = 1
        var increment: Float = 1
        /// The increment while the helper exits (GainRamp.shutdownSeconds).
        var fadeIncrement: Float = 1
        var tapChannels = 2
    }

    /// The IOProc body: copies the tap's channels (the aggregate's last
    /// `tapChannels` input channels) to every output channel, scaled along
    /// the gain ramp (the slow shutdown ramp once `fading` is non-zero).
    /// Real-time safe: no allocation, locks or Swift
    /// reference counting (only raw pointers are captured). Missing input
    /// renders silence; output buffers are always fully written.
    nonisolated static func renderGain(input: UnsafePointer<AudioBufferList>, output: UnsafeMutablePointer<AudioBufferList>,
                                       target: UnsafeMutablePointer<Float>, fading: UnsafeMutablePointer<Float>,
                                       state: UnsafeMutablePointer<RenderState>) {
        let inputs = UnsafeMutableAudioBufferListPointer(UnsafeMutablePointer(mutating: input))
        let outputs = UnsafeMutableAudioBufferListPointer(output)
        let start = state.pointee.current
        let goal = target.pointee // an aligned 32-bit load: never torn
        let increment = fading.pointee != 0 ? state.pointee.fadeIncrement : state.pointee.increment

        var inputChannels = 0
        for buffer in inputs { inputChannels += Int(buffer.mNumberChannels) }
        let tapChannels = min(state.pointee.tapChannels, inputChannels)
        let firstTapChannel = inputChannels - tapChannels

        var outputChannel = 0
        var rendered = 0
        for buffer in outputs {
            let channels = Int(buffer.mNumberChannels)
            guard let data = buffer.mData, channels > 0 else { continue }
            let frames = Int(buffer.mDataByteSize) / (MemoryLayout<Float>.size * channels)
            let destination = data.assumingMemoryBound(to: Float.self)
            for channel in 0..<channels {
                var source: UnsafePointer<Float>?
                var stride = 1
                var sourceFrames = 0
                if tapChannels > 0 {
                    // Output channel n takes tap channel n modulo the tap's.
                    var wanted = firstTapChannel + (outputChannel + channel) % tapChannels
                    for candidate in inputs {
                        let count = Int(candidate.mNumberChannels)
                        if wanted < count {
                            if let samples = candidate.mData, count > 0 {
                                source = UnsafePointer(samples.assumingMemoryBound(to: Float.self)) + wanted
                                stride = count
                                sourceFrames = Int(candidate.mDataByteSize) / (MemoryLayout<Float>.size * count)
                            }
                            break
                        }
                        wanted -= count
                    }
                }
                GainRamp.apply(source: source, sourceStride: stride, sourceFrames: sourceFrames,
                               destination: destination + channel, destinationStride: channels, frames: frames,
                               start: start, target: goal, increment: increment)
            }
            outputChannel += channels
            rendered = max(rendered, frames)
        }
        state.pointee.current = GainRamp.gain(atFrame: rendered, start: start, target: goal, increment: increment)
    }
}

extension AppVolume {
    /// Appends the tap's input, mixed to mono, to the level meter's ring
    /// (see LevelMeter). It reads the input, before the gain, so the bars
    /// do not follow the volume. Real-time safe like renderGain: the ring
    /// comes as a raw pointer and is used without reference counting.
    nonisolated static func capture(input: UnsafePointer<AudioBufferList>, ring: UnsafeMutableRawPointer,
                                    state: UnsafeMutablePointer<RenderState>) {
        guard #available(macOS 15.0, *) else { return }
        let inputs = UnsafeMutableAudioBufferListPointer(UnsafeMutablePointer(mutating: input))
        var inputChannels = 0
        for buffer in inputs { inputChannels += Int(buffer.mNumberChannels) }
        let tapChannels = min(state.pointee.tapChannels, inputChannels)
        guard tapChannels > 0 else { return }
        // The tap's channels are the aggregate's last ones; mix those that
        // share the first one's buffer (all of them, when interleaved).
        var first = inputChannels - tapChannels
        for buffer in inputs {
            let count = Int(buffer.mNumberChannels)
            guard first < count else {
                first -= count
                continue
            }
            guard let data = buffer.mData else { return }
            let frames = Int(buffer.mDataByteSize) / (MemoryLayout<Float>.size * count)
            let source = UnsafePointer(data.assumingMemoryBound(to: Float.self)) + first
            Unmanaged<SampleRing>.fromOpaque(ring)._withUnsafeGuaranteedRef {
                $0.write(source: source, stride: count, channels: min(tapChannels, count - first), frames: frames)
            }
            return
        }
    }
}

/// Minimal HAL property access for AppVolume.
private enum HAL {
    static let system = AudioObjectID(kAudioObjectSystemObject)

    static func address(_ selector: AudioObjectPropertySelector,
                        _ scope: AudioObjectPropertyScope = kAudioObjectPropertyScopeGlobal) -> AudioObjectPropertyAddress {
        AudioObjectPropertyAddress(mSelector: selector, mScope: scope, mElement: kAudioObjectPropertyElementMain)
    }

    static func value<T>(_ object: AudioObjectID, _ selector: AudioObjectPropertySelector,
                         _ scope: AudioObjectPropertyScope = kAudioObjectPropertyScopeGlobal, _ initial: T) -> T? {
        var address = address(selector, scope)
        var value = initial
        var size = UInt32(MemoryLayout<T>.size)
        let status = withUnsafeMutablePointer(to: &value) { AudioObjectGetPropertyData(object, &address, 0, nil, &size, $0) }
        return status == noErr ? value : nil
    }

    static func string(_ object: AudioObjectID, _ selector: AudioObjectPropertySelector) -> String? {
        var address = address(selector)
        var value: Unmanaged<CFString>?
        var size = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
        guard AudioObjectGetPropertyData(object, &address, 0, nil, &size, &value) == noErr, let value else { return nil }
        return value.takeRetainedValue() as String
    }

    static func format(_ object: AudioObjectID, _ selector: AudioObjectPropertySelector,
                       _ scope: AudioObjectPropertyScope) -> AudioStreamBasicDescription? {
        value(object, selector, scope, AudioStreamBasicDescription())
    }

    static func defaultOutputDevice() -> AudioObjectID? {
        guard let device = value(system, kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectPropertyScopeGlobal,
                                 AudioObjectID(kAudioObjectUnknown)),
              device != kAudioObjectUnknown
        else { return nil }
        return device
    }

    /// The HAL's process objects (responsiblePID left unset).
    static func processes() -> [AudioProcessRecord] {
        var address = address(kAudioHardwarePropertyProcessObjectList)
        var size: UInt32 = 0
        guard AudioObjectGetPropertyDataSize(system, &address, 0, nil, &size) == noErr, size > 0 else { return [] }
        var objects = [AudioObjectID](repeating: 0, count: Int(size) / MemoryLayout<AudioObjectID>.size)
        guard AudioObjectGetPropertyData(system, &address, 0, nil, &size, &objects) == noErr else { return [] }
        return objects.map { object in
            AudioProcessRecord(
                objectID: object,
                pid: value(object, kAudioProcessPropertyPID, kAudioObjectPropertyScopeGlobal, pid_t(0)) ?? 0,
                bundleID: string(object, kAudioProcessPropertyBundleID) ?? "",
                isRunningOutput: (value(object, kAudioProcessPropertyIsRunningOutput, kAudioObjectPropertyScopeGlobal, UInt32(0)) ?? 0) != 0,
                responsiblePID: nil)
        }
    }

    /// Calls `changed` on the main actor whenever a system property changes.
    static func listen(_ selector: AudioObjectPropertySelector, _ changed: @escaping @MainActor () -> Void) {
        var address = address(selector)
        let status = AudioObjectAddPropertyListenerBlock(system, &address, DispatchQueue.main) { _, _ in
            MainActor.assumeIsolated { changed() }
        }
        if status != noErr {
            log("app volume: could not watch HAL property \(selector) (OSStatus \(status))")
        }
    }
}
