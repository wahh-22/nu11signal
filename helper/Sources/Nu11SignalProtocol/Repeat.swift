// Pure argument handling of the `setRepeat` command, free of MusicKit so
// it can be unit tested.
import Foundation

/// Repeat modes on the wire: "off", "all" (the queue) or "one" (the
/// current song). `state` events carry the player's mode as `repeat`.
public enum RepeatSetting {
    public static let modes: Set<String> = ["off", "all", "one"]

    /// The mode `setRepeat` asks for; anything but a wire name is an error.
    public static func requested(_ request: Request) throws -> String {
        guard let mode = request.string("mode"), modes.contains(mode) else {
            throw ArgumentError(description: #"\#(request.cmd) requires "mode": "off", "all" or "one""#)
        }
        return mode
    }
}
