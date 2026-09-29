import XCTest
@testable import Nu11SignalProtocol

final class HelperLaunchTests: XCTestCase {
    func testRelaunchesOnceUnlessSystemVolumeIsForced() {
        let cases: [(String, [String: String], Bool)] = [
            ("first launch", [:], true),
            ("already relaunched", [HelperLaunch.relaunchedKey: "1"], false),
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
        XCTAssertEqual(CapturePermission(preflightResult: -1), .undetermined)
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
        XCTAssertEqual(RemotePlayerTarget.select(records, helperPID: 42, baseline: [])?.objectID, 2)
    }

    func testWithoutResponsibilityANewCopyPlayingWins() {
        let records = [
            record(1, pid: 100, output: true),  // another client's copy, older than the helper
            record(2, pid: 300, output: false), // idle new copy
            record(3, pid: 200, output: true),  // started after the helper, playing
        ]
        XCTAssertEqual(RemotePlayerTarget.select(records, helperPID: 42, baseline: [100])?.objectID, 3)
    }

    func testANewIdleCopyIsTheLastResortAndOldCopiesNever() {
        XCTAssertEqual(RemotePlayerTarget.select([record(1, pid: 100, output: true), record(2, pid: 300)],
                                                 helperPID: 42, baseline: [100])?.objectID, 2)
        XCTAssertNil(RemotePlayerTarget.select([record(1, pid: 100, output: true)], helperPID: 42, baseline: [100]))
        XCTAssertNil(RemotePlayerTarget.select([record(1, pid: 100, bundle: "com.apple.Music", output: true)],
                                               helperPID: 42, baseline: []))
    }

    func testAmongEqualCandidatesTheNewestPidWins() {
        let records = [record(1, pid: 200, output: true), record(2, pid: 250, output: true)]
        XCTAssertEqual(RemotePlayerTarget.select(records, helperPID: 42, baseline: [])?.objectID, 2)
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
