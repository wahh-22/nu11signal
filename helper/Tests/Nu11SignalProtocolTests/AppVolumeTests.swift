import XCTest
@testable import Nu11SignalProtocol

final class HelperLaunchTests: XCTestCase {
    func testRelaunchesOnceUnlessSystemVolumeIsForced() {
        let cases: [(String, [String: String], Bool)] = [
            ("first launch", [:], true),
            ("relaunch already attempted", [HelperLaunch.relaunchAttemptedKey: "1"], false),
            ("system volume forced", [HelperLaunch.volumeModeKey: "system"], false),
            ("app volume asked for", [HelperLaunch.volumeModeKey: "app"], true),
        ]
        for (name, environment, want) in cases {
            XCTAssertEqual(HelperLaunch.shouldRelaunch(environment: environment), want, name)
        }
    }

    func testOnlyTheExactSystemValueForcesSystemVolume() {
        XCTAssertTrue(HelperLaunch.forcesSystemVolume(environment: [HelperLaunch.volumeModeKey: "system"]))
        XCTAssertFalse(HelperLaunch.forcesSystemVolume(environment: [HelperLaunch.volumeModeKey: "app"]))
        XCTAssertFalse(HelperLaunch.forcesSystemVolume(environment: [HelperLaunch.volumeModeKey: ""]))
        XCTAssertFalse(HelperLaunch.forcesSystemVolume(environment: [:]))
    }
}

final class VolumeModePolicyTests: XCTestCase {
    private let able = VolumeModePolicy(disclaimed: true, tapsSupported: true, forcedSystem: false)

    func testTheAppVolumeNeedsEveryPrerequisiteAndAGrant() {
        XCTAssertEqual(able.mode(.authorized), .app)
        for permission in [CapturePermission.denied, .undetermined, .unavailable] {
            XCTAssertEqual(able.mode(permission), .system, "\(permission)")
        }
        let lacking: [(String, VolumeModePolicy)] = [
            ("not its own responsible process", VolumeModePolicy(disclaimed: false, tapsSupported: true, forcedSystem: false)),
            ("macOS without process taps", VolumeModePolicy(disclaimed: true, tapsSupported: false, forcedSystem: false)),
            ("system volume forced", VolumeModePolicy(disclaimed: true, tapsSupported: true, forcedSystem: true)),
        ]
        for (name, policy) in lacking {
            XCTAssertEqual(policy.mode(.authorized), .system, name)
            XCTAssertFalse(policy.shouldRequest(.undetermined), name)
        }
    }

    func testOnlyAnUndeterminedPermissionIsAskedFor() {
        XCTAssertTrue(able.shouldRequest(.undetermined))
        for permission in [CapturePermission.authorized, .denied, .unavailable] {
            XCTAssertFalse(able.shouldRequest(permission), "\(permission)")
        }
    }

    func testPreflightResultsMapToPermissions() {
        XCTAssertEqual(CapturePermission(preflightResult: 0), .authorized)
        XCTAssertEqual(CapturePermission(preflightResult: 1), .denied)
        XCTAssertEqual(CapturePermission(preflightResult: 2), .undetermined)
        // Anything TCC does not document cannot be trusted either way.
        XCTAssertEqual(CapturePermission(preflightResult: -1), .unavailable)
        XCTAssertEqual(CapturePermission(preflightResult: 3), .unavailable)
    }
}

final class AppGainTests: XCTestCase {
    func testStoredLevelsDefaultToFullAndAreClamped() {
        let cases: [(String, Any?, Double)] = [
            ("never stored", nil, 1),
            ("stored", 0.6, 0.6),
            ("stored as float", Float(0.25), 0.25),
            ("above", 3.0, 1),
            ("below", -1.0, 0),
            ("not a number", "loud", 1),
            ("nan", Double.nan, 1),
            ("bool", true, 1),
        ]
        for (name, value, want) in cases {
            XCTAssertEqual(AppGain.stored(value), want, accuracy: 1e-9, name)
        }
    }

    func testAmplitudeFollowsASquareLawCurve() {
        XCTAssertEqual(AppGain.amplitude(level: 0), 0)
        XCTAssertEqual(AppGain.amplitude(level: 1), 1)
        XCTAssertEqual(AppGain.amplitude(level: 0.5), 0.25, accuracy: 1e-6)
        XCTAssertEqual(AppGain.amplitude(level: 0.6), 0.36, accuracy: 1e-6)
        XCTAssertEqual(AppGain.amplitude(level: 2), 1)
        XCTAssertEqual(AppGain.amplitude(level: -1), 0)
        XCTAssertEqual(AppGain.amplitude(level: .nan), 0)
    }
}

