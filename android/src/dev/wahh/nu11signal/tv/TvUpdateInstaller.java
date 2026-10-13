package dev.wahh.nu11signal.tv;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ComponentName;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.Signature;
import android.content.pm.ResolveInfo;
import android.net.Uri;
import android.os.Looper;
import android.os.SystemClock;
import android.provider.Settings;
import android.system.ErrnoException;
import android.system.Os;
import android.util.Log;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.List;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;

/** Native-only boundary; the caller owns explicit consent, lifecycle and background execution. */
public final class TvUpdateInstaller {
    private static final long MAX_BYTES = 256L * 1024 * 1024;
    private static final long VALID_FOR_MS = 5 * 60 * 1000;
    private static final Object VALIDATION_LOCK = new Object();
    private static final TvUpdateGuard.SnapshotPool SNAPSHOTS = new TvUpdateGuard.SnapshotPool();
    // At most one running and one queued cleanup. A queued sweep covers all retired snapshots.
    private static final ThreadPoolExecutor CLEANUP = new ThreadPoolExecutor(1, 1, 0,
            TimeUnit.MILLISECONDS, new ArrayBlockingQueue<>(1), runnable -> {
                Thread thread = new Thread(runnable, "tv-update-cleanup");
                thread.setDaemon(true);
                return thread;
            }, new ThreadPoolExecutor.DiscardPolicy());
    private final Activity activity;
    private long generation;
    private Validated validated;

    private static final class Validated {
        final File source;
        final String version;
        final long code;
        final TvUpdateGuard.Snapshot snapshot;
        final long time;
        Validated(File source, String version, long code, TvUpdateGuard.Snapshot snapshot) {
            this.source = source; this.version = version; this.code = code; this.snapshot = snapshot;
            this.time = SystemClock.elapsedRealtime();
        }
    }

    public TvUpdateInstaller(Activity activity) {
        if (activity == null) throw new IllegalArgumentException("Activity is required.");
        this.activity = activity;
    }

    /** Cancels publication of in-flight validation; never deletes files held by an installer. */
    public void clearValidation() {
        final Validated old;
        synchronized (this) { generation++; old = validated; validated = null; }
        discardLater(old);
    }

    private static void discardLater(Validated old) {
        if (old == null) return;
        TvUpdateApkProvider.forget(old.snapshot.file);
        SNAPSHOTS.retire(old.snapshot);
        CLEANUP.execute(() -> {
            synchronized (VALIDATION_LOCK) {
                try { SNAPSHOTS.cleanup(); }
                catch (IOException e) { Log.w("TvUpdateInstaller", "Snapshot cleanup failed; retention limit remains enforced."); }
            }
        });
    }

    /** Blocking worker-only policy checks on a separate immutable snapshot, not the download. */
    public void validate(File apk, String expectedVersion, long expectedCode) throws IOException {
        if (Looper.myLooper() == Looper.getMainLooper())
            throw new IOException("APK validation must run on a background thread.");
        final long attempt;
        final Validated old;
        synchronized (this) { old = validated; validated = null; attempt = ++generation; }
        discardLater(old);
        synchronized (VALIDATION_LOCK) { validateSnapshot(apk, expectedVersion, expectedCode, attempt); }
    }

    private synchronized boolean current(long attempt) { return attempt == generation; }

    private void validateSnapshot(File apk, String expectedVersion, long expectedCode, long attempt) throws IOException {
        if (!current(attempt)) throw new IOException("APK validation cancelled.");
        if (apk == null) throw new IOException("No APK was downloaded.");
        File directory = new File(activity.getFilesDir(), "updates").getCanonicalFile();
        File source = apk.getAbsoluteFile();
        if (!source.equals(source.getCanonicalFile()) || !directory.equals(source.getParentFile())
                || !source.getName().endsWith(".apk") || source.getName().startsWith("verified-")
                || !source.isFile())
            throw new IOException("Download must be a regular private updates APK, not a symlink or snapshot.");
        TvUpdateGuard.Snapshot owned = SNAPSHOTS.create(directory);
        File snapshot = owned.file;
        boolean published = false;
        try {
            copyAndSeal(source, snapshot, attempt);
            PackageManager pm = activity.getPackageManager();
            // Inspected AOSP verifies here; the public metadata API is not our cross-device
            // integrity guarantee. Android's package installer must verify again.
            PackageInfo archive = pm.getPackageArchiveInfo(snapshot.getPath(), PackageManager.GET_SIGNING_CERTIFICATES);
            final PackageInfo installed;
            try { installed = pm.getPackageInfo(TvUpdateGuard.PACKAGE, PackageManager.GET_SIGNING_CERTIFICATES); }
            catch (PackageManager.NameNotFoundException e) { throw new IOException("Cannot inspect the installed app.", e); }
            TvUpdateGuard.requireApproved(metadata(archive), metadata(installed), expectedVersion, expectedCode);
            synchronized (this) {
                if (attempt != generation || Thread.currentThread().isInterrupted())
                    throw new IOException("APK validation cancelled; validate again before installing.");
                validated = new Validated(source, expectedVersion, expectedCode, owned);
                published = true;
            }
        } finally {
            if (!published) { SNAPSHOTS.retire(owned); SNAPSHOTS.cleanup(); }
        }
    }

