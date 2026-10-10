package dev.wahh.nu11signal.tv;

import android.app.Activity;
import android.content.pm.ApplicationInfo;
import android.net.LocalSocket;
import android.net.LocalSocketAddress;
import android.os.Bundle;
import android.os.Message;
import android.os.SystemClock;
import android.system.Os;
import android.util.Base64;
import android.util.Log;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputConnectionWrapper;
import android.view.inputmethod.InputMethodManager;
import android.webkit.ConsoleMessage;
import android.webkit.CookieManager;
import android.webkit.JavascriptInterface;
import android.webkit.PermissionRequest;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.EditText;
import android.widget.FrameLayout;

import org.json.JSONObject;
import java.io.BufferedReader;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.security.SecureRandom;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.RejectedExecutionException;

public class MainActivity extends Activity {
    private static final String TAG = "nu11signal";
    private FrameLayout root;
    private WebView music, terminal, popup;
    private EditText keyboard;
    private boolean loginShown, keyboardShown, started, centerLong, backLong;
    private volatile boolean destroyed, startupFailed, connected;
    private volatile int cols = 120, rows = 34;
    private volatile Process process;
    private volatile LocalSocket socket;
    private volatile OutputStream bridgeOutput;
    // Evaluations sent to the music page and not answered yet.
    private final java.util.Set<Long> pendingEvaluations = java.util.concurrent.ConcurrentHashMap.newKeySet();
    private final ExecutorService bridgeWriter = Executors.newSingleThreadExecutor();
    private final ExecutorService inputWriter = Executors.newSingleThreadExecutor();
    private final String token = newToken();