final class GainRampTests: XCTestCase {
    func testIncrementCoversAFullSwingInTheRampTime() {
        let increment = GainRamp.increment(sampleRate: 48_000)
        let frames = Float(48_000 * GainRamp.seconds)
        XCTAssertEqual(increment * frames, 1, accuracy: 1e-4)
        // A device without a rate jumps rather than dividing by zero.
        XCTAssertEqual(GainRamp.increment(sampleRate: 0), 1)
        XCTAssertEqual(GainRamp.increment(sampleRate: .nan), 1)
    }

    func testGainMovesTowardTheTargetAndStopsThere() {
        XCTAssertEqual(GainRamp.gain(atFrame: 0, start: 1, target: 0.5, increment: 0.1), 1)
        XCTAssertEqual(GainRamp.gain(atFrame: 2, start: 1, target: 0.5, increment: 0.1), 0.8, accuracy: 1e-6)
        XCTAssertEqual(GainRamp.gain(atFrame: 100, start: 1, target: 0.5, increment: 0.1), 0.5)
        XCTAssertEqual(GainRamp.gain(atFrame: 3, start: 0, target: 1, increment: 0.25), 0.75, accuracy: 1e-6)
        XCTAssertEqual(GainRamp.gain(atFrame: 9, start: 0, target: 1, increment: 0.25), 1)
        XCTAssertEqual(GainRamp.gain(atFrame: 5, start: 0.3, target: 0.3, increment: 0.25), 0.3)
    }

    func testApplyScalesInterleavedSamplesAlongTheRamp() {
        // Two interleaved channels, four frames; channel 1 is rendered.
        let source: [Float] = [9, 1, 9, 1, 9, 1, 9, 1]
        var destination = [Float](repeating: -1, count: 8)
        source.withUnsafeBufferPointer { src in
            destination.withUnsafeMutableBufferPointer { dst in
                GainRamp.apply(source: src.baseAddress! + 1, sourceStride: 2, sourceFrames: 4,
                               destination: dst.baseAddress! + 1, destinationStride: 2, frames: 4,
                               start: 1, target: 0.5, increment: 0.25)
            }
        }
        XCTAssertEqual(destination, [-1, 1, -1, 0.75, -1, 0.5, -1, 0.5])
    }

    func testApplySilencesMissingOrShortSources() {
        var destination = [Float](repeating: -1, count: 3)
        destination.withUnsafeMutableBufferPointer { dst in
            GainRamp.apply(source: nil, sourceStride: 1, sourceFrames: 0, destination: dst.baseAddress!,
                           destinationStride: 1, frames: 3, start: 1, target: 1, increment: 1)
        }
        XCTAssertEqual(destination, [0, 0, 0])

        let source: [Float] = [0.5]
        destination = [-1, -1, -1]
        source.withUnsafeBufferPointer { src in
            destination.withUnsafeMutableBufferPointer { dst in
                GainRamp.apply(source: src.baseAddress!, sourceStride: 1, sourceFrames: 1, destination: dst.baseAddress!,
                               destinationStride: 1, frames: 3, start: 1, target: 1, increment: 1)
            }
        }
        XCTAssertEqual(destination, [0.5, 0, 0])
    }
}

final class RemotePlayerTargetTests: XCTestCase {
    private let service = RemotePlayerTarget.bundleID

    private func record(_ object: UInt32, pid: Int32, bundle: String? = nil, output: Bool = false,
                        responsible: Int32? = nil) -> AudioProcessRecord {
        AudioProcessRecord(objectID: object, pid: pid, bundleID: bundle ?? service, isRunningOutput: output,
                           responsiblePID: responsible)
    }

    func testTheCopyThisHelperIsResponsibleForWins() {
        let records = [
            record(1, pid: 900, output: true, responsible: 500),
            record(2, pid: 400, output: false, responsible: 42),
            record(3, pid: 950, bundle: "com.example.other", output: true, responsible: 42),
        ]
        XCTAssertEqual(RemotePlayerTarget.select(records, helperPID: 42)?.objectID, 2)
    }

