package dev.wahh.nu11signal.tv;

/** Immutable advertised metadata, not a statement of APK authenticity. */
public final class TvUpdateRelease {
    public final String version;
    public final long versionCode;
    public final String url;
    public final long size;

    /** Fixture-friendly constructor; the client revalidates all fields before download. */
    public TvUpdateRelease(String version, long versionCode, String url, long size) {
        this.version = version;
        this.versionCode = versionCode;
        this.url = url;
        this.size = size;
    }

    /** Canonical stable core only, using the component/code bounds in android/build.sh. */
    public static long versionCode(String stableCore) {
        if (stableCore == null || !stableCore.matches("(0|[1-9][0-9]{0,5})\\.(0|[1-9][0-9]?)\\.(0|[1-9][0-9]?)")) {
            throw new IllegalArgumentException("Invalid stable Android version");
        }
        String[] parts = stableCore.split("\\.");
        long code = Long.parseLong(parts[0]) * 10000 + Long.parseLong(parts[1]) * 100 + Long.parseLong(parts[2]);
        if (code < 1 || code > 2100000000L) throw new IllegalArgumentException("Android versionCode out of range");
        return code;
    }
}
