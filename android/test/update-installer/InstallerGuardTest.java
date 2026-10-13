package dev.wahh.nu11signal.tv;

import java.io.File;
import java.io.IOException;

public final class InstallerGuardTest {
    private static int assertions;
    private static final String PIN = "448148925a1ecad2f51f3f5007343535d22d41716b4f0b9c4d88daaed0494320";
    private static TvUpdateGuard.Metadata metadata(String pkg, String version, long code,
            boolean debug, String... signers) {
        return new TvUpdateGuard.Metadata(pkg, version, code, debug, signers);
    }
    private static TvUpdateGuard.Metadata good() {
        return metadata("dev.wahh.nu11signal.tv", "1.2.3", 10203, false, PIN);
    }
    private static TvUpdateGuard.Metadata installed() {
        return metadata("dev.wahh.nu11signal.tv", "1.2.2", 10202, false, PIN);
    }
    interface Action { void run() throws IOException; }
    private static void reject(String label, Action action) throws IOException {
        try { action.run(); } catch (IOException expected) {
            if (expected.getMessage() == null || expected.getMessage().isEmpty())
                throw new AssertionError("Missing actionable error: " + label);
            assertions++; return;
        }
        throw new AssertionError("Accepted " + label);
    }
    private static void rejectApk(String label, TvUpdateGuard.Metadata apk) throws IOException {
        reject(label, () -> TvUpdateGuard.requireApproved(apk, installed(), "1.2.3", 10203));
    }
    public static void main(String[] args) throws Exception {
        TvUpdateGuard.requireApproved(good(), installed(), "1.2.3", 10203); assertions++;
        rejectApk("parse failure", null);
        rejectApk("wrong package", metadata("other.app", "1.2.3", 10203, false, PIN));
        rejectApk("wrong version", metadata("dev.wahh.nu11signal.tv", "1.2.4", 10203, false, PIN));
        rejectApk("wrong code", metadata("dev.wahh.nu11signal.tv", "1.2.3", 10204, false, PIN));
        rejectApk("debuggable", metadata("dev.wahh.nu11signal.tv", "1.2.3", 10203, true, PIN));
        rejectApk("unsigned", metadata("dev.wahh.nu11signal.tv", "1.2.3", 10203, false));
        rejectApk("missing signers", metadata("dev.wahh.nu11signal.tv", "1.2.3", 10203, false, (String[]) null));
        rejectApk("wrong signer", metadata("dev.wahh.nu11signal.tv", "1.2.3", 10203, false, "debug"));
        rejectApk("multiple signers", metadata("dev.wahh.nu11signal.tv", "1.2.3", 10203, false, PIN, PIN));
        reject("missing installed", () -> TvUpdateGuard.requireApproved(good(), null, "1.2.3", 10203));
        reject("wrong installed package", () -> TvUpdateGuard.requireApproved(good(),
                metadata("other.app", "1.2.2", 10202, false, PIN), "1.2.3", 10203));
        reject("unsigned installed", () -> TvUpdateGuard.requireApproved(good(),
                metadata("dev.wahh.nu11signal.tv", "1.2.2", 10202, false), "1.2.3", 10203));
        reject("debug-key installed", () -> TvUpdateGuard.requireApproved(good(),
                metadata("dev.wahh.nu11signal.tv", "1.2.2", 10202, false, "debug"), "1.2.3", 10203));
        reject("debuggable installed", () -> TvUpdateGuard.requireApproved(good(),
                metadata("dev.wahh.nu11signal.tv", "1.2.2", 10202, true, PIN), "1.2.3", 10203));
        reject("multiple installed signers", () -> TvUpdateGuard.requireApproved(good(),
                metadata("dev.wahh.nu11signal.tv", "1.2.2", 10202, false, PIN, PIN), "1.2.3", 10203));
        reject("same versionCode", () -> TvUpdateGuard.requireApproved(good(), good(), "1.2.3", 10203));
        reject("downgrade", () -> TvUpdateGuard.requireApproved(installed(), good(), "1.2.2", 10202));
        reject("missing advertised version", () -> TvUpdateGuard.requireApproved(good(), installed(), null, 10203));
        File directory = new File(args[0]);
        String path = "/verified-12345678-1234-1234-1234-123456789abc.apk";
        reject("missing private directory", () -> TvUpdateGuard.resolveReadPath(null, path, "r"));
        File resolved = TvUpdateGuard.resolveReadPath(directory, path, "r");
        if (!resolved.getParentFile().equals(directory.getCanonicalFile())) throw new AssertionError("Wrong directory");
        assertions++;
        for (String mode : new String[]{"w", "rw", "rwt", "wa", "", null})
            reject("write mode " + mode, () -> TvUpdateGuard.resolveReadPath(directory, path, mode));
        for (String bad : new String[]{"/../" + path, "/arbitrary.apk", path + "/extra", path + "?x", "/%2e%2e/x.apk", path.substring(1)})
            reject("path " + bad, () -> TvUpdateGuard.resolveReadPath(directory, bad, "r"));
        java.nio.file.Files.createDirectories(directory.toPath());
        File outside = new File(directory.getParentFile(), "outside.apk");
        java.nio.file.Files.write(outside.toPath(), new byte[]{1});
        java.nio.file.Files.createSymbolicLink(new File(directory, path.substring(1)).toPath(), outside.toPath());
        reject("symlink escape", () -> TvUpdateGuard.resolveReadPath(directory, path, "r"));
        String[] mutableSigners = new String[]{PIN};
        TvUpdateGuard.Metadata copied = metadata("dev.wahh.nu11signal.tv", "1.2.3", 10203, false, mutableSigners);
        mutableSigners[0] = "debug";
        TvUpdateGuard.requireApproved(copied, installed(), "1.2.3", 10203); assertions++;
        System.out.println("PASS: " + assertions + " installer guard assertions");
    }
}
