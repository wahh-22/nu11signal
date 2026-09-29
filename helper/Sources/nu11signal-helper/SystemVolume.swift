import AudioToolbox
import CoreAudio
import Nu11SignalProtocol

/// The volume of the system's default output device. MusicKit's
/// ApplicationMusicPlayer has no volume of its own on macOS (and
/// MPMusicPlayerController is unavailable there), so the helper drives the
/// device's virtual main volume, as the menu bar volume control does.
enum SystemVolume {
    private static var volumeAddress: AudioObjectPropertyAddress {
        AudioObjectPropertyAddress(
            mSelector: kAudioHardwareServiceDeviceProperty_VirtualMainVolume,
            mScope: kAudioDevicePropertyScopeOutput,
            mElement: kAudioObjectPropertyElementMain
        )
    }

    /// The current level, 0...1.
    static func level() throws -> Double {
        let device = try outputDevice()
        var address = volumeAddress
        guard AudioObjectHasProperty(device, &address) else {
            throw CommandError("the output device has no volume control")
        }
        var level: Float32 = 0
        var size = UInt32(MemoryLayout<Float32>.size)
        try check(AudioObjectGetPropertyData(device, &address, 0, nil, &size, &level), "read the output volume")
        return VolumeLevel.reported(level)
    }

    /// Sets the level; fails on devices whose volume cannot be changed
    /// (HDMI and some USB outputs).
    static func setLevel(_ level: Float32) throws {
        let device = try outputDevice()
        var address = volumeAddress
        var settable: DarwinBoolean = false
        guard AudioObjectHasProperty(device, &address),
              AudioObjectIsPropertySettable(device, &address, &settable) == noErr, settable.boolValue
        else {
            throw CommandError("the output device's volume cannot be changed")
        }
        var value = level
        try check(
            AudioObjectSetPropertyData(device, &address, 0, nil, UInt32(MemoryLayout<Float32>.size), &value),
            "set the output volume"
        )
    }

    private static func outputDevice() throws -> AudioObjectID {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDefaultOutputDevice,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain
        )
        var device = AudioObjectID(kAudioObjectUnknown)
        var size = UInt32(MemoryLayout<AudioObjectID>.size)
        try check(
            AudioObjectGetPropertyData(AudioObjectID(kAudioObjectSystemObject), &address, 0, nil, &size, &device),
            "find the default output device"
        )
        guard device != kAudioObjectUnknown else {
            throw CommandError("there is no default output device")
        }
        return device
    }

    private static func check(_ status: OSStatus, _ action: String) throws {
        guard status == noErr else {
            throw CommandError("could not \(action) (OSStatus \(status))")
        }
    }
}
