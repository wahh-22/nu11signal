import CoreFoundation
import Darwin
import Foundation
import Nu11SignalProtocol

/// The audio capture permission (`kTCCServiceAudioCapture`), checked and
/// requested through the TCC framework's `TCCAccessPreflight` and
/// `TCCAccessRequest`. Apple has no public API for this permission; the
/// functions are looked up at run time, and without them the permission is
/// `.unavailable` (the volume stays the system volume). Checking first
/// lets the helper ask before it creates a muted tap: a muted tap created
/// while the prompt is pending silences the music until the user answers.
enum CaptureAuthorization {
    private typealias Preflight = @convention(c) (CFString, CFDictionary?) -> Int
    private typealias Request = @convention(c) (CFString, CFDictionary?, @escaping @convention(block) (Bool) -> Void) -> Void

    private static let service = "kTCCServiceAudioCapture" as CFString
    nonisolated(unsafe) private static let framework =
        dlopen("/System/Library/PrivateFrameworks/TCC.framework/Versions/A/TCC", RTLD_NOW)

    private static func function(_ name: String) -> UnsafeMutableRawPointer? {
        guard let framework else { return nil }
        return dlsym(framework, name)
    }

    /// The current permission, without prompting.
    static func preflight() -> CapturePermission {
        guard let symbol = function("TCCAccessPreflight") else { return .unavailable }
        return CapturePermission(preflightResult: unsafeBitCast(symbol, to: Preflight.self)(service, nil))
    }

    /// Shows the permission prompt; `done` gets the answer on an arbitrary
    /// queue. Returns false (and never calls `done`) when it cannot ask.
    static func request(_ done: @escaping @Sendable (Bool) -> Void) -> Bool {
        guard let symbol = function("TCCAccessRequest") else { return false }
        unsafeBitCast(symbol, to: Request.self)(service, nil) { granted in done(granted) }
        return true
    }
}
