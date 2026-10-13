package dev.wahh.nu11signal.tv.updatetest;

import android.app.Activity;
import android.app.Instrumentation;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.Signature;
import android.os.Bundle;
import android.util.Log;
import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.lang.reflect.Constructor;
import java.lang.reflect.Field;
import java.lang.reflect.Proxy;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

/** Test APK only: substitutes transport bytes, never production guard/installer decisions. */
public final class UpdateE2eInstrumentation extends Instrumentation {
    private static final String TARGET = "dev.wahh.nu11signal.tv";
    private static final String PIN = "448148925a1ecad2f51f3f5007343535d22d41716b4f0b9c4d88daaed0494320";
    private static final String API = "https://api.github.com/repos/wahh-22/nu11signal/releases?per_page=30&page=1";
    private static final String APK_URL = "https://github.com/wahh-22/nu11signal/releases/download/v0.0.4/nu11signal-tv-0.0.4.apk";
    private volatile boolean injected, failed;
    private long apkSize;

    @Override public void onCreate(Bundle args) { super.onCreate(args); start(); }

    @Override public void onStart() {
        try {
            PackageInfo installed = getTargetContext().getPackageManager().getPackageInfo(TARGET,
                    PackageManager.GET_SIGNING_CERTIFICATES);
            if (installed.getLongVersionCode() != 3 || !"0.0.3".equals(installed.versionName)
                    || (installed.applicationInfo.flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0)
                throw new IOException("Requires the unchanged signed 0.0.3 release target.");
            Signature[] signers = installed.signingInfo.getApkContentsSigners();
            if (signers.length != 1 || !PIN.equals(hex(MessageDigest.getInstance("SHA-256").digest(signers[0].toByteArray()))))
                throw new IOException("Target signer mismatch.");
            // Count assets off the main thread, before launching the target Activity.
            try (InputStream in = getContext().getAssets().open("update.apk")) {
                byte[] buf = new byte[65536];
                int count;
                while ((count = in.read(buf)) != -1) apkSize += count;
            }
            if (apkSize <= 0 || apkSize > 256L * 1024 * 1024) throw new IOException("Invalid fixture size.");
            startActivitySync(new Intent(Intent.ACTION_MAIN).setClassName(TARGET, TARGET + ".MainActivity")
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
            if (!injected || failed) throw new IOException("Transport fixture was not installed.");
            report("READY: use the real update offer with the remote. Fixture transport serves signed 0.0.4; Android confirmation remains manual.");
            // Do not call finish(): instrumentation and provider stay alive until installation
            // replaces the target process. Runner termination is NOT installation evidence.
        } catch (Exception error) { fail(error); }
    }

    @Override public void callActivityOnCreate(Activity activity, Bundle state) {
        super.callActivityOnCreate(activity, state);
        if (!activity.getClass().getName().equals(TARGET + ".MainActivity")) return;
        try {
            // This hook completes before onResume/terminal callbacks can start discovery.
            // Only target-loader classes are used; no duplicate provider or client in test dex.
            ClassLoader loader = activity.getClassLoader();
            Field updatesField = activity.getClass().getDeclaredField("updates");
            updatesField.setAccessible(true);
            Object ui = updatesField.get(activity);
            Class<?> transportType = loader.loadClass(TARGET + ".TvUpdateClient$Transport");
            Constructor<?> response = loader.loadClass(TARGET + ".TvUpdateClient$Response")
                    .getConstructor(int.class, String.class, long.class, InputStream.class, Runnable.class);
            Object transport = Proxy.newProxyInstance(loader, new Class<?>[]{transportType}, (proxy, method, args) -> {
                if (!"open".equals(method.getName())) throw new UnsupportedOperationException(method.getName());
                String url = (String) args[0];
                InputStream body;
                long length;
                if (API.equals(url)) {
                    String json = "[{\"draft\":false,\"prerelease\":false,\"tag_name\":\"v0.0.4\",\"assets\":[{\"name\":\"nu11signal-tv-0.0.4.apk\",\"size\":"
                            + apkSize + ",\"browser_download_url\":\"" + APK_URL + "\"}]}]";
                    byte[] bytes = json.getBytes(StandardCharsets.UTF_8);
                    body = new ByteArrayInputStream(bytes); length = bytes.length;
                    report("DISCOVERY: supplied isolated release metadata (not a published release).");
                } else if (APK_URL.equals(url)) {
                    body = getContext().getAssets().open("update.apk"); length = apkSize;
                    report("DOWNLOAD: streaming embedded signed APK through production downloader.");
                } else throw new IOException("Fixture refused unexpected request.");
                return response.newInstance(200, null, length, body, (Runnable) () -> { });
            });
            Object client = loader.loadClass(TARGET + ".TvUpdateClient").getConstructor(transportType).newInstance(transport);
            Field field = ui.getClass().getDeclaredField("client");
            field.setAccessible(true);
            field.set(ui, client);
            if (field.get(ui) != client) throw new IOException("Cannot replace test transport.");
            injected = true;
        } catch (Exception error) { fail(error); }
    }

    private void report(String message) {
        Log.i("Nu11UpdateE2E", message);
        Bundle evidence = new Bundle(); evidence.putString("stream", message + "\n"); sendStatus(1, evidence);
    }
    private void fail(Exception error) {
        failed = true;
        String message = "FAILED: " + error.getClass().getSimpleName() + ": " + error.getMessage();
        Log.e("Nu11UpdateE2E", message);
        Bundle result = new Bundle(); result.putString("stream", message); finish(Activity.RESULT_CANCELED, result);
    }
    private static String hex(byte[] data) {
        StringBuilder value = new StringBuilder();
        for (byte b : data) value.append(String.format("%02x", b & 255));
        return value.toString();
    }
}
