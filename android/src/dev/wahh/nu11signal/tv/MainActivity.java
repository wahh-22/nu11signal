package dev.wahh.nu11signal.tv;

import android.app.Activity;
import android.Manifest;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.media.audiofx.Visualizer;
import android.content.pm.ApplicationInfo;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Rect;
import android.net.LocalSocket;
import android.net.LocalSocketAddress;
import android.os.Build;
import android.os.Bundle;
import android.os.Message;
import android.os.SystemClock;
import android.system.Os;
import android.util.Base64;
import android.util.Log;
import android.text.Editable;
import android.text.TextWatcher;
import android.view.Gravity;
import android.view.InputDevice;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.view.WindowInsets;
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
import android.widget.LinearLayout;
import android.widget.TextView;

import org.json.JSONObject;
import org.json.JSONArray;
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
    private TvUpdateUi updates;
    private boolean menuLong;
    private WebView music, terminal, popup;
    private EditText keyboard;
    private String keyboardTextSent = "";
    private boolean resettingKeyboard, keyboardWasVisible, keyboardAfterSearch;
    private View pointer;
    private TextView pointerHint, loginConfirmation;
    private LinearLayout loginGuide;
    private final Runnable hideLoginConfirmation = () -> loginConfirmation.setVisibility(View.GONE);
    private float pointerX, pointerY;
    private WebView touchTarget;
    private long touchDownTime;
    private float touchX, touchY;
    private Runnable touchContinuation;
    private boolean loginShown, keyboardShown, textInputAvailable, started, centerLong, backLong;
    // A phone or tablet (no leanback UI): touch input and an automatic keyboard.
    private boolean touchDevice;
    private volatile boolean destroyed, startupFailed, connected, terminalVisible;
    private volatile int cols = 161, rows = 44;
    private volatile Process process;
    private volatile LocalSocket socket;
    private volatile OutputStream bridgeOutput;
    // Evaluations sent to the music page and not answered yet.
    private final java.util.Set<Long> pendingEvaluations = java.util.concurrent.ConcurrentHashMap.newKeySet();
    private final ExecutorService bridgeWriter = Executors.newSingleThreadExecutor();
    private final ExecutorService inputWriter = Executors.newSingleThreadExecutor();
    private final String token = newToken();
    private static final int MAX_PENDING_TERMINAL_BYTES = 256 * 1024;
    private final Object terminalOutputLock = new Object();
    private final ByteArrayOutputStream pendingTerminalOutput = new ByteArrayOutputStream(32768);
    // Includes a posted flush, so there is never more than one evaluation queued or running.
    private boolean terminalEvaluationPending;
    private static final int LEVELS_PERMISSION = 11;
    private Visualizer visualizer;
    private boolean levelsWanted, capturePaused = true, permissionAsked, captureFailed;
    private JSONArray capturedWave;
    private final double[] smoothBands = new double[24];
    private long lastCapture;

    private static String newToken() {
        byte[] bytes = new byte[32];
        new SecureRandom().nextBytes(bytes);
        StringBuilder hex = new StringBuilder();
        for (byte b : bytes) hex.append(String.format("%02x", b & 255));
        return hex.toString();
    }

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        PlaybackService.attach(this);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        WebView.setWebContentsDebuggingEnabled(
                (getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0);
        CookieManager.getInstance().setAcceptCookie(true);
        root = new FrameLayout(this);
        music = newMusicView();
        music.addJavascriptInterface(new MusicBridge(), "NU11");
        root.addView(music, fill());
        touchDevice = !isTelevision();
        terminal = new WebView(this);
        terminal.setBackgroundColor(0xff000000);
        // On a touch device a tap is mouse input to the TUI (terminal.html);
        // it must not take focus from the keyboard while typing.
        if (touchDevice) terminal.setFocusableInTouchMode(false);
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
            @Override public boolean onKeyPreIme(int key, KeyEvent event) {
                // Consume both halves: the first Back only dismisses the IME,
                // and clears our state even when Android intercepts Back.
                if (key == KeyEvent.KEYCODE_BACK && keyboardShown) {
                    if (event.getAction() == KeyEvent.ACTION_UP) setKeyboard(false);
                    return true;
                }
                return super.onKeyPreIme(key, event);
            }
            @Override public InputConnection onCreateInputConnection(EditorInfo info) {
                InputConnection base = super.onCreateInputConnection(info);
                if (base == null) return null;
                return new InputConnectionWrapper(base, false) {
                    @Override public boolean deleteSurroundingText(int before, int after) {
                        if (getText().length() == 0 && before > 0) {
                            for (int i = 0; i < before; i++) input("\u007f");
                            return true;
                        }
                        return super.deleteSurroundingText(before, after);
                    }
                    @Override public boolean deleteSurroundingTextInCodePoints(int before, int after) {
                        if (getText().length() == 0 && before > 0) {
                            for (int i = 0; i < before; i++) input("\u007f");
                            return true;
                        }
                        return super.deleteSurroundingTextInCodePoints(before, after);
                    }
                    @Override public boolean performEditorAction(int action) {
                        submitKeyboard();
                        return true;
                    }
                    @Override public boolean sendKeyEvent(KeyEvent event) {
                        return MainActivity.this.dispatchKeyEvent(event);
                    }
                };
            }
        };
        keyboard.setSingleLine(true);
        keyboard.setInputType(android.text.InputType.TYPE_CLASS_TEXT);
        keyboard.addTextChangedListener(new TextWatcher() {
            @Override public void beforeTextChanged(CharSequence s, int start, int count, int after) { }
            @Override public void onTextChanged(CharSequence s, int start, int before, int count) {
                if (resettingKeyboard) return;
                String next = s.toString();
                input(KeyboardEdits.diff(keyboardTextSent, next));
                keyboardTextSent = next;
            }
            @Override public void afterTextChanged(Editable text) { }
        });
        keyboard.setImeOptions(EditorInfo.IME_ACTION_SEND | EditorInfo.IME_FLAG_NO_EXTRACT_UI);
        keyboard.setAlpha(0f);
        root.addView(keyboard, new FrameLayout.LayoutParams(1, 1));
        pointer = new View(this) {
            private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
            @Override protected void onDraw(Canvas canvas) {
                paint.setStyle(Paint.Style.STROKE);
                paint.setColor(0xff000000);
                paint.setStrokeWidth(dp(6));
                canvas.drawCircle(pointerX, pointerY, dp(13), paint);
                paint.setColor(0xffffffff);
                paint.setStrokeWidth(dp(3));
                canvas.drawCircle(pointerX, pointerY, dp(13), paint);
            }
        };
        pointer.setFocusable(false);
        pointer.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
        pointer.setVisibility(View.GONE);
        root.addView(pointer, fill());
        pointerHint = new TextView(this);
        pointerHint.setText(touchDevice
                ? "Tap Sign In at the top right of the page · Back returns to nu11signal"
                : "Arrows move the pointer · OK selects · Back returns to nu11signal");
        pointerHint.setTextColor(0xffffffff);
        pointerHint.setTextSize(20);
        pointerHint.setBackgroundColor(0xb3000000);
        pointerHint.setPadding((int) dp(12), (int) dp(6), (int) dp(12), (int) dp(6));
        pointerHint.setFocusable(false);
        pointerHint.setVisibility(View.GONE);
        // On a touch device the hint sits at the bottom: at the top it covers
        // Apple's Sign In button, which the user taps directly.
        root.addView(pointerHint, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT,
                (touchDevice ? Gravity.BOTTOM : Gravity.TOP) | Gravity.CENTER_HORIZONTAL));
        loginGuide = new LinearLayout(this);
        loginGuide.setOrientation(LinearLayout.VERTICAL);
        loginGuide.setGravity(Gravity.CENTER_VERTICAL);
        loginGuide.setPadding((int) dp(32), (int) dp(24), (int) dp(32), (int) dp(24));
        loginGuide.setBackgroundColor(0xff101010);
        loginGuide.setFocusableInTouchMode(true);
        loginGuide.setClickable(true);
        addGuideText("Sign in to Apple Music", 40);
        addGuideText("nu11signal plays your Apple Music through Apple's web player. Sign in once on this TV; it stays signed in.", 26);
        addGuideText("1. Use the arrows to move the white circle to Sign In (top right of the page), then press OK.", 26);
        addGuideText("2. In the Apple ID window, move the circle to each field and press OK to open the keyboard. Type your Apple ID, password and verification code.", 26);
        addGuideText("3. When you finish, nu11signal returns by itself.", 26);
        addGuideText("Press OK to start · Back to skip for now", 26);
        loginGuide.setVisibility(View.GONE);
        root.addView(loginGuide, fill());
        loginConfirmation = new TextView(this);
        loginConfirmation.setText("Signed in to Apple Music");
        loginConfirmation.setTextSize(32);
        loginConfirmation.setTextColor(0xffffffff);
        loginConfirmation.setBackgroundColor(0xd9000000);
        loginConfirmation.setGravity(Gravity.CENTER);
        loginConfirmation.setPadding((int) dp(24), (int) dp(16), (int) dp(24), (int) dp(16));
        loginConfirmation.setFocusable(false);
        loginConfirmation.setVisibility(View.GONE);
        root.addView(loginConfirmation, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, Gravity.CENTER));
        root.addOnLayoutChangeListener((v, left, top, right, bottom, oldLeft, oldTop, oldRight, oldBottom) -> {
            pointerX = Math.max(0, Math.min(pointerX, Math.max(0, root.getWidth() - 1)));
            pointerY = Math.max(0, Math.min(pointerY, Math.max(0, root.getHeight() - 1)));
            pointer.invalidate();
        });
        setContentView(root);
        root.getViewTreeObserver().addOnGlobalLayoutListener(() -> {
            if (!keyboardShown) { keyboardWasVisible = false; return; }
            if (loginImeVisible()) keyboardWasVisible = true;
            else if (keyboardWasVisible) setKeyboard(false);
        });
        updateMusicLayout();
        music.loadUrl("https://music.apple.com/");
        // A TV fits the desktop's 161x44 grid to the screen; a phone or
        // tablet keeps a readable text size and takes the grid that fits.
        terminal.loadUrl("file:///android_asset/terminal.html" + (touchDevice ? "?touch=1" : ""));
        terminal.requestFocus();
        updates = new TvUpdateUi(this, () -> started && !destroyed && !loginShown
                && popup == null && !guideVisible()
                && TvUpdateState.inputIdle(keyboardShown, keyboardAfterSearch));
    }

    /** True on a TV (leanback UI); phones and tablets are touch devices. */
    private boolean isTelevision() {
        android.app.UiModeManager ui = getSystemService(android.app.UiModeManager.class);
        return (ui != null && ui.getCurrentModeType() == Configuration.UI_MODE_TYPE_TELEVISION)
                || getPackageManager().hasSystemFeature(PackageManager.FEATURE_LEANBACK);
    }

    private void addGuideText(String text, int size) {
        TextView label = new TextView(this);
        label.setText(text);
        label.setTextSize(size);
        label.setTextColor(0xffffffff);
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        params.bottomMargin = (int) dp(12);
        loginGuide.addView(label, params);
    }

    private boolean guideVisible() { return loginGuide.getVisibility() == View.VISIBLE; }

    private void startLoginPointer() {
        loginGuide.setVisibility(View.GONE);
        pointer.setVisibility(View.VISIBLE);
        pointerHint.setVisibility(View.VISIBLE);
        raisePointer();
        (popup != null ? popup : music).requestFocus();
    }

    private void confirmLogin() {
        root.removeCallbacks(hideLoginConfirmation);
        loginConfirmation.setVisibility(View.VISIBLE);
        loginConfirmation.bringToFront();
        root.postDelayed(hideLoginConfirmation, 4000);
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
            // Apple Music's pages play animated album art: silent HLS video
            // that nobody sees in this hidden page, and that kept a hardware
            // decoder and the CPU busy for many seconds after startup.
            @Override public WebResourceResponse shouldInterceptRequest(WebView v, WebResourceRequest request) {
                if (!MotionArtwork.matches(request.getUrl().getHost(), request.getUrl().getPath())) return null;
                return new WebResourceResponse("text/plain", "UTF-8", 403, "Forbidden",
                        java.util.Collections.emptyMap(), new ByteArrayInputStream(new byte[0]));
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
                updateMusicLayout();
                raisePointer();
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
        cancelPointerTouch();
        root.removeView(popup);
        popup.destroy();
        popup = null;
        updateMusicLayout();
        (loginShown ? music : terminal).requestFocus();
    }

    private void showLogin(boolean show) {
        if (show && keyboardShown) toggleKeyboard();
        if (!show) closePopup();
        boolean entering = show && !loginShown;
        loginShown = show;
        if (!show) cancelPointerTouch();
        if (show) {
            root.removeCallbacks(hideLoginConfirmation);
            loginConfirmation.setVisibility(View.GONE);
        }
        if (!show) loginGuide.setVisibility(View.GONE);
        if (entering && !touchDevice) {
            // A phone or tablet touches Apple's page directly: no guide, no pointer.
            loginGuide.setVisibility(View.VISIBLE);
            centerLong = false;
            pointerX = root.getWidth() / 2f;
            pointerY = root.getHeight() / 2f;
            pointer.invalidate();
        }
        pointer.setVisibility(show && !guideVisible() && !touchDevice ? View.VISIBLE : View.GONE);
        pointerHint.setVisibility(show && !guideVisible() ? View.VISIBLE : View.GONE);
        raisePointer();
        updateMusicLayout();
        terminal.setVisibility(show ? View.GONE : View.VISIBLE);
        (guideVisible() ? loginGuide : show ? music : terminal).requestFocus();
    }

    private void updateMusicLayout() {
        boolean fullScreen = loginShown || guideVisible() || popup != null;
        // Stay attached and VISIBLE for MusicKit/audio, but avoid compositing an animated
        // full-screen Apple page underneath the opaque TUI. Never pause the WebView.
        music.setLayoutParams(fullScreen ? fill() :
                new FrameLayout.LayoutParams(1, 1, Gravity.TOP | Gravity.LEFT));
    }

    private float dp(float value) { return value * getResources().getDisplayMetrics().density; }

    private void raisePointer() {
        if (pointer != null) pointer.bringToFront();
        if (pointerHint != null) pointerHint.bringToFront();
        if (loginGuide != null && guideVisible()) loginGuide.bringToFront();
    }

    private boolean loginImeVisible() {
        if (Build.VERSION.SDK_INT >= 30) {
            WindowInsets insets = root.getRootWindowInsets();
            if (insets != null) return insets.isVisible(WindowInsets.Type.ime());
        }
        // API 28/29: visible-frame shrink also covers adjustPan keyboards.
        Rect visible = new Rect();
        root.getWindowVisibleDisplayFrame(visible);
        return root.getRootView().getHeight() - visible.bottom > dp(100);
    }

    private void movePointer(int key, int repeats) {
        float step = dp(Math.min(48, 8 + repeats * 4));
        float maxX = Math.max(0, root.getWidth() - 1), maxY = Math.max(0, root.getHeight() - 1);
        float dy = key == KeyEvent.KEYCODE_DPAD_UP ? -step : key == KeyEvent.KEYCODE_DPAD_DOWN ? step : 0;
        boolean edge = dy < 0 && pointerY == 0 || dy > 0 && pointerY == maxY;
        if (key == KeyEvent.KEYCODE_DPAD_LEFT) pointerX -= step;
        if (key == KeyEvent.KEYCODE_DPAD_RIGHT) pointerX += step;
        pointerX = Math.max(0, Math.min(pointerX, maxX));
        pointerY = Math.max(0, Math.min(pointerY + dy, maxY));
        pointer.invalidate();
        if (edge) startPointerTouch(dy);
    }

    private void dispatchPointerTouch(int action) {
        MotionEvent touch = MotionEvent.obtain(touchDownTime, SystemClock.uptimeMillis(), action, touchX, touchY, 0);
        touch.setSource(InputDevice.SOURCE_TOUCHSCREEN);
        touchTarget.dispatchTouchEvent(touch);
        touch.recycle();
    }

    private void cancelPointerTouch() {
        if (touchContinuation != null) root.removeCallbacks(touchContinuation);
        touchContinuation = null;
        if (touchTarget != null) {
            dispatchPointerTouch(MotionEvent.ACTION_CANCEL);
            touchTarget = null;
        }
    }

    private void startPointerTouch(float scroll) {
        if (touchTarget != null || !loginShown || destroyed) return;
        WebView target = popup != null ? popup : music;
        int[] rootLocation = new int[2], targetLocation = new int[2];
        root.getLocationOnScreen(rootLocation);
        target.getLocationOnScreen(targetLocation);
        float x = pointerX + rootLocation[0] - targetLocation[0];
        float y = pointerY + rootLocation[1] - targetLocation[1];
        float distance = Math.min(dp(48) + Math.abs(scroll), target.getHeight() / 3f);
        if (scroll != 0) {
            // Drag inside the page, not scrollBy: DOM inner scroll containers
            // receive native touch scrolling too. Finger motion opposes scrolling.
            y = scroll > 0 ? target.getHeight() - 1 - distance - dp(8) : distance + dp(8);
        }
        touchX = Math.max(0, Math.min(x, target.getWidth() - 1));
        touchY = Math.max(0, Math.min(y, target.getHeight() - 1));
        final float startY = touchY;
        touchTarget = target;
        touchDownTime = SystemClock.uptimeMillis();
        target.requestFocus();
        dispatchPointerTouch(MotionEvent.ACTION_DOWN);
        touchContinuation = new Runnable() {
            private int frame;
            @Override public void run() {
                if (touchTarget != target) return;
                if (scroll != 0 && frame < 4) {
                    frame++;
                    touchY = startY + (scroll > 0 ? -distance : distance) * frame / 4;
                    dispatchPointerTouch(MotionEvent.ACTION_MOVE);
                    root.postDelayed(this, 30);
                } else {
                    dispatchPointerTouch(MotionEvent.ACTION_UP);
                    touchTarget = null;
                    touchContinuation = null;
                }
            }
        };
        root.postDelayed(touchContinuation, scroll == 0 ? 50 : 30);
    }

    public final class TerminalBridge {
        @JavascriptInterface public void ready() {
            runOnUiThread(() -> {
                if (started || destroyed) return;
                started = true;
                new Thread(MainActivity.this::startProcess, "go-start").start();
                if (updates != null) updates.maybeOffer();
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
            // Go keeps UTC on Android; it loads this zone for the clock.
            builder.environment().put("TZ", java.util.TimeZone.getDefault().getID());
            Process child = builder.start();
            process = child;
            if (destroyed) { child.destroy(); return; }
            // Go draws the boot splash itself while Apple Music loads.
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
                    runOnUiThread(() -> {
                        if (destroyed) return;
                        boolean signedIn = !show && loginShown;
                        showLogin(show);
                        if (signedIn) confirmLogin();
                    });
                } else if ("nowplaying".equals(type)) {
                    runOnUiThread(() -> { if (!destroyed) PlaybackService.update(message); });
                } else if ("levels".equals(type)) {
                    boolean on = message.getBoolean("on");
                    runOnUiThread(() -> { levelsWanted = on; updateCapture(); });
                } else if ("keyboard".equals(type)) {
                    boolean show = message.getBoolean("show");
                    runOnUiThread(() -> {
                        if (destroyed) return;
                        // This is TUI input availability, not an IME-open request,
                        // except on a touch device, which has no OK key to ask.
                        textInputAvailable = show;
                        if (!show) setKeyboard(false);
                        else if (touchDevice || keyboardAfterSearch) {
                            keyboardAfterSearch = false;
                            setKeyboard(true);
                        }
                    });
                } else if ("cancel".equals(type)) {
                    // Remove on the socket reader, before queued UI tasks run.
                    // Running MusicKit promises cannot be aborted; settle drops their replies.
                    pendingEvaluations.remove(message.getLong("id"));
                } else if ("evaluate".equals(type)) {
                    long id = message.getLong("id");
                    String expr = message.getString("expr");
                    pendingEvaluations.add(id);
                    String script = "(async()=>{try{const v=await (" + expr + ");NU11.settle(" + JSONObject.quote(token) + "," + id
                        + ",JSON.stringify({ok:true,value:v===undefined?null:v}));}catch(e){NU11.settle(" + JSONObject.quote(token) + "," + id
                        + ",JSON.stringify({ok:false,error:String((e&&e.message)||e)}));}})();void 0";
                    runOnUiThread(() -> {
                        if (!destroyed && pendingEvaluations.contains(id)) music.evaluateJavascript(script, null);
                    });
                }
            }
        }
    }

    // RECORD_AUDIO gates Visualizer's output mix (session 0), not a microphone.
    // Only a playback-driven levels request can open the permission dialog.
    private void updateCapture() {
        if (!levelsWanted || capturePaused || destroyed) { releaseCapture(); return; }
        if (captureFailed || visualizer != null) return;
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            if (permissionAsked) return;
            boolean askedBefore = getPreferences(MODE_PRIVATE).getBoolean("levelsPermissionAsked", false);
            if (askedBefore && !shouldShowRequestPermissionRationale(Manifest.permission.RECORD_AUDIO)) return;
            permissionAsked = true;
            getPreferences(MODE_PRIVATE).edit().putBoolean("levelsPermissionAsked", true).apply();
            requestPermissions(new String[]{Manifest.permission.RECORD_AUDIO}, LEVELS_PERMISSION);
            return;
        }
        try {
            visualizer = new Visualizer(0);
            visualizer.setEnabled(false);
            visualizer.setCaptureSize(Visualizer.getCaptureSizeRange()[1]);
            // NORMALIZED avoids TV-volume-dependent levels; AS_PLAYED follows volume.
            // Its automatic gain means these are normalized, not absolute source dBFS.
            visualizer.setScalingMode(Visualizer.SCALING_MODE_NORMALIZED);
            java.util.Arrays.fill(smoothBands, 0);
            capturedWave = null;
            lastCapture = SystemClock.elapsedRealtime();
            int result = visualizer.setDataCaptureListener(new Visualizer.OnDataCaptureListener() {
                @Override public void onWaveFormDataCapture(Visualizer source, byte[] wave, int rate) {
                    if (source != visualizer || !levelsWanted || capturePaused) return;
                    JSONArray points = new JSONArray();
                    for (int i = 0; i < 64; i++) {
                        int swing = 0;
                        for (int j = i * wave.length / 64; j < (i + 1) * wave.length / 64; j++) {
                            int value = (wave[j] & 255) - 128;
                            if (Math.abs(value) > Math.abs(swing)) swing = value;
                        }
                        points.put(Math.max(-100, Math.min(100, Math.round(swing * 100f / 128))));
                    }
                    capturedWave = points;
                    try { sendSpectrum(wave, rate / 1000.0); }
                    catch (Exception e) { captureUnavailable(e); }
                }
                @Override public void onFftDataCapture(Visualizer source, byte[] fft, int rate) { }
            }, Math.min(20000, Visualizer.getMaxCaptureRate()), true, false);
            if (result != Visualizer.SUCCESS || visualizer.setEnabled(true) != Visualizer.SUCCESS) {
                throw new IllegalStateException("Cannot enable output-mix capture");
            }
        } catch (RuntimeException e) { captureUnavailable(e); }
    }

    private void sendSpectrum(byte[] wave, double sampleRate) throws org.json.JSONException {
        long now = SystemClock.elapsedRealtime();
        if (now - lastCapture < 67) return; // At most ~15 bridge readings/second.
        double fall = 100.0 * (now - lastCapture) / 1000.0;
        lastCapture = now;
        double[] levels = Spectrum.levels(wave, sampleRate);
        JSONArray bands = new JSONArray();
        for (int band = 0; band < levels.length; band++) {
            smoothBands[band] = Math.max(levels[band], smoothBands[band] - fall);
            bands.put((int) Math.round(smoothBands[band]));
        }
        send(new JSONObject().put("type", "levels").put("bands", bands)
                .put("wave", capturedWave == null ? new JSONArray() : capturedWave));
    }

    private void captureUnavailable(Exception error) {
        if (!captureFailed) Log.w(TAG, "Output-mix spectrum unavailable; using decorative rain", error);
        captureFailed = true;
        releaseCapture();
    }

    private void releaseCapture() {
        Visualizer old = visualizer;
        visualizer = null;
        capturedWave = null;
        if (old != null) {
            try { old.release(); }
            catch (RuntimeException e) {
                if (!captureFailed) Log.w(TAG, "Cannot release output-mix capture", e);
                captureFailed = true;
            }
        }
    }

    @Override public void onRequestPermissionsResult(int request, String[] permissions, int[] grants) {
        super.onRequestPermissionsResult(request, permissions, grants);
        if (request == LEVELS_PERMISSION && grants.length > 0 && grants[0] == PackageManager.PERMISSION_GRANTED) updateCapture();
    }

    @Override protected void onStart() {
        super.onStart();
        synchronized (terminalOutputLock) { terminalVisible = true; }
        // SIGWINCH causes Bubble Tea to redraw after discarded background output.
        sendResize();
    }

    @Override protected void onStop() {
        synchronized (terminalOutputLock) {
            terminalVisible = false;
            pendingTerminalOutput.reset();
            terminalOutputLock.notifyAll();
        }
        // Do not pause either WebView or its timers: MusicKit owns audio.
        super.onStop();
    }

    @Override protected void onPause() {
        if (updates != null) updates.pause();
        PlaybackService.activityForeground(false);
        capturePaused = true;
        releaseCapture();
        super.onPause();
    }

    @Override protected void onResume() {
        super.onResume();
        PlaybackService.activityForeground(true);
        capturePaused = false;
        updateCapture();
        if (updates != null) updates.resume();
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

    // Called only by the same-process media service on the main thread.
    void mediaInput(String text) {
        if (!destroyed && !loginShown && popup == null) input(text);
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
        boolean interrupted = false;
        try {
            synchronized (terminalOutputLock) {
                int offset = 0;
                while (!destroyed && offset < bytes.length) {
                    // Backpressure the producer at the cap; never discard terminal bytes.
                    while (!destroyed && terminalVisible && pendingTerminalOutput.size() == MAX_PENDING_TERMINAL_BYTES) {
                        try { terminalOutputLock.wait(); }
                        catch (InterruptedException e) { interrupted = true; }
                    }
                    if (destroyed || !terminalVisible) return;
                    int count = Math.min(bytes.length - offset,
                            MAX_PENDING_TERMINAL_BYTES - pendingTerminalOutput.size());
                    pendingTerminalOutput.write(bytes, offset, count);
                    offset += count;
                    if (!terminalEvaluationPending) {
                        terminalEvaluationPending = true;
                        root.post(this::flushTerminalOutput);
                    }
                }
            }
        } finally {
            if (interrupted) Thread.currentThread().interrupt();
        }
    }

    private void flushTerminalOutput() {
        byte[] bytes;
        synchronized (terminalOutputLock) {
            if (destroyed || !terminalVisible) {
                pendingTerminalOutput.reset();
                terminalEvaluationPending = false;
                terminalOutputLock.notifyAll();
                return;
            }
            bytes = pendingTerminalOutput.toByteArray();
            pendingTerminalOutput.reset();
            terminalOutputLock.notifyAll();
        }
        String base64 = Base64.encodeToString(bytes, Base64.NO_WRAP);
        terminal.evaluateJavascript("window.nu11Write('" + base64 + "')", result -> {
            synchronized (terminalOutputLock) {
                terminalEvaluationPending = false;
                if (!destroyed && terminalVisible && pendingTerminalOutput.size() > 0) {
                    terminalEvaluationPending = true;
                    root.post(this::flushTerminalOutput);
                }
            }
        });
    }

    private void error(String message) {
        Log.e(TAG, message);
        // Remove control characters so error descriptions cannot inject terminal escapes.
        writeTerminal(("\r\n" + message.replaceAll("[\\p{Cntrl}]", " ") + "\r\n").getBytes(StandardCharsets.UTF_8));
    }

    private void resetKeyboard() {
        resettingKeyboard = true;
        keyboardTextSent = "";
        keyboard.getText().clear();
        resettingKeyboard = false;
    }

    private void submitKeyboard() {
        input("\r");
        resetKeyboard();
    }

    private void toggleKeyboard() { setKeyboard(!keyboardShown); }

    private void setKeyboard(boolean show) {
        if (!show) resetKeyboard();
        if (keyboardShown == show) return;
        // Every new IME session starts empty; never diff against an old session.
        if (show) resetKeyboard();
        InputMethodManager ime = (InputMethodManager) getSystemService(INPUT_METHOD_SERVICE);
        keyboardShown = show;
        keyboardWasVisible = false;
        if (keyboardShown) { keyboard.requestFocus(); ime.showSoftInput(keyboard, InputMethodManager.SHOW_IMPLICIT); }
        else { ime.hideSoftInputFromWindow(keyboard.getWindowToken(), 0); terminal.requestFocus(); }
    }

    @Override public boolean dispatchKeyEvent(KeyEvent event) {
        int key = event.getKeyCode();
        if (updates != null && updates.visible()) return super.dispatchKeyEvent(event);
        if (updates != null && event.getAction() == KeyEvent.ACTION_UP) root.post(updates::maybeOffer);
        // Route the guide before IME, pointer, media, or terminal handling.
        // Activate on release so the same OK cannot also tap Apple's page.
        if (guideVisible()) {
            if (event.getAction() == KeyEvent.ACTION_UP && !event.isCanceled()) {
                if (key == KeyEvent.KEYCODE_DPAD_CENTER || key == KeyEvent.KEYCODE_ENTER
                        || key == KeyEvent.KEYCODE_NUMPAD_ENTER) startLoginPointer();
                else if (key == KeyEvent.KEYCODE_BACK) onBackPressed();
            }
            return key == KeyEvent.KEYCODE_VOLUME_UP || key == KeyEvent.KEYCODE_VOLUME_DOWN
                    || key == KeyEvent.KEYCODE_VOLUME_MUTE ? super.dispatchKeyEvent(event) : true;
        }
        if (loginShown || popup != null) {
            // Media input must not reach either the hidden TUI or Apple's page.
            if (key >= KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE && key <= KeyEvent.KEYCODE_MEDIA_FAST_FORWARD
                    || key == KeyEvent.KEYCODE_MEDIA_PLAY || key == KeyEvent.KEYCODE_MEDIA_PAUSE
                    || key == KeyEvent.KEYCODE_MEDIA_CLOSE || key == KeyEvent.KEYCODE_MEDIA_EJECT
                    || key == KeyEvent.KEYCODE_MEDIA_RECORD || key == KeyEvent.KEYCODE_MEDIA_AUDIO_TRACK
                    || key == KeyEvent.KEYCODE_MEDIA_SKIP_FORWARD || key == KeyEvent.KEYCODE_MEDIA_SKIP_BACKWARD
                    || key == KeyEvent.KEYCODE_MEDIA_STEP_FORWARD || key == KeyEvent.KEYCODE_MEDIA_STEP_BACKWARD) return true;
            if (loginImeVisible()) return super.dispatchKeyEvent(event);
            if (loginShown && (key == KeyEvent.KEYCODE_DPAD_UP || key == KeyEvent.KEYCODE_DPAD_DOWN
                    || key == KeyEvent.KEYCODE_DPAD_LEFT || key == KeyEvent.KEYCODE_DPAD_RIGHT)) {
                if (event.getAction() == KeyEvent.ACTION_DOWN) movePointer(key, event.getRepeatCount());
                return true;
            }
            if (loginShown && (key == KeyEvent.KEYCODE_DPAD_CENTER || key == KeyEvent.KEYCODE_ENTER
                    || key == KeyEvent.KEYCODE_NUMPAD_ENTER)) {
                if (event.getAction() == KeyEvent.ACTION_DOWN) {
                    if (event.getRepeatCount() == 0) centerLong = false;
                    if (event.isLongPress()) centerLong = true;
                } else if (event.getAction() == KeyEvent.ACTION_UP && !event.isCanceled() && !centerLong
                        && event.getEventTime() - event.getDownTime() < android.view.ViewConfiguration.getLongPressTimeout()) {
                    startPointerTouch(0);
                }
                return true;
            }
            if (key == KeyEvent.KEYCODE_BACK) {
                if (event.getAction() == KeyEvent.ACTION_UP) onBackPressed();
                return true;
            }
            return super.dispatchKeyEvent(event);
        }
        if (key == KeyEvent.KEYCODE_MENU) {
            if (event.getAction() == KeyEvent.ACTION_DOWN && event.getRepeatCount() == 0) menuLong = false;
            if (event.isLongPress()) menuLong = true;
            if (event.getAction() == KeyEvent.ACTION_UP && !event.isCanceled()) {
                boolean held = menuLong || event.getEventTime() - event.getDownTime()
                        >= android.view.ViewConfiguration.getLongPressTimeout();
                if (held && !keyboardShown) {
                    if (updates != null) updates.open();
                    return true;
                }
                if (textInputAvailable) {
                    if (!keyboardShown) setKeyboard(true);
                } else {
                    // Non-printable MENU signal opens/refocuses SEARCH even
                    // with a parked page. Wait for Go to confirm input focus.
                    keyboardAfterSearch = true;
                    input("\u001f");
                }
            }
            return true;
        }
        // A different remote action cancels an outstanding MENU IME request.
        keyboardAfterSearch = false;
        if (keyboardShown && (key == KeyEvent.KEYCODE_ENTER || key == KeyEvent.KEYCODE_NUMPAD_ENTER)) {
            if (event.getAction() == KeyEvent.ACTION_DOWN) submitKeyboard();
            return true;
        }
        if (keyboardShown && key == KeyEvent.KEYCODE_DEL) {
            if (keyboard.length() == 0) {
                if (event.getAction() == KeyEvent.ACTION_DOWN) input("\u007f");
                return true;
            }
            return super.dispatchKeyEvent(event);
        }
        if (keyboardShown && (key == KeyEvent.KEYCODE_DPAD_UP || key == KeyEvent.KEYCODE_DPAD_DOWN
                || key == KeyEvent.KEYCODE_DPAD_LEFT || key == KeyEvent.KEYCODE_DPAD_RIGHT)) {
            return super.dispatchKeyEvent(event);
        }
        if (keyboardShown && (event.getUnicodeChar() > 0
                || event.getAction() == KeyEvent.ACTION_MULTIPLE && event.getCharacters() != null)) {
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
                    if (key == KeyEvent.KEYCODE_BACK && !backLong) { backLong = true; if (keyboardShown) toggleKeyboard(); else input("\u0011"); }
                    if (key == KeyEvent.KEYCODE_DPAD_CENTER && !centerLong) { centerLong = true; toggleKeyboard(); }
                }
                if (key == KeyEvent.KEYCODE_DPAD_CENTER && keyboardShown && !centerLong) {
                    return super.dispatchKeyEvent(event);
                }
            } else if (event.getAction() == KeyEvent.ACTION_UP && !event.isCanceled()) {
                boolean held = event.getEventTime() - event.getDownTime() >= android.view.ViewConfiguration.getLongPressTimeout();
                if (key == KeyEvent.KEYCODE_BACK && !backLong) { if (keyboardShown) toggleKeyboard(); else input(held ? "\u0011" : "\u001b"); }
                if (key == KeyEvent.KEYCODE_DPAD_CENTER && !centerLong) {
                    if (held) toggleKeyboard();
                    else if (keyboardShown) return super.dispatchKeyEvent(event);
                    else if (textInputAvailable) setKeyboard(true);
                    else input("\r");
                }
            }
            return true;
        }
        if ((key == KeyEvent.KEYCODE_ENTER || key == KeyEvent.KEYCODE_NUMPAD_ENTER)
                && textInputAvailable && !keyboardShown) {
            // Activate on release, consuming both halves so opening the IME
            // cannot also submit the input with the same Enter.
            if (event.getAction() == KeyEvent.ACTION_UP && !event.isCanceled()) setKeyboard(true);
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
        if (guideVisible()) {
            showLogin(false);
            try { send(new JSONObject().put("type", "login").put("closed", true)); }
            catch (Exception e) { Log.w(TAG, "Login close failed", e); }
            return;
        }
        if (popup != null) {
            cancelPointerTouch();
            if (popup.canGoBack()) popup.goBack(); else closePopup();
            return;
        }
        if (loginShown) {
            showLogin(false);
            try { send(new JSONObject().put("type", "login").put("closed", true)); }
            catch (Exception e) { Log.w(TAG, "Login close failed", e); }
        } else if (keyboardShown) toggleKeyboard();
        else input("\u001b");
    }

    @Override protected void onDestroy() {
        destroyed = true;
        if (updates != null) updates.destroy();
        PlaybackService.detach(this);
        releaseCapture();
        synchronized (terminalOutputLock) { terminalOutputLock.notifyAll(); }
        root.removeCallbacks(hideLoginConfirmation);
        cancelPointerTouch();
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

// Package-private pure DSP, kept here because the platform build compiles this file only.
final class Spectrum {
    private Spectrum() { }

    /* Unsigned PCM -> [-1,1), periodic Hann -> unscaled radix-2 FFT.
     * One-sided bin power = |X[k]|^2 / (N * sum(window^2)), doubled except
     * at DC/Nyquist. This compensates Hann's energy gain, so summing bins
     * measures mean-square full-scale power (a unit sine is -3.01 dBFS).
     * Empty log bands interpolate power, not dB, at their geometric centre.
     */
    static double[] levels(byte[] pcm, double sampleRate) {
        int n = pcm.length;
        if (n < 2 || (n & (n - 1)) != 0 || !(sampleRate > 0)) {
            throw new IllegalArgumentException("Power-of-two PCM and positive sample rate required");
        }
        double[] re = new double[n], im = new double[n];
        double windowEnergy = 0;
        for (int i = 0; i < n; i++) {
            double window = 0.5 - 0.5 * Math.cos(2 * Math.PI * i / n);
            re[i] = ((pcm[i] & 255) - 128) / 128.0 * window;
            windowEnergy += window * window;
        }
        fft(re, im);
        double[] power = new double[n / 2 + 1];
        for (int k = 0; k < power.length; k++) {
            double sides = k == 0 || k == n / 2 ? 1 : 2;
            power[k] = sides * (re[k] * re[k] + im[k] * im[k]) / (n * windowEnergy);
        }
        return bandLevels(power, sampleRate / n);
    }

    static double[] bandLevels(double[] power, double binWidth) {
        double[] result = new double[24];
        for (int band = 0; band < result.length; band++) {
            double low = 40 * Math.pow(400, band / 24.0);
            double high = 40 * Math.pow(400, (band + 1) / 24.0);
            double sum = 0;
            int count = 0;
            for (int k = 0; k < power.length; k++) {
                double frequency = k * binWidth;
                if (frequency >= low && frequency < high) {
                    sum += power[k];
                    count++;
                }
            }
            double centre = Math.sqrt(low * high) / binWidth;
            if (count == 0 && centre <= power.length - 1) {
                int left = (int) centre;
                int right = Math.min(left + 1, power.length - 1);
                sum = power[left] + (power[right] - power[left]) * (centre - left);
            }
            // Frequencies beyond Nyquist have no physical data and remain silent.
            double db = 10 * Math.log10(Math.max(1e-12, sum));
            result[band] = Math.max(0, Math.min(100, (db + 50) * 100 / 40));
        }
        return result;
    }

    private static void fft(double[] re, double[] im) {
        int n = re.length;
        for (int i = 1, j = 0; i < n; i++) {
            int bit = n >> 1;
            for (; (j & bit) != 0; bit >>= 1) j ^= bit;
            j ^= bit;
            if (i < j) {
                double value = re[i]; re[i] = re[j]; re[j] = value;
            }
        }
        for (int size = 2; size <= n; size <<= 1) {
            for (int start = 0; start < n; start += size) {
                for (int k = 0; k < size / 2; k++) {
                    double angle = -2 * Math.PI * k / size;
                    double c = Math.cos(angle), s = Math.sin(angle);
                    int a = start + k, b = a + size / 2;
                    double real = c * re[b] - s * im[b];
                    double imaginary = s * re[b] + c * im[b];
                    re[b] = re[a] - real; im[b] = im[a] - imaginary;
                    re[a] += real; im[a] += imaginary;
                }
            }
        }
    }
}

final class KeyboardEdits {
    private KeyboardEdits() { }

    static String diff(String previous, String next) {
        int prefix = 0;
        // Compare whole code points so a shared surrogate never splits UTF-8.
        while (prefix < previous.length() && prefix < next.length()) {
            int cp = previous.codePointAt(prefix);
            if (cp != next.codePointAt(prefix)) break;
            prefix += Character.charCount(cp);
        }
        StringBuilder edit = new StringBuilder();
        int removed = previous.codePointCount(prefix, previous.length());
        for (int i = 0; i < removed; i++) edit.append('\u007f');
        return edit.append(next.substring(prefix)).toString();
    }
}
