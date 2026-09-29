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
/// output device, and renders it there scaled by the app gain.
///
/// App mode needs the helper to be its own responsible process (see
/// Relaunch), macOS 14.2+ and the audio capture permission; the permission
/// is asked for on the first playback, and until it is granted (or when it
/// is denied, or a tap cannot be built) the system volume is used. Nothing
/// muted is ever created before the permission is known to be granted.
///
/// Resources: the tap and aggregate are built lazily when playback starts;
/// the IOProc stops when playback pauses (the muted tap stays, so resuming
/// is never briefly loud) and everything is destroyed when playback stops,
/// on exit, and before a rebuild (a new default output device, or a new
/// RemotePlayerService copy).
@MainActor
final class AppVolume {
    static let shared = AppVolume()

    /// Called whenever `mode` changes (the state emitter reports it).
    var onModeChange: (() -> Void)?

    private(set) var mode: VolumeMode = .system
    private let policy: VolumeModePolicy
    private var permission: CapturePermission
    private var requesting = false
    /// Set once a tap failed: the rest of the session uses the system volume.
    private var broken = false

    /// The app level, 0...1, as reported and stored.
    private var level: Double
    /// The amplitude the IOProc ramps to; written here, read on the IO thread.
    private let target = UnsafeMutablePointer<Float>.allocate(capacity: 1)
    /// IO-thread-only render state; set before each IOProc start.
    private let render = UnsafeMutablePointer<RenderState>.allocate(capacity: 1)

    private var tap: Tap?
    private var running = false
    private var status = "stopped"
    /// A play was asked for and the player has not reported a new status.
    private var expectingPlayback = false
    /// RemotePlayerService pids seen at startup: other clients' copies.
    private let baseline: Set<Int32>
    private var listening = false

    private init() {
        let environment = ProcessInfo.processInfo.environment
        var supported = false
        if #available(macOS 14.2, *) { supported = true }
        policy = VolumeModePolicy(disclaimed: Relaunch.isDisclaimed, tapsSupported: supported,
                                  forcedSystem: HelperLaunch.forcesSystemVolume(environment: environment))
        permission = policy.mode(.authorized) == .app ? CaptureAuthorization.preflight() : .unavailable
        level = AppGain.stored(UserDefaults.standard.object(forKey: AppGain.defaultsKey))
        target.initialize(to: AppGain.amplitude(level: level))
        render.initialize(to: RenderState())
        baseline = Set(HAL.processes().filter { $0.bundleID == RemotePlayerTarget.bundleID }.map(\.pid))
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
            target.pointee = AppGain.amplitude(level: level)
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
        expectingPlayback = true
        if mode == .app {
            build()
        }
    }

    /// The player's status, as the state emitter reads it (repeatedly).
    func playbackStatus(_ status: String) {
        guard status != self.status else { return }
        self.status = status
        expectingPlayback = false
        guard mode == .app else { return }
        switch status {
        case "playing", "seeking":
            build()
            start()
        case "stopped":
            teardown()
        default:
            stop()
        }
    }

    /// Releases the tap and aggregate device; called on exit.
    func shutdown() {
        teardown()
    }

    private func permissionAnswered(_ granted: Bool) {
        requesting = false
        permission = granted ? .authorized : .denied
        log("audio capture permission \(granted ? "granted" : "denied")")
        updateMode()
        if mode == .app, playing {
            build()
            start()
        }
    }

    private var playing: Bool { status == "playing" || status == "seeking" }

    private func updateMode() {
        let next: VolumeMode = broken ? .system : policy.mode(permission)
        guard next != mode else { return }
        mode = next
        if mode == .app {
            target.pointee = AppGain.amplitude(level: level)
            listen()
        } else {
            teardown()
        }
        onModeChange?()
    }

    /// Gives up on the app volume for this session.
    private func fail(_ reason: String) {
        log("app volume unavailable (\(reason)); using the system volume")
        teardown()
        broken = true
        updateMode()
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
    }

