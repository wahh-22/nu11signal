package dev.wahh.nu11signal.tv;

import java.io.File;
import java.io.IOException;
import java.nio.file.Files;

public final class SnapshotLifecycleTest {
    private static int assertions;
    private static void check(boolean condition, String label) {
        if (!condition) throw new AssertionError(label);
        assertions++;
    }
    public static void main(String[] args) throws Exception {
        File root = new File(args[0]);
        Files.createDirectories(root.toPath());
        TvUpdateGuard.SnapshotPool pool = new TvUpdateGuard.SnapshotPool();
        TvUpdateGuard.Snapshot rejected = pool.create(root);
        Files.write(rejected.file.toPath(), new byte[]{1, 2});
        check(rejected.file.setReadOnly(), "Cannot seal rejection fixture");
        pool.retire(rejected); pool.cleanup();
        check(!rejected.file.exists(), "Rejected snapshot retained");
        TvUpdateGuard.Snapshot cancelled = pool.create(root);
        pool.retire(cancelled); pool.cleanup();
        check(!cancelled.file.exists(), "Cancelled/superseded snapshot retained");
        try { pool.handOff(cancelled); throw new AssertionError("Retired authorization resurrected"); }
        catch (IllegalStateException expected) { assertions++; }
        for (int i = 0; i < 10; i++) {
            TvUpdateGuard.Snapshot repeated = pool.create(root);
            pool.retire(repeated); pool.cleanup();
            check(!repeated.file.exists(), "Repeated rejection leaked a snapshot");
        }
        TvUpdateGuard.Snapshot handedOff = pool.create(root);
        Files.write(handedOff.file.toPath(), new byte[]{7});
        pool.handOff(handedOff); pool.retire(handedOff); pool.cleanup();
        check(handedOff.file.exists(), "Handed-off snapshot deleted");
        pool.handOff(pool.create(root));
        pool.handOff(pool.create(root));
        try { pool.create(root); throw new AssertionError("Retention limit exceeded"); }
        catch (IOException expected) { assertions++; }
        File download = new File(root, "download.apk");
        Files.write(download.toPath(), new byte[]{1});
        File arbitrary = new File(root, "verified-not-a-uuid.apk");
        Files.write(arbitrary.toPath(), new byte[]{1});
        try (java.io.FileInputStream reader = new java.io.FileInputStream(handedOff.file)) {
            // A new pool models a NEW PROCESS, never a second pool in one app process.
            TvUpdateGuard.SnapshotPool restarted = new TvUpdateGuard.SnapshotPool();
            TvUpdateGuard.Snapshot fresh = restarted.create(root);
            check(!handedOff.file.exists(), "Prior-process snapshot retained");
            check(reader.read() == 7, "Already-open descriptor broken by cold-start unlink");
            check(download.exists() && arbitrary.exists(), "Unrelated file deleted");
            restarted.retire(fresh); restarted.cleanup();
        }
        System.out.println("PASS: " + assertions + " snapshot lifecycle assertions");
    }
}
