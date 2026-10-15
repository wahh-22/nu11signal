package dev.wahh.nu11signal.tv;

import android.app.Activity;
import android.app.AlertDialog;
import android.os.Handler;
import android.os.Looper;
import java.io.File;
import java.io.IOException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.function.BooleanSupplier;

/** Native dialogs own remote focus; no installer capability is exposed to either WebView. */
final class TvUpdateUi {
    private final Activity activity;
    private final BooleanSupplier safeToOffer;
    private final TvUpdateClient client = new TvUpdateClient();
    private final TvUpdateInstaller installer;
    private final TvUpdateState state = new TvUpdateState();
    private final Handler main = new Handler(Looper.getMainLooper());
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private Future<?> job;
    private AlertDialog dialog;
    private TvUpdateRelease offer;
    private File downloaded;
    private boolean foreground, destroyed, checked, dismissed, permissionPending;

    TvUpdateUi(Activity activity, BooleanSupplier safeToOffer) {
        this.activity = activity;
        this.safeToOffer = safeToOffer;
        installer = new TvUpdateInstaller(activity);
    }

    void resume() {
        foreground = true;
        state.resume();
        if (permissionPending) {
            permissionPending = false;
            // Returning from Settings is not itself an instruction to install.
            main.post(() -> { if (available()) showOffer(); });
        } else maybeOffer();
    }

    void pause() {
        foreground = false;
        state.pause();
        if (job != null) job.cancel(true);
        installer.clearValidation();
        closeDialog();
    }

    void destroy() {
        destroyed = true;
        pause();
        state.destroy();
        worker.shutdownNow();
        main.removeCallbacksAndMessages(null);
        // Downloads and handed-off snapshots are private. The next attempt cleans unused
        // downloads; the installer owns its separate snapshot lifecycle.
    }

    private boolean available() { return foreground && !destroyed && safeToOffer.getAsBoolean(); }
    boolean visible() { return dialog != null && dialog.isShowing(); }

    /** Called on terminal readiness and after remote interaction; never steals sign-in/IME focus. */
    void maybeOffer() {
        if (!available() || visible()) return;
        if (!checked) check(false);
        else if (offer != null && !dismissed) showOffer();
    }

    /** Long Menu is explicit and remains usable when a previous auto-check failed. */
    void open() {
        if (!available() || visible()) return;
        dismissed = false;
        if (offer != null) showOffer(); else check(true);
    }

    private void check(boolean explicit) {
        long token = state.begin();
        if (token < 0) return;
        checked = true;
        if (explicit) progress("Checking for updates…");
        job = worker.submit(() -> {
            try {
                long installed = activity.getPackageManager().getPackageInfo(activity.getPackageName(), 0)
                        .getLongVersionCode();
                TvUpdateRelease result = client.check(installed);
                main.post(() -> {
                    if (!state.accept(token)) return;
                    state.finish(token);
                    closeDialog();
                    offer = result;
                    if (!available()) return;
                    if (offer != null) showOffer();
                    else if (explicit) message("No newer version found", "Only stable releases containing an Android APK are offered.");
                });
            } catch (Exception error) { failed(token, "Could not check for updates", error, explicit); }
        });
    }

    private void showOffer() {
        if (!available() || visible() || offer == null) return;
        String action = downloaded == null ? "Download and install" : "Install update";
        dialog = builder("nu11signal " + offer.version + " is available")
                .setMessage("Update this app. Android will ask you to confirm installation. Your Apple Music session is kept.")
                .setPositiveButton(action, (d, which) -> { dialog = null; prepare(); })
                .setNegativeButton("Later", (d, which) -> { dismissed = true; dialog = null; })
                .setOnCancelListener(d -> { dismissed = true; dialog = null; }).create();
        dialog.show();
        dialog.getButton(AlertDialog.BUTTON_NEGATIVE).requestFocus();
    }