    /// Builds the tap, aggregate device and IOProc if there are none and
    /// the RemotePlayerService copy exists (it starts with the first play;
    /// the process list listener builds once it appears).
    private func build() {
        guard tap == nil, mode == .app else { return }
        guard #available(macOS 14.2, *) else { return }
        // A permission revoked since startup must not leave a muted tap.
        guard CaptureAuthorization.preflight() == .authorized else {
            permission = .denied
            updateMode()
            return
        }
        guard let process = selectProcess() else { return }
        guard let output = HAL.defaultOutputDevice(), let outputUID = HAL.string(output, kAudioDevicePropertyDeviceUID) else {
            log("app volume: no default output device")
            return
        }

        let description = CATapDescription(stereoMixdownOfProcesses: [process.objectID])
        description.name = "nu11signal volume"
        description.isPrivate = true
        description.muteBehavior = .muted
        var tapID = AudioObjectID(kAudioObjectUnknown)
        var status = AudioHardwareCreateProcessTap(description, &tapID)
        guard status == noErr, tapID != kAudioObjectUnknown else {
            return fail("could not create the process tap, OSStatus \(status)")
        }
        let tapFormat = HAL.format(tapID, kAudioTapPropertyFormat, kAudioObjectPropertyScopeGlobal)
        let tapUID = HAL.string(tapID, kAudioTapPropertyUID) ?? description.uuid.uuidString
        guard let tapFormat, Self.renderable(tapFormat) else {
            AudioHardwareDestroyProcessTap(tapID)
            return fail("the tap's format is not 32-bit float")
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
        status = AudioHardwareCreateAggregateDevice(aggregate as CFDictionary, &aggregateID)
        guard status == noErr, aggregateID != kAudioObjectUnknown else {
            AudioHardwareDestroyProcessTap(tapID)
            return fail("could not create the aggregate device, OSStatus \(status)")
        }
        let outputFormat = HAL.format(aggregateID, kAudioDevicePropertyStreamFormat, kAudioObjectPropertyScopeOutput)
        guard let outputFormat, Self.renderable(outputFormat) else {
            AudioHardwareDestroyAggregateDevice(aggregateID)
            AudioHardwareDestroyProcessTap(tapID)
            return fail("the output format is not 32-bit float")
        }

        render.pointee = RenderState(
            current: target.pointee, increment: GainRamp.increment(sampleRate: outputFormat.mSampleRate),
            tapChannels: Int(max(tapFormat.mChannelsPerFrame, 1)))
        let target = self.target, render = self.render
        var procID: AudioDeviceIOProcID?
        // No dispatch queue: the block runs on the real-time IO thread.
        status = AudioDeviceCreateIOProcIDWithBlock(&procID, aggregateID, nil) { _, input, _, output, _ in
            AppVolume.renderGain(input: input, output: output, target: target, state: render)
        }
        guard status == noErr, let procID else {
            AudioHardwareDestroyAggregateDevice(aggregateID)
            AudioHardwareDestroyProcessTap(tapID)
            return fail("could not create the IOProc, OSStatus \(status)")
        }
        tap = Tap(tapID: tapID, aggregateID: aggregateID, procID: procID, processObject: process.objectID,
                  outputDevice: output)
        log("app volume: tapping RemotePlayerService pid \(process.pid) on \(outputUID) "
            + "(\(Int(outputFormat.mSampleRate)) Hz, \(tapFormat.mChannelsPerFrame) channels)")
    }

    private func start() {
        guard let tap, !running else { return }
        let status = AudioDeviceStart(tap.aggregateID, tap.procID)
        guard status == noErr else { return fail("could not start the aggregate device, OSStatus \(status)") }
        running = true
    }

    private func stop() {
        guard let tap, running else { return }
        AudioDeviceStop(tap.aggregateID, tap.procID)
        running = false
    }

    private func teardown() {
        stop()
        guard let tap else { return }
        AudioDeviceDestroyIOProcID(tap.aggregateID, tap.procID)
        AudioHardwareDestroyAggregateDevice(tap.aggregateID)
        if #available(macOS 14.2, *) { AudioHardwareDestroyProcessTap(tap.tapID) }
        self.tap = nil
    }

    /// Rebuilds for a new output device or RemotePlayerService copy.
    private func rebuild() {
        teardown()
        guard mode == .app, playing || expectingPlayback else { return }
        build()
        if playing { start() }
    }

    private func selectProcess() -> AudioProcessRecord? {
        let records = HAL.processes().map { record -> AudioProcessRecord in
            var record = record
            if record.bundleID == RemotePlayerTarget.bundleID {
                record.responsiblePID = Relaunch.responsiblePID(of: record.pid)
            }
            return record
        }
        return RemotePlayerTarget.select(records, helperPID: getpid(), baseline: baseline)
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
        rebuild()
    }

    private func processesChanged() {
        guard mode == .app, tap != nil || playing || expectingPlayback else { return }
        let process = selectProcess()
        if let tap, process?.objectID == tap.processObject { return }
        if tap != nil { log("app volume: the RemotePlayerService process changed; rebuilding") }
        rebuild()
    }

    // MARK: Real-time rendering

    /// State only the IO thread touches after the IOProc starts.
    struct RenderState {
        var current: Float = 1
        var increment: Float = 1
        var tapChannels = 2
    }

    /// The IOProc body: copies the tap's channels (the aggregate's last
    /// `tapChannels` input channels) to every output channel, scaled along
    /// the gain ramp. Real-time safe: no allocation, locks or Swift
    /// reference counting (only raw pointers are captured). Missing input
    /// renders silence; output buffers are always fully written.
    nonisolated static func renderGain(input: UnsafePointer<AudioBufferList>, output: UnsafeMutablePointer<AudioBufferList>,
                                       target: UnsafeMutablePointer<Float>, state: UnsafeMutablePointer<RenderState>) {
        let inputs = UnsafeMutableAudioBufferListPointer(UnsafeMutablePointer(mutating: input))
        let outputs = UnsafeMutableAudioBufferListPointer(output)
        let start = state.pointee.current
        let goal = target.pointee // an aligned 32-bit load: never torn
        let increment = state.pointee.increment

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