    func testACopyNotAttributedToTheHelperIsNeverTapped() {
        // Another client's copy started after the helper, playing: tapping
        // it would mute that client.
        let unrelated = [
            record(1, pid: 100, output: true, responsible: 77),
            record(2, pid: 300, output: true, responsible: 300),
            record(3, pid: 200, output: false),
        ]
        XCTAssertNil(RemotePlayerTarget.select(unrelated, helperPID: 42))
        XCTAssertNil(RemotePlayerTarget.select([record(1, pid: 100, bundle: "com.apple.Music", output: true, responsible: 42)],
                                               helperPID: 42))
    }

    func testTheHelpersCopyWinsBesideAnUnrelatedNewCopy() {
        let records = [
            record(1, pid: 500, output: true, responsible: 77), // new, playing, someone else's
            record(2, pid: 400, output: false, responsible: 42),
        ]
        XCTAssertEqual(RemotePlayerTarget.select(records, helperPID: 42)?.objectID, 2)
    }

    func testAmongEqualCandidatesTheNewestPidWins() {
        let records = [record(1, pid: 200, output: true, responsible: 42), record(2, pid: 250, output: true, responsible: 42)]
        XCTAssertEqual(RemotePlayerTarget.select(records, helperPID: 42)?.objectID, 2)
    }
}

final class TapFormatTests: XCTestCase {
    func testOnlyThirtyTwoBitFloatLinearPCMIsRendered() {
        let lpcm: UInt32 = 0x6C70_636D // 'lpcm'
        let float: UInt32 = 1          // kAudioFormatFlagIsFloat
        XCTAssertTrue(TapFormat.isFloat32(formatID: lpcm, flags: float | 8, bitsPerChannel: 32))
        XCTAssertFalse(TapFormat.isFloat32(formatID: lpcm, flags: float, bitsPerChannel: 64))
        XCTAssertFalse(TapFormat.isFloat32(formatID: lpcm, flags: 4, bitsPerChannel: 32))
        XCTAssertFalse(TapFormat.isFloat32(formatID: 0x6161_6320, flags: float, bitsPerChannel: 32))
    }
}

final class PlaybackActivityTests: XCTestCase {
    func testStatusNamesMapToActivities() {
        let cases: [(String, PlaybackActivity)] = [
            ("playing", .playing), ("seeking", .playing), ("stopped", .stopped),
            ("paused", .paused), ("interrupted", .paused),
        ]
        for (status, want) in cases {
            XCTAssertEqual(PlaybackActivity(status: status), want, status)
            XCTAssertEqual(PlaybackActivity.isPlaying(status), want == .playing, status)
        }
    }
}

final class TapLifecycleTests: XCTestCase {
    private typealias L = TapLifecycle

    /// A lifecycle driven through events, collecting every action.
    private func drive(_ events: [L.Event], from start: L = L()) -> (L, [L.Action]) {
        var lifecycle = start
        var actions: [L.Action] = []
        for event in events { actions += lifecycle.handle(event) }
        return (lifecycle, actions)
    }

    func testAPlayBuildsTheMutedTapFirstAndStartsItWhenTheAudioPlays() {
        var lifecycle = L()
        XCTAssertEqual(lifecycle.handle(.prepare), [.build])
        XCTAssertEqual(lifecycle.phase, .building)
        XCTAssertEqual(lifecycle.handle(.built), [])
        XCTAssertEqual(lifecycle.phase, .building)
        XCTAssertEqual(lifecycle.handle(.playback(.playing)), [.start])
        XCTAssertEqual(lifecycle.handle(.started), [])
        XCTAssertEqual(lifecycle.phase, .running)
    }

    func testPauseStopsTheAudioThreadAndStopReleasesEverything() {
        var (lifecycle, _) = drive([.prepare, .built, .playback(.playing), .started])
        XCTAssertEqual(lifecycle.handle(.playback(.paused)), [.stop])
        XCTAssertEqual(lifecycle.phase, .paused)
        XCTAssertEqual(lifecycle.handle(.prepare), [], "a resume keeps the paused tap")
        XCTAssertEqual(lifecycle.handle(.playback(.playing)), [.start])
        XCTAssertEqual(lifecycle.handle(.playback(.stopped)), [.teardown])
        XCTAssertEqual(lifecycle.phase, .idle)
    }

