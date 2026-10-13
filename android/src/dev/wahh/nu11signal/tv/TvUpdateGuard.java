package dev.wahh.nu11signal.tv;

import java.io.File;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;

/** Metadata policy only: Android's package installer must still verify APK integrity. */
public final class TvUpdateGuard {
    public static final String PACKAGE = "dev.wahh.nu11signal.tv";
    public static final String SIGNER_SHA256 =
            "448148925a1ecad2f51f3f5007343535d22d41716b4f0b9c4d88daaed0494320";
    private TvUpdateGuard() {}

    public static final class Metadata {
        final String packageName;
        final String version;
        final long code;
        final boolean debuggable;
        final String[] signers;
        public Metadata(String packageName, String version, long code, boolean debuggable,
                String[] signers) {
            this.packageName = packageName;
            this.version = version;
            this.code = code;
            this.debuggable = debuggable;
            this.signers = signers == null ? null : signers.clone();
        }
    }

    public static void requireApproved(Metadata apk, Metadata installed,
            String expectedVersion, long expectedCode) throws IOException {
        requireOfficial(apk, "Downloaded APK");
        requireOfficial(installed, "Installed app");
        if (expectedVersion == null || !expectedVersion.equals(apk.version)
                || expectedCode != apk.code)
            throw new IOException("APK version does not match the advertised release. Download again.");
        if (apk.code <= installed.code)
            throw new IOException("Only a strictly newer Android versionCode can be installed.");
        if (!apk.signers[0].equals(installed.signers[0]))
            throw new IOException("APK signer differs from the installed app. Update rejected.");
    }

    private static void requireOfficial(Metadata value, String label) throws IOException {
        if (value == null) throw new IOException(label + " could not be parsed or has no signing metadata.");
        if (!PACKAGE.equals(value.packageName)) throw new IOException(label + " has the wrong package name.");
        if (value.debuggable) throw new IOException(label + " is debuggable. Use an official release build.");
        if (value.signers == null || value.signers.length != 1
                || !SIGNER_SHA256.equals(value.signers[0]))
            throw new IOException(label + " must have exactly one pinned official signer. "
                    + "Debug-key installations cannot use this updater; no uninstall will be attempted.");
    }

    /** One pool per app PROCESS. create/cleanup are serialized on a background worker. */
    static final class SnapshotPool {
        private final List<Snapshot> snapshots = new ArrayList<>();
        private boolean initialized;
        Snapshot create(File directory) throws IOException {
            cleanup();
            if (!initialized) {
                File[] old = directory.listFiles();
                if (old == null) throw new IOException("Cannot inspect private updater directory.");
                for (File file : old) {
                    if (!file.getName().matches("verified-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\\.apk")) continue;
                    File exact = resolveReadPath(directory, "/" + file.getName(), "r");
                    if (!exact.delete() && exact.exists())
                        throw new IOException("Cannot clean prior-process APK snapshots. Restart and retry.");
                }
                initialized = true;
            }
            synchronized (this) {
                if (snapshots.size() >= 3)
                    throw new IOException("Three APK snapshots are retained. Restart the app before another update.");
            }
            File file = new File(directory, "verified-" + UUID.randomUUID() + ".apk");
            if (!file.createNewFile()) throw new IOException("Cannot create unique APK snapshot.");
            Snapshot snapshot = new Snapshot(file);
            synchronized (this) { snapshots.add(snapshot); }
            return snapshot;
        }
        synchronized void retire(Snapshot snapshot) {
            if (!snapshot.handedOff) snapshot.retired = true;
        }
        synchronized void handOff(Snapshot snapshot) {
            if (snapshot.retired) throw new IllegalStateException("Snapshot was invalidated.");
            snapshot.handedOff = true;
        }
        void cleanup() throws IOException {
            List<Snapshot> retired = new ArrayList<>();
            synchronized (this) {
                for (Snapshot snapshot : snapshots)
                    if (snapshot.retired && !snapshot.handedOff) retired.add(snapshot);
            }
            // No file I/O while holding the lock used by foreground invalidation/handoff.
            for (Snapshot snapshot : retired) {
                if (!snapshot.file.delete() && snapshot.file.exists())
                    throw new IOException("Cannot remove rejected APK snapshot. Restart and retry.");
                synchronized (this) { snapshots.remove(snapshot); }
            }
        }
    }

    static final class Snapshot {
        final File file;
        private boolean retired;
        private boolean handedOff;
        Snapshot(File file) { this.file = file; }
    }

    public static File resolveReadPath(File directory, String path, String mode) throws IOException {
        if (!"r".equals(mode)) throw new IOException("Updater APK provider permits read-only access.");
        if (path == null || !path.matches("/verified-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\\.apk"))
            throw new IOException("Invalid updater APK path.");
        if (directory == null) throw new IOException("Private updater directory is unavailable.");
        File root = directory.getCanonicalFile();
        File file = new File(root, path.substring(1));
        if (!file.equals(file.getCanonicalFile()) || !root.equals(file.getParentFile()))
            throw new IOException("Updater APK path escapes its private directory.");
        return file;
    }
}