    private static String newToken() {
        byte[] bytes = new byte[32];
        new SecureRandom().nextBytes(bytes);
        StringBuilder hex = new StringBuilder();
        for (byte b : bytes) hex.append(String.format("%02x", b & 255));
        return hex.toString();
    }

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        WebView.setWebContentsDebuggingEnabled(
                (getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0);
        CookieManager.getInstance().setAcceptCookie(true);
        root = new FrameLayout(this);
        music = newMusicView();
        music.addJavascriptInterface(new MusicBridge(), "NU11");
        root.addView(music, fill());
        terminal = new WebView(this);
        terminal.setBackgroundColor(0xff000000);
        terminal.getSettings().setJavaScriptEnabled(true);
        terminal.getSettings().setAllowFileAccess(false);
        terminal.getSettings().setAllowContentAccess(false);
        terminal.getSettings().setBlockNetworkLoads(true);
        terminal.addJavascriptInterface(new TerminalBridge(), "Android");
        terminal.setWebViewClient(new WebViewClient() {
            private boolean local(String url) { return url.startsWith("file:///android_asset/"); }
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return !local(request.getUrl().toString());
            }
            @Override public boolean shouldOverrideUrlLoading(WebView view, String url) {
                return !local(url);
            }
            @Override public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                return local(request.getUrl().toString()) ? null :
                    new WebResourceResponse("text/plain", "UTF-8", new ByteArrayInputStream(new byte[0]));
            }
        });
        root.addView(terminal, fill());
        keyboard = new EditText(this) {
            @Override public InputConnection onCreateInputConnection(EditorInfo info) {
                InputConnection base = super.onCreateInputConnection(info);
                if (base == null) return null;
                return new InputConnectionWrapper(base, false) {
                    @Override public boolean commitText(CharSequence text, int position) {
                        text.toString().codePoints().forEach(cp -> input(new String(Character.toChars(cp))));
                        getText().clear();
                        return true;
                    }
                    @Override public boolean deleteSurroundingText(int before, int after) {
                        for (int i = 0; i < before; i++) input("\u007f");
                        return true;
                    }
                    @Override public boolean performEditorAction(int action) {
                        input("\r");
                        return true;
                    }
                    @Override public boolean sendKeyEvent(KeyEvent event) {
                        return MainActivity.this.dispatchKeyEvent(event);
                    }
                };
            }
        };
        keyboard.setSingleLine(true);
        keyboard.setInputType(android.text.InputType.TYPE_CLASS_TEXT | android.text.InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS);
        keyboard.setImeOptions(EditorInfo.IME_ACTION_SEND | EditorInfo.IME_FLAG_NO_EXTRACT_UI);
        keyboard.setAlpha(0f);
        root.addView(keyboard, new FrameLayout.LayoutParams(1, 1));
        setContentView(root);
        music.loadUrl("https://music.apple.com/");
        terminal.loadUrl("file:///android_asset/terminal.html");
        terminal.requestFocus();
    }

    private static FrameLayout.LayoutParams fill() {
        return new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT);
    }

    private WebView newMusicView() {
        WebView view = new WebView(this);
        WebSettings settings = view.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setMediaPlaybackRequiresUserGesture(false);
        settings.setLoadWithOverviewMode(true);
        settings.setUseWideViewPort(true);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSupportMultipleWindows(true);
        settings.setJavaScriptCanOpenWindowsAutomatically(true);
        CookieManager.getInstance().setAcceptThirdPartyCookies(view, true);
        view.setFocusable(true);
        view.setFocusableInTouchMode(true);
        view.setWebViewClient(new WebViewClient() {
            @Override public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest request) {
                return !"https".equalsIgnoreCase(request.getUrl().getScheme());
            }
            // A new document drops the old one's pending promises without
            // settling them; fail those evaluations, as CDP reports a
            // destroyed context, so Go polls again instead of hanging.
            @Override public void onPageStarted(WebView v, String url, android.graphics.Bitmap favicon) {
                if (v == music) failPendingEvaluations("page navigated");
            }
        });
        view.setWebChromeClient(new WebChromeClient() {
            @Override public boolean onConsoleMessage(ConsoleMessage message) {
                Log.i(TAG, message.messageLevel() + ": " + message.message());
                return true;
            }
            @Override public void onPermissionRequest(PermissionRequest request) {
                for (String resource : request.getResources()) {
                    if (PermissionRequest.RESOURCE_PROTECTED_MEDIA_ID.equals(resource)) {
                        request.grant(new String[]{PermissionRequest.RESOURCE_PROTECTED_MEDIA_ID});
                        return;
                    }
                }
                request.deny();
            }
            @Override public boolean onCreateWindow(WebView v, boolean dialog, boolean gesture, Message result) {
                closePopup();
                popup = newMusicView();
                root.addView(popup, fill());
                popup.requestFocus();
                ((WebView.WebViewTransport) result.obj).setWebView(popup);
                result.sendToTarget();
                return true;
            }
            @Override public void onCloseWindow(WebView window) { if (window == popup) closePopup(); }
        });
        return view;
    }

    private void closePopup() {
        if (popup == null) return;
        root.removeView(popup);
        popup.destroy();
        popup = null;
        (loginShown ? music : terminal).requestFocus();
    }

    private void showLogin(boolean show) {
        if (show && keyboardShown) toggleKeyboard();
        if (!show) closePopup();
        loginShown = show;
        // Keep the music WebView running underneath the opaque terminal.
        terminal.setVisibility(show ? View.GONE : View.VISIBLE);
        (show ? music : terminal).requestFocus();
    }

    public final class TerminalBridge {
        @JavascriptInterface public void ready() {
            runOnUiThread(() -> {
                if (started || destroyed) return;
                started = true;
                new Thread(MainActivity.this::startProcess, "go-start").start();
            });
        }
        @JavascriptInterface public void input(String text) { MainActivity.this.input(text); }
        @JavascriptInterface public void resize(int columns, int lines) {
            if (columns <= 0 || columns > 65535 || lines <= 0 || lines > 65535) return;
            cols = columns; rows = lines;
            sendResize();
        }
    }

    private void failPendingEvaluations(String reason) {
        for (Long id : pendingEvaluations.toArray(new Long[0])) {
            if (!pendingEvaluations.remove(id)) continue;
            try { send(new JSONObject().put("type", "result").put("id", id).put("ok", false).put("error", reason)); }
            catch (Exception e) { Log.w(TAG, "Cannot fail evaluation", e); }
        }
    }

    public final class MusicBridge {
        @JavascriptInterface public void settle(String suppliedToken, long id, String json) {
            if (!token.equals(suppliedToken) || id <= 0 || destroyed) return;
            // Each evaluation is answered once: here or by a navigation.
            if (!pendingEvaluations.remove(id)) return;
            try {
                JSONObject result = new JSONObject(json);
                boolean ok = result.getBoolean("ok");
                JSONObject reply = new JSONObject().put("type", "result").put("id", id).put("ok", ok);
                if (ok) reply.put("value", result.opt("value") == null ? JSONObject.NULL : result.opt("value"));
                else reply.put("error", result.optString("error", "JavaScript error"));
                send(reply);
            } catch (Exception e) { Log.w(TAG, "Invalid evaluation result", e); }
        }
    }

    private void startProcess() {
        try {
            File home = new File(getFilesDir(), "home"), config = new File(getFilesDir(), "config"), run = new File(getFilesDir(), "run");
            for (File dir : new File[]{home, config, run}) {
                if (!dir.isDirectory() && !dir.mkdirs()) throw new java.io.IOException("Cannot create " + dir.getName());
            }
            Os.chmod(run.getAbsolutePath(), 0700);
            String path = new File(run, "bridge.sock").getAbsolutePath();
            ProcessBuilder builder = new ProcessBuilder(getApplicationInfo().nativeLibraryDir + "/libnu11signal.so", "--android-tv", path);
            builder.environment().put("HOME", home.getAbsolutePath());
            builder.environment().put("XDG_CONFIG_HOME", config.getAbsolutePath());
            builder.environment().put("TMPDIR", getCacheDir().getAbsolutePath());
            builder.environment().put("TERM", "xterm-256color");
            builder.environment().put("COLORTERM", "truecolor");
            Process child = builder.start();
            process = child;
            if (destroyed) { child.destroy(); return; }
            writeTerminal(("nu11signal: loading Apple Music, this can take a minute on a TV...\r\n").getBytes(StandardCharsets.UTF_8));
            Thread output = new Thread(() -> readTerminal(child.getInputStream()), "go-output");
            output.start();
            new Thread(() -> {
                try (BufferedReader reader = new BufferedReader(new InputStreamReader(child.getErrorStream(), StandardCharsets.UTF_8))) {
                    String line;
                    while ((line = reader.readLine()) != null) Log.e(TAG, line);
                } catch (Exception e) { if (!destroyed) Log.w(TAG, "stderr closed", e); }
            }, "go-stderr").start();
            new Thread(() -> {
                try {
                    int exit = child.waitFor();
                    output.join();
                    if (!destroyed && connected && !startupFailed) {
                        if (exit == 0) runOnUiThread(() -> { if (!destroyed) finish(); });
                        else error("nu11signal exited with status " + exit + ". See logcat tag nu11signal.");
                    }
                } catch (InterruptedException e) { Thread.currentThread().interrupt(); }
            }, "go-exit").start();
            long deadline = SystemClock.elapsedRealtime() + 20000;
            LocalSocket accepted = null;
            while (!destroyed && SystemClock.elapsedRealtime() < deadline) {
                LocalSocket attempt = new LocalSocket();
                socket = attempt;
                if (destroyed) { attempt.close(); return; }
                try {
                    attempt.connect(new LocalSocketAddress(path, LocalSocketAddress.Namespace.FILESYSTEM));
                    accepted = attempt;
                    break;
                } catch (java.io.IOException e) {
                    attempt.close();
                    if (!child.isAlive()) throw new java.io.IOException("Go exited before the bridge connected; see logcat tag nu11signal.");
                    Thread.sleep(100);
                }
            }
            if (destroyed) { if (accepted != null) accepted.close(); return; }
            if (accepted == null) throw new java.io.IOException("Bridge connection timed out after 20 seconds.");
            bridgeOutput = accepted.getOutputStream();
            connected = true;
            sendResize();
            readBridge(accepted);
        } catch (Exception e) {
            if (!destroyed) {
                startupFailed = true;
                error("Cannot run nu11signal: " + e.getMessage());
                Process child = process;
                if (child != null) child.destroy();
            }
        }
    }

    private void readBridge(LocalSocket connection) throws Exception {
        try (BufferedReader reader = new BufferedReader(new InputStreamReader(connection.getInputStream(), StandardCharsets.UTF_8))) {
            String line;
            while (!destroyed && (line = reader.readLine()) != null) {
                JSONObject message = new JSONObject(line);
                String type = message.getString("type");
                if ("login".equals(type)) {
                    boolean show = message.getBoolean("show");
                    runOnUiThread(() -> { if (!destroyed) showLogin(show); });
                } else if ("evaluate".equals(type)) {
                    long id = message.getLong("id");
                    String expr = message.getString("expr");
                    pendingEvaluations.add(id);
                    String script = "(async()=>{try{const v=await (" + expr + ");NU11.settle(" + JSONObject.quote(token) + "," + id
                        + ",JSON.stringify({ok:true,value:v===undefined?null:v}));}catch(e){NU11.settle(" + JSONObject.quote(token) + "," + id
                        + ",JSON.stringify({ok:false,error:String((e&&e.message)||e)}));}})();void 0";
                    runOnUiThread(() -> { if (!destroyed) music.evaluateJavascript(script, null); });
                }
            }
        }
    }

    private void sendResize() {
        try { send(new JSONObject().put("type", "resize").put("cols", cols).put("rows", rows)); }
        catch (Exception e) { Log.w(TAG, "Resize failed", e); }
    }

    private void submit(ExecutorService executor, Runnable task) {
        if (destroyed) return;
        try { executor.execute(task); } catch (RejectedExecutionException ignored) { /* Activity is closing. */ }
    }

    private void send(JSONObject message) {
        if (bridgeOutput == null) return;
        byte[] data = (message.toString() + "\n").getBytes(StandardCharsets.UTF_8);
        submit(bridgeWriter, () -> {
            try { if (!destroyed) { bridgeOutput.write(data); bridgeOutput.flush(); } }
            catch (Exception e) { if (!destroyed) error("Bridge write failed: " + e.getMessage()); }
        });
    }

    private void input(String text) {
        byte[] data = text.getBytes(StandardCharsets.UTF_8);
        submit(inputWriter, () -> {
            try {
                Process child = process;
                if (child != null && !destroyed) { child.getOutputStream().write(data); child.getOutputStream().flush(); }
            } catch (Exception e) { if (!destroyed) Log.w(TAG, "Input pipe closed", e); }
        });
    }

    private void readTerminal(InputStream stream) {
        // Block for the first bytes; collect available bytes for at most 16 ms / 32 KiB.
        byte[] buffer = new byte[32768];
        try {
            int count;
            while (!destroyed && (count = stream.read(buffer)) != -1) {
                ByteArrayOutputStream batch = new ByteArrayOutputStream(32768);
                batch.write(buffer, 0, count);
                long deadline = SystemClock.elapsedRealtime() + 16;
                while (!destroyed && batch.size() < 32768 && SystemClock.elapsedRealtime() < deadline) {
                    int available = stream.available();
                    if (available == 0) { Thread.sleep(1); continue; }
                    int n = stream.read(buffer, 0, Math.min(available, 32768 - batch.size()));
                    if (n < 0) break;
                    batch.write(buffer, 0, n);
                }
                writeTerminal(batch.toByteArray());
            }
        } catch (Exception e) { if (!destroyed) Log.w(TAG, "Output pipe closed", e); }
    }

    private void writeTerminal(byte[] bytes) {
        String base64 = Base64.encodeToString(bytes, Base64.NO_WRAP);
        runOnUiThread(() -> { if (!destroyed) terminal.evaluateJavascript("window.nu11Write('" + base64 + "')", null); });
    }

    private void error(String message) {
        Log.e(TAG, message);
        // Remove control characters so error descriptions cannot inject terminal escapes.
        writeTerminal(("\r\n" + message.replaceAll("[\\p{Cntrl}]", " ") + "\r\n").getBytes(StandardCharsets.UTF_8));
    }

    private void toggleKeyboard() {
        InputMethodManager ime = (InputMethodManager) getSystemService(INPUT_METHOD_SERVICE);
        keyboardShown = !keyboardShown;
        if (keyboardShown) { keyboard.requestFocus(); ime.showSoftInput(keyboard, InputMethodManager.SHOW_IMPLICIT); }
        else { ime.hideSoftInputFromWindow(keyboard.getWindowToken(), 0); terminal.requestFocus(); }
    }

    @Override public boolean dispatchKeyEvent(KeyEvent event) {
        int key = event.getKeyCode();
        if (loginShown || popup != null) {
            if (key == KeyEvent.KEYCODE_BACK) {
                if (event.getAction() == KeyEvent.ACTION_UP) onBackPressed();
                return true;
            }
            return super.dispatchKeyEvent(event);
        }
        if (event.getAction() == KeyEvent.ACTION_MULTIPLE && event.getCharacters() != null) {
            input(event.getCharacters());
            return true;
        }
        if (key == KeyEvent.KEYCODE_BACK || key == KeyEvent.KEYCODE_DPAD_CENTER) {
            if (event.getAction() == KeyEvent.ACTION_DOWN) {
                if (event.getRepeatCount() == 0) { if (key == KeyEvent.KEYCODE_BACK) backLong = false; else centerLong = false; }
                if (event.isLongPress() || event.getEventTime() - event.getDownTime() >= android.view.ViewConfiguration.getLongPressTimeout()) {
                    if (key == KeyEvent.KEYCODE_BACK && !backLong) { backLong = true; if (keyboardShown) toggleKeyboard(); else input("q"); }
                    if (key == KeyEvent.KEYCODE_DPAD_CENTER && !centerLong) { centerLong = true; toggleKeyboard(); }
                }
            } else if (event.getAction() == KeyEvent.ACTION_UP && !event.isCanceled()) {
                boolean held = event.getEventTime() - event.getDownTime() >= android.view.ViewConfiguration.getLongPressTimeout();
                if (key == KeyEvent.KEYCODE_BACK && !backLong) { if (keyboardShown) toggleKeyboard(); else input(held ? "q" : "\u001b"); }
                if (key == KeyEvent.KEYCODE_DPAD_CENTER && !centerLong) { if (held) toggleKeyboard(); else input("\r"); }
            }
            return true;
        }
        String text = null;
        switch (key) {
            case KeyEvent.KEYCODE_DPAD_UP: text = "\u001b[A"; break;
            case KeyEvent.KEYCODE_DPAD_DOWN: text = "\u001b[B"; break;
            case KeyEvent.KEYCODE_DPAD_RIGHT: text = "\u001b[C"; break;
            case KeyEvent.KEYCODE_DPAD_LEFT: text = "\u001b[D"; break;
            case KeyEvent.KEYCODE_ENTER: case KeyEvent.KEYCODE_NUMPAD_ENTER: text = "\r"; break;
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE: case KeyEvent.KEYCODE_MEDIA_PLAY: case KeyEvent.KEYCODE_MEDIA_PAUSE: text = " "; break;
            case KeyEvent.KEYCODE_MEDIA_NEXT: case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD: text = "n"; break;
            case KeyEvent.KEYCODE_MEDIA_PREVIOUS: case KeyEvent.KEYCODE_MEDIA_REWIND: text = "p"; break;
            case KeyEvent.KEYCODE_MENU: text = "/"; break;
            case KeyEvent.KEYCODE_DEL: text = "\u007f"; break;
            case KeyEvent.KEYCODE_ESCAPE: text = "\u001b"; break;
            default:
                int unicode = event.getUnicodeChar();
                if (unicode > 0 && (unicode & android.view.KeyCharacterMap.COMBINING_ACCENT) == 0) text = new String(Character.toChars(unicode));
        }
        if (text != null) { if (event.getAction() == KeyEvent.ACTION_DOWN) input(text); return true; }
        // Let system volume/power keys through, but never deliver raw keys to xterm.
        return key == KeyEvent.KEYCODE_VOLUME_UP || key == KeyEvent.KEYCODE_VOLUME_DOWN || key == KeyEvent.KEYCODE_VOLUME_MUTE
                ? super.dispatchKeyEvent(event) : true;
    }

    @Override public void onBackPressed() {
        if (popup != null) { closePopup(); return; }
        if (loginShown) {
            showLogin(false);
            try { send(new JSONObject().put("type", "login").put("closed", true)); }
            catch (Exception e) { Log.w(TAG, "Login close failed", e); }
        } else if (keyboardShown) toggleKeyboard();
        else input("\u001b");
    }

    @Override protected void onDestroy() {
        destroyed = true;
        LocalSocket connection = socket;
        if (connection != null) try { connection.close(); } catch (Exception ignored) { }
        Process child = process;
        if (child != null) {
            // Terminate first so any blocking pipe reader/writer can unblock.
            child.destroy();
            try { child.getOutputStream().close(); child.getInputStream().close(); child.getErrorStream().close(); }
            catch (Exception ignored) { }
        }
        bridgeWriter.shutdownNow();
        inputWriter.shutdownNow();
        closePopup();
        terminal.destroy();
        music.destroy();
        super.onDestroy();
    }
}