    func testAFailedPlayLeavesNoMutedTap() {
        var (lifecycle, _) = drive([.prepare, .built])
        XCTAssertEqual(lifecycle.handle(.playFailed), [.teardown])
        XCTAssertEqual(lifecycle.phase, .idle)
        // A failed play of another queue while music plays keeps its tap.
        (lifecycle, _) = drive([.prepare, .built, .playback(.playing), .started, .prepare])
        XCTAssertEqual(lifecycle.handle(.playFailed), [])
        XCTAssertEqual(lifecycle.phase, .running)
    }

    func testAFailedPlayCancelsAPendingRetry() {
        var (lifecycle, _) = drive([.prepare, .failed(.transient("device busy"))])
        XCTAssertEqual(lifecycle.handle(.playFailed), [.teardown], "no muted leftover")
        XCTAssertEqual(lifecycle.phase, .idle)
        XCTAssertEqual(lifecycle.handle(.retryDue), [], "the stale retry does nothing")
    }

    func testTransientFailuresRetryWithBackoffThenFallBack() {
        var (lifecycle, _) = drive([.prepare, .built, .playback(.playing)])
        var delays: [Double] = []
        for attempt in 1...L.retryDelays.count {
            let actions = lifecycle.handle(.failed(.transient("start failed")))
            guard case let .scheduleRetry(after)? = actions.last else { return XCTFail("\(actions)") }
            // The muted tap stays while retrying: silence, never a jump
            // to the system volume.
            XCTAssertEqual(actions.first, .stop)
            XCTAssertFalse(actions.contains(.teardown))
            XCTAssertEqual(lifecycle.phase, .retrying(attempt: attempt))
            delays.append(after)
            XCTAssertEqual(lifecycle.handle(.retryDue), [.build])
            XCTAssertEqual(lifecycle.handle(.built), [.start])
        }
        XCTAssertEqual(delays, L.retryDelays)
        XCTAssertEqual(delays, delays.sorted(), "the backoff grows")
        let last = lifecycle.handle(.failed(.transient("start failed")))
        guard last.count == 1, case let .fallBack(reason)? = last.last else { return XCTFail("\(last)") }
        XCTAssertTrue(reason.contains("start failed"), reason)
        XCTAssertEqual(lifecycle.phase, .fallback(reason: reason))
        XCTAssertEqual(lifecycle.handle(.playback(.playing)), [], "the fallback lasts")
        XCTAssertEqual(lifecycle.handle(.prepare), [])
    }

    func testAStartedTapForgetsEarlierFailures() {
        var (lifecycle, _) = drive([.prepare, .playback(.playing), .failed(.transient("x")), .retryDue, .built])
        XCTAssertEqual(lifecycle.phase, .running)
        (lifecycle, _) = drive([.started], from: lifecycle)
        for _ in 1...L.retryDelays.count {
            let actions = lifecycle.handle(.failed(.transient("x")))
            XCTAssertFalse(actions.contains { if case .fallBack = $0 { return true }; return false }, "\(actions)")
            _ = lifecycle.handle(.retryDue)
            _ = lifecycle.handle(.built)
        }
    }

    func testAPermanentFailureFallsBackAtOnce() {
        var (lifecycle, _) = drive([.prepare])
        XCTAssertEqual(lifecycle.handle(.failed(.permanent("not float"))), [.fallBack(reason: "not float")])
        XCTAssertEqual(lifecycle.phase, .fallback(reason: "not float"))
    }

    func testAMissingPlayerProcessWaitsBeforeTheAudioButRetriesOnceItPlays() {
        var lifecycle = L()
        _ = lifecycle.handle(.prepare)
        XCTAssertEqual(lifecycle.handle(.noTarget), [])
        XCTAssertEqual(lifecycle.phase, .idle)
        // The process list changed: the copy may be there now.
        XCTAssertEqual(lifecycle.handle(.rebuild(.playerProcess)), [.build])
        XCTAssertEqual(lifecycle.handle(.noTarget), [])
        // The audio plays and still no copy is attributed to the helper.
        XCTAssertEqual(lifecycle.handle(.playback(.playing)), [.build])
        let actions = lifecycle.handle(.noTarget)
        XCTAssertEqual(actions.first, .stop)
        XCTAssertEqual(actions.last, .scheduleRetry(after: L.retryDelays[0]))
    }