    private void prepare() {
        if (!available() || offer == null) return;
        if (!installer.canInstall()) {
            dialog = builder("Allow nu11signal updates")
                    .setMessage("In Android Settings, allow this app to install unknown apps, then press Back. You will still confirm each update.")
                    .setPositiveButton("Open Settings", (d, which) -> {
                        dialog = null;
                        try { permissionPending = true; installer.openPermissionSettings(); }
                        catch (RuntimeException error) {
                            permissionPending = false;
                            message("Settings unavailable", "Use Android's app settings to allow installations from nu11signal, then try again.");
                        }
                    }).setNegativeButton("Cancel", (d, which) -> { dismissed = true; dialog = null; })
                    .setOnCancelListener(d -> { dismissed = true; dialog = null; }).create();
            dialog.show();
            return;
        }
        long token = state.begin();
        if (token < 0) return;
        TvUpdateRelease selected = offer;
        File existing = downloaded;
        progress(existing == null ? "Downloading update…" : "Verifying update…");
        job = worker.submit(() -> {
            File apk = existing;
            try {
                File dir = new File(activity.getFilesDir(), "updates").getCanonicalFile();
                if (!dir.isDirectory() && !dir.mkdirs()) throw new IOException("Cannot create private update directory.");
                if (apk == null) {
                    // Only this serial worker creates download files; never touch verified-*
                    // files, which may already be open in Android's package installer.
                    File[] files = dir.listFiles();
                    if (files == null) throw new IOException("Cannot inspect private update directory.");
                    for (File file : files) if (file.getName().startsWith("tv-update-")
                            && (file.getName().endsWith(".apk") || file.getName().endsWith(".part"))) {
                        if (!file.delete()) throw new IOException("Cannot remove a previous download.");
                    }
                    apk = client.download(selected, dir);
                }
                installer.validate(apk, selected.version, selected.versionCode);
                File ready = apk;
                main.post(() -> {
                    // Pause/Back already revoked this attempt. A stale callback must not
                    // clear validation belonging to a newer request.
                    if (!state.accept(token)) return;
                    state.finish(token);
                    downloaded = ready;
                    closeDialog();
                    if (!available()) { installer.clearValidation(); return; }
                    try {
                        // This callback still belongs to the explicit button press; pause,
                        // Back or destruction invalidate its token before any installer launch.
                        dismissed = true;
                        installer.install(ready, selected.version, selected.versionCode);
                    } catch (IOException | RuntimeException error) {
                        message("Could not open installer", safeError(error));
                    }
                });
            } catch (Exception error) {
                installer.clearValidation();
                failed(token, "Update could not be prepared", error, true);
            }
        });
    }

    private void progress(String title) {
        closeDialog();
        dialog = builder(title)
                .setMessage("You can cancel and keep using nu11signal.")
                .setNegativeButton("Cancel", (d, which) -> cancel())
                .setOnCancelListener(d -> cancel()).create();
        dialog.show();
    }

    private void cancel() {
        state.cancel();
        if (job != null) job.cancel(true);
        installer.clearValidation();
        dismissed = true;
        closeDialog();
    }

    private void failed(long token, String title, Exception error, boolean explicit) {
        main.post(() -> {
            if (!state.accept(token)) return;
            state.finish(token);
            closeDialog();
            if (explicit && available()) message(title, safeError(error));
            // An automatic network failure is quiet; long Menu offers a manual retry.
        });
    }

    private static String safeError(Exception error) {
        String value = error.getMessage();
        if (value == null) return "Please try again. You can still update by sideloading the official APK.";
        value = value.replaceAll("[\\p{Cntrl}]", " ");
        return value.substring(0, Math.min(value.length(), 240));
    }

    private void message(String title, String text) {
        if (!available()) return;
        closeDialog();
        dialog = builder(title).setMessage(text)
                .setPositiveButton("OK", (d, which) -> { dialog = null; dismissed = true; })
                .setOnCancelListener(d -> { dialog = null; dismissed = true; }).create();
        dialog.show();
    }

    private void closeDialog() {
        if (dialog != null) { dialog.dismiss(); dialog = null; }
    }

    private AlertDialog.Builder builder(String title) {
        return new AlertDialog.Builder(activity).setIcon(R.drawable.logo_mark).setTitle(title);
    }
}
