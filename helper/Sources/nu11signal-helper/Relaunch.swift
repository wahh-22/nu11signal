import Darwin
import Foundation
import Nu11SignalProtocol

/// Makes the helper its own "responsible process". macOS attributes TCC
/// permissions to the responsible process, which for a program started
/// from a terminal is the terminal: the audio capture permission the app
/// volume needs would be asked of (and silently refused to) the terminal.
///
/// At startup, before any thread or state exists, the helper re-executes
/// itself in place (`POSIX_SPAWN_SETEXEC`: same pid, same stdin/stdout/
/// stderr, so the Go client's pipes and process handle are untouched) with
/// the private `responsibility_spawnattrs_setdisclaim` attribute, the one
/// LLDB and Chromium use. The symbol is looked up at run time: without it,
/// or if the exec fails, the helper runs on as it was and the volume falls
/// back to the system volume. HelperLaunch.relaunchAttemptedKey, set before
/// the attempt, stops a loop.
enum Relaunch {
    /// dlfcn.h's RTLD_DEFAULT (a macro Swift does not import): every image.
    private static let defaultHandle = UnsafeMutableRawPointer(bitPattern: -2)
    private typealias SetDisclaim = @convention(c) (UnsafeMutablePointer<posix_spawnattr_t?>, Int32) -> Int32
    private typealias ResponsibleFor = @convention(c) (pid_t) -> pid_t

    /// Re-executes the helper disclaimed, once; returns only when it does not.
    static func disclaimIfNeeded() {
        guard HelperLaunch.shouldRelaunch(environment: ProcessInfo.processInfo.environment) else { return }
        setenv(HelperLaunch.relaunchAttemptedKey, "1", 1)
        guard let symbol = dlsym(defaultHandle, "responsibility_spawnattrs_setdisclaim") else {
            log("responsibility_spawnattrs_setdisclaim is unavailable; the volume stays the system volume")
            return
        }
        guard let path = Bundle.main.executablePath else { return }
        let setDisclaim = unsafeBitCast(symbol, to: SetDisclaim.self)
        var attributes: posix_spawnattr_t?
        guard posix_spawnattr_init(&attributes) == 0 else { return }
        defer { posix_spawnattr_destroy(&attributes) }
        guard posix_spawnattr_setflags(&attributes, Int16(POSIX_SPAWN_SETEXEC)) == 0,
              setDisclaim(&attributes, 1) == 0
        else {
            log("could not prepare the disclaimed relaunch; the volume stays the system volume")
            return
        }
        // Only returns on failure: SETEXEC replaces this image.
        let status = posix_spawn(nil, path, nil, &attributes, CommandLine.unsafeArgv, environ)
        log("disclaimed relaunch failed (\(String(cString: strerror(status)))); the volume stays the system volume")
    }

    /// Whether the helper is its own responsible process. Without the
    /// private lookup this cannot be known, and neither can which player
    /// process serves the helper (see RemotePlayerTarget), so it is false.
    static var isDisclaimed: Bool {
        guard let symbol = dlsym(defaultHandle, "responsibility_get_pid_responsible_for_pid") else { return false }
        return responsiblePID(of: getpid(), symbol) == getpid()
    }

    /// The pid macOS holds responsible for pid, when that can be read.
    static func responsiblePID(of pid: pid_t) -> pid_t? {
        guard let symbol = dlsym(defaultHandle, "responsibility_get_pid_responsible_for_pid") else { return nil }
        let responsible = responsiblePID(of: pid, symbol)
        return responsible > 0 ? responsible : nil
    }

    private static func responsiblePID(of pid: pid_t, _ symbol: UnsafeMutableRawPointer) -> pid_t {
        unsafeBitCast(symbol, to: ResponsibleFor.self)(pid)
    }
}