    private void copyAndSeal(File source, File snapshot, long attempt) throws IOException {
        try (FileInputStream input = new FileInputStream(source);
                FileOutputStream output = new FileOutputStream(snapshot)) {
            byte[] buffer = new byte[64 * 1024];
            long total = 0;
            int count;
            while ((count = input.read(buffer)) != -1) {
                if (!current(attempt) || Thread.currentThread().isInterrupted())
                    throw new IOException("APK validation cancelled.");
                total += count;
                if (total > MAX_BYTES) throw new IOException("APK exceeds the 256 MiB validation limit.");
                output.write(buffer, 0, count);
            }
            if (total == 0) throw new IOException("Downloaded APK is empty.");
            output.getFD().sync();
        }
        try { Os.chmod(snapshot.getPath(), 0400); }
        catch (ErrnoException e) { throw new IOException("Cannot make APK snapshot read-only.", e); }
    }

    private static TvUpdateGuard.Metadata metadata(PackageInfo info) throws IOException {
        if (info == null || info.applicationInfo == null || info.signingInfo == null) return null;
        Signature[] certificates = info.signingInfo.getApkContentsSigners();
        String[] hashes = certificates == null ? null : new String[certificates.length];
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            if (certificates != null) for (int i = 0; i < certificates.length; i++) {
                if (certificates[i] == null) throw new IOException("Missing APK signer certificate.");
                byte[] bytes = digest.digest(certificates[i].toByteArray());
                StringBuilder hex = new StringBuilder(64);
                for (byte b : bytes) hex.append(Character.forDigit((b >>> 4) & 15, 16))
                        .append(Character.forDigit(b & 15, 16));
                hashes[i] = hex.toString();
            }
        } catch (NoSuchAlgorithmException e) { throw new IOException("SHA-256 is unavailable.", e); }
        return new TvUpdateGuard.Metadata(info.packageName, info.versionName, info.getLongVersionCode(),
                (info.applicationInfo.flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0, hashes);
    }

    public boolean canInstall() { return activity.getPackageManager().canRequestPackageInstalls(); }

    /** Foreground explicit user choice only; caller revalidates after returning from Settings. */
    public void openPermissionSettings() {
        requireMain();
        clearValidation();
        activity.startActivity(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                Uri.parse("package:" + activity.getPackageName())));
    }

    /** No file work. Consumes a recent worker-produced snapshot; never bypasses platform approval. */
    public void install(File apk, String expectedVersion, long expectedCode) throws IOException {
        requireMain();
        final Validated ready;
        synchronized (this) { ready = validated; validated = null; generation++; }
        boolean launched = false;
        Uri uri = null;
        try {
            if (ready == null || apk == null || !ready.source.equals(apk.getAbsoluteFile())
                    || !ready.version.equals(expectedVersion) || ready.code != expectedCode
                    || SystemClock.elapsedRealtime() - ready.time > VALID_FOR_MS)
                throw new IOException("APK validation is missing or expired. Validate again before installing.");
            if (!canInstall()) throw new IOException("Allow updates from this app in Android Settings, then validate again.");
            uri = TvUpdateApkProvider.authorize(ready.snapshot.file);
            Intent intent = new Intent(Intent.ACTION_INSTALL_PACKAGE).setDataAndType(uri,
                    "application/vnd.android.package-archive")
                    .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
            intent.setClipData(ClipData.newRawUri("nu11signal update", uri));
            // Never grant the APK to an arbitrary third-party ACTION_INSTALL_PACKAGE handler.
            List<ResolveInfo> installers = activity.getPackageManager().queryIntentActivities(intent,
                    PackageManager.MATCH_DEFAULT_ONLY | PackageManager.MATCH_SYSTEM_ONLY);
            if (installers.size() != 1 || installers.get(0).activityInfo == null)
                throw new IOException("No unique Android system package installer is available.");
            ResolveInfo installer = installers.get(0);
            if (!installer.activityInfo.exported || !installer.activityInfo.enabled
                    || (installer.activityInfo.applicationInfo.flags
                        & (ApplicationInfo.FLAG_SYSTEM | ApplicationInfo.FLAG_UPDATED_SYSTEM_APP)) == 0)
                throw new IOException("Android system package installer is unavailable.");
            intent.setComponent(new ComponentName(installer.activityInfo.packageName,
                    installer.activityInfo.name));
            try {
                activity.startActivity(intent);
                SNAPSHOTS.handOff(ready.snapshot);
                launched = true;
            } catch (ActivityNotFoundException | SecurityException e) {
                throw new IOException("Android package installer is unavailable or denied access.", e);
            }
        } finally {
            if (!launched) {
                try {
                    if (uri != null) activity.revokeUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION);
                } finally { discardLater(ready); }
            }
        }
    }

    private static void requireMain() {
        if (Looper.myLooper() != Looper.getMainLooper())
            throw new IllegalStateException("Installer and permission settings require the main thread.");
    }
}
