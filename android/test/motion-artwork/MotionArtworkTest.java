package dev.wahh.nu11signal.tv;

public final class MotionArtworkTest {
    private static final String HOST = "mvod.itunes.apple.com";
    private static final String DIR = "/itunes-assets/HLSMusic221/v4/cc/1b/32/cc1b3206-f839-a82e-2fba-c09bea8a1b2c/";

    public static void main(String[] args) {
        int checks = 0;
        // Silent animated-artwork streams seen on the TV's music.apple.com home page.
        require(MotionArtwork.matches(HOST, DIR + "P1235481269_Anull_video_gr660_sdr_830x1108.m3u8"), "silent video playlist blocked"); checks++;
        require(MotionArtwork.matches(HOST, DIR + "P1235481269_Anull_video_gr660_sdr_830x1108-.mp4"), "silent video segment blocked"); checks++;
        require(MotionArtwork.matches(HOST, DIR + "P1235481269_Anull_trickPlay_gr560_sdr_830x1108_i.m3u8"), "trick-play stream blocked"); checks++;
        require(MotionArtwork.matches("MVOD.ITUNES.APPLE.COM", DIR + "P1_Anull_video_gr660_sdr_830x1108.m3u8"), "host compared case-insensitively"); checks++;
        // Everything with audio, or elsewhere, is left alone.
        require(!MotionArtwork.matches(HOST, DIR + "P1235481269_default.m3u8"), "master playlist allowed"); checks++;
        require(!MotionArtwork.matches(HOST, DIR + "P1235481269_A1234567_video_gr660_sdr_1920x1080.m3u8"), "video with an audio group allowed"); checks++;
        require(!MotionArtwork.matches(HOST, DIR + "P1235481269_Aaac_audio_stereo.m3u8"), "audio rendition allowed"); checks++;
        require(!MotionArtwork.matches("music.apple.com", "/assets/P1_Anull_video_x.m3u8"), "other hosts allowed"); checks++;
        require(!MotionArtwork.matches("evilmvod.itunes.apple.com.example", DIR + "P1_Anull_video_x.m3u8"), "look-alike host allowed"); checks++;
        require(!MotionArtwork.matches(HOST, "/itunes-assets/Anull_folder/P1_default.m3u8"), "only the file name counts"); checks++;
        require(!MotionArtwork.matches(null, DIR + "P1_Anull_video_x.m3u8"), "missing host allowed"); checks++;
        require(!MotionArtwork.matches(HOST, null), "missing path allowed"); checks++;
        System.out.println("PASS: " + checks + " motion artwork assertions");
    }

    private static void require(boolean value, String message) {
        if (!value) throw new AssertionError(message);
    }
}
