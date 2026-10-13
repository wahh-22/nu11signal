package dev.wahh.nu11signal.tv;

import java.util.Locale;

/** Recognizes Apple Music's animated album art: HLS streams without an audio
 * group ("Anull" in the file name) on the music video host. The hidden music
 * page has no use for them, and decoding a dozen at once kept the TV's CPU
 * busy for many seconds after startup. Songs and anything with audio pass.
 * Pure Java so host tests run without the Android SDK.
 */
final class MotionArtwork {
    static final String HOST = "mvod.itunes.apple.com";

    private MotionArtwork() { }

    static boolean matches(String host, String path) {
        if (host == null || path == null || !HOST.equals(host.toLowerCase(Locale.ROOT))) return false;
        String name = path.substring(path.lastIndexOf('/') + 1);
        return name.contains("_Anull_");
    }
}