    func testDeviceOrProcessChangesRebuildOnlyWhatIsNeeded() {
        var (lifecycle, _) = drive([.prepare, .built, .playback(.playing), .started])
        XCTAssertEqual(lifecycle.handle(.rebuild(.outputDevice)), [.build], "the old tap stays until the new one starts")
        XCTAssertEqual(lifecycle.handle(.built), [.start])
        XCTAssertEqual(lifecycle.handle(.started), [.releaseReplaced])
        _ = lifecycle.handle(.playback(.paused))
        XCTAssertEqual(lifecycle.handle(.rebuild(.outputDevice)), [.teardown], "a paused tap is rebuilt on resume")
        XCTAssertEqual(lifecycle.phase, .idle)
        XCTAssertEqual(lifecycle.handle(.rebuild(.outputDevice)), [])
        XCTAssertEqual(lifecycle.handle(.prepare), [.build])
        // A retry already pending rebuilds by itself.
        (lifecycle, _) = drive([.prepare, .failed(.transient("x"))])
        XCTAssertEqual(lifecycle.handle(.rebuild(.outputDevice)), [])
        XCTAssertEqual(lifecycle.handle(.rebuild(.playerProcess)), [])
    }

    func testAFailedRebuildKeepsTheOldTapMuted() {
        var (lifecycle, _) = drive([.gain(quieterThanSystem: true), .prepare, .built, .playback(.playing), .started])
        XCTAssertEqual(lifecycle.handle(.rebuild(.outputDevice)), [.build])
        let actions = lifecycle.handle(.failed(.transient("device changing")))
        // The old tap still mutes the player: its audio thread stops, so
        // the music is silent while it retries, never loud; no pause.
        XCTAssertEqual(actions, [.stop, .scheduleRetry(after: L.retryDelays[0])])
        XCTAssertTrue(lifecycle.hasTap)
        XCTAssertTrue(lifecycle.tapMutesPlayer)
        XCTAssertEqual(lifecycle.handle(.retryDue), [.build])
        XCTAssertEqual(lifecycle.handle(.built), [.start])
        XCTAssertEqual(lifecycle.handle(.started), [.releaseReplaced], "the swap releases the old tap only now")
        XCTAssertEqual(lifecycle.phase, .running)
    }

    func testASuccessfulRebuildSwapsWithoutATeardown() {
        var (lifecycle, _) = drive([.prepare, .built, .playback(.playing), .started])
        let (after, actions) = drive([.rebuild(.playerProcess), .built, .started], from: lifecycle)
        XCTAssertEqual(actions, [.build, .start, .releaseReplaced])
        XCTAssertFalse(actions.contains(.teardown))
        XCTAssertTrue(after.tapMutesPlayer)
        lifecycle = after
        XCTAssertEqual(lifecycle.handle(.started), [], "nothing left to release")
    }

    func testAnUncoveredPlayerIsPausedDuringRetriesWhenQuieterAndResumedOnSuccess() {
        var (lifecycle, _) = drive([.gain(quieterThanSystem: true), .prepare, .built, .playback(.playing), .started])
        // The player moved to a new process the old tap does not mute.
        XCTAssertEqual(lifecycle.handle(.rebuild(.playerProcess)), [.build])
        XCTAssertFalse(lifecycle.tapMutesPlayer)
        XCTAssertEqual(lifecycle.handle(.noTarget),
                       [.stop, .pausePlayer, .scheduleRetry(after: L.retryDelays[0])])
        XCTAssertTrue(lifecycle.holdingPlayer)
        // The pause the lifecycle asked for does not cancel the retry.
        XCTAssertEqual(lifecycle.handle(.playback(.paused)), [])
        XCTAssertEqual(lifecycle.handle(.retryDue), [.build])
        XCTAssertEqual(lifecycle.handle(.built), [.resumePlayer])
        XCTAssertFalse(lifecycle.holdingPlayer)
        XCTAssertEqual(lifecycle.handle(.playback(.playing)), [.start])
        XCTAssertEqual(lifecycle.handle(.started), [.releaseReplaced])
        XCTAssertEqual(lifecycle.phase, .running)
    }

    func testAnUncoveredPlayerAtFullGainKeepsPlayingDuringRetries() {
        var (lifecycle, _) = drive([.gain(quieterThanSystem: false), .prepare, .built, .playback(.playing), .started,
                                    .rebuild(.playerProcess)])
        XCTAssertEqual(lifecycle.handle(.noTarget), [.stop, .scheduleRetry(after: L.retryDelays[0])],
                       "full gain is the system volume: no jump")
        XCTAssertFalse(lifecycle.holdingPlayer)
    }

    func testAPlayerProcessChangeDuringARetryPausesWhenQuieter() {
        var (lifecycle, _) = drive([.gain(quieterThanSystem: true), .prepare, .built, .playback(.playing), .started,
                                    .rebuild(.outputDevice), .failed(.transient("x"))])
        XCTAssertTrue(lifecycle.tapMutesPlayer)
        XCTAssertEqual(lifecycle.handle(.rebuild(.playerProcess)), [.pausePlayer], "the kept tap no longer mutes it")
        XCTAssertEqual(lifecycle.handle(.rebuild(.playerProcess)), [], "paused once")
        // A retry that keeps failing ends in the fallback, the player paused.
        (lifecycle, _) = drive([.retryDue, .failed(.transient("x")), .retryDue, .failed(.transient("x")),
                                .retryDue], from: lifecycle)
        let last = lifecycle.handle(.failed(.transient("x")))
        guard case .fallBack? = last.last else { return XCTFail("\(last)") }
        XCTAssertFalse(last.contains(.resumePlayer))
    }

    func testStoppingWhileHoldingThePlayerForgetsTheHold() {
        var (lifecycle, _) = drive([.gain(quieterThanSystem: true), .prepare, .built, .playback(.playing), .started,
                                    .rebuild(.playerProcess), .noTarget])
        XCTAssertTrue(lifecycle.holdingPlayer)
        XCTAssertEqual(lifecycle.handle(.playback(.stopped)), [.teardown])
        XCTAssertFalse(lifecycle.holdingPlayer)
        XCTAssertEqual(lifecycle.handle(.retryDue), [])
    }

    func testProcessListChangesAreComparedOnlyWhenTheyMatter() {
        var lifecycle = L()
        XCTAssertFalse(lifecycle.watchesPlayerProcess, "idle, nothing playing")
        _ = lifecycle.handle(.prepare)
        XCTAssertTrue(lifecycle.watchesPlayerProcess, "building for a play")
        (lifecycle, _) = drive([.prepare, .built, .playback(.playing), .started])
        XCTAssertTrue(lifecycle.watchesPlayerProcess, "running")
        _ = lifecycle.handle(.playback(.paused))
        XCTAssertTrue(lifecycle.watchesPlayerProcess, "paused with a tap")
        // Retrying with a tap that still mutes the player: a change makes
        // it stale; retrying without one: the retry builds anyway.
        (lifecycle, _) = drive([.prepare, .built, .playback(.playing), .started, .failed(.transient("x"))])
        XCTAssertTrue(lifecycle.watchesPlayerProcess)
        (lifecycle, _) = drive([.prepare, .playback(.playing), .failed(.transient("x"))])
        XCTAssertFalse(lifecycle.watchesPlayerProcess)
        (lifecycle, _) = drive([.prepare, .failed(.permanent("x"))])
        XCTAssertFalse(lifecycle.watchesPlayerProcess, "the fallback")
        (lifecycle, _) = drive([.prepare, .built, .playback(.playing), .started, .playback(.stopped)])
        XCTAssertFalse(lifecycle.watchesPlayerProcess, "stopped")
    }

    func testStoppingDuringARetryCancelsIt() {
        var (lifecycle, _) = drive([.prepare, .playback(.playing), .failed(.transient("x"))])
        XCTAssertEqual(lifecycle.handle(.playback(.stopped)), [.teardown])
        XCTAssertEqual(lifecycle.phase, .idle)
        XCTAssertEqual(lifecycle.handle(.retryDue), [])
    }

    func testARetryDueWhilePausedWaitsForTheNextPlay() {
        var (lifecycle, _) = drive([.prepare, .built, .playback(.playing), .started,
                                    .failed(.transient("x")), .playback(.paused)])
        XCTAssertEqual(lifecycle.handle(.retryDue), [.teardown])
        XCTAssertEqual(lifecycle.phase, .idle)
        XCTAssertEqual(lifecycle.handle(.prepare), [.build])
    }
}
