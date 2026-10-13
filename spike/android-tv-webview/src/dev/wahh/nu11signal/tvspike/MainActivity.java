package dev.wahh.nu11signal.tvspike;

import android.app.Activity;
import android.os.Bundle;
import android.os.Message;
import android.util.Log;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.webkit.ConsoleMessage;
import android.webkit.CookieManager;
import android.webkit.PermissionRequest;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;

public class MainActivity extends Activity {
    private static final String TAG = "Nu11TvSpike";
    private FrameLayout root;
    private WebView webView;
    // The window MusicKit opens for Apple ID sign-in (authorize.music.apple.com);
    // it answers its opener with postMessage, so it must be a real child window.
    private WebView popup;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        WebView.setWebContentsDebuggingEnabled(true);
        CookieManager.getInstance().setAcceptCookie(true);

        root = new FrameLayout(this);
        webView = newWebView();
        String ua = getIntent().getStringExtra("ua");
        if (ua != null) {
            webView.getSettings().setUserAgentString(ua);
        }
        root.addView(webView, fill());
        setContentView(root);
        webView.requestFocus();

        if (savedInstanceState == null || webView.restoreState(savedInstanceState) == null) {
            String url = getIntent().getStringExtra("url");
            webView.loadUrl(url != null ? url : "https://music.apple.com/");
        }
        // Intentionally do not call WebView.onPause(): keep audio active when backgrounded.
    }

    private WebView newWebView() {
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
        view.setWebChromeClient(chromeClient);
        view.setWebViewClient(viewClient);
        view.setFocusable(true);
        view.setFocusableInTouchMode(true);
        return view;
    }

    private static FrameLayout.LayoutParams fill() {
        return new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT);
    }

    private void closePopup() {
        if (popup == null) {
            return;
        }
        root.removeView(popup);
        popup.destroy();
        popup = null;
        webView.requestFocus();
    }

    private final WebChromeClient chromeClient = new WebChromeClient() {
        @Override
        public boolean onCreateWindow(WebView view, boolean isDialog, boolean isUserGesture,
                Message resultMsg) {
            Log.i(TAG, "Opening child window (dialog=" + isDialog
                    + ", gesture=" + isUserGesture + ")");
            closePopup();
            popup = newWebView();
            root.addView(popup, fill());
            popup.requestFocus();
            WebView.WebViewTransport transport = (WebView.WebViewTransport) resultMsg.obj;
            transport.setWebView(popup);
            resultMsg.sendToTarget();
            return true;
        }

        @Override
        public void onCloseWindow(WebView window) {
            Log.i(TAG, "Child window closed itself");
            if (window == popup) {
                closePopup();
            }
        }

        @Override
        public void onPermissionRequest(PermissionRequest request) {
            for (String resource : request.getResources()) {
                Log.i(TAG, "Permission requested: " + resource);
            }
            for (String resource : request.getResources()) {
                if (PermissionRequest.RESOURCE_PROTECTED_MEDIA_ID.equals(resource)) {
                    Log.i(TAG, "Granting protected media ID only");
                    request.grant(new String[] { PermissionRequest.RESOURCE_PROTECTED_MEDIA_ID });
                    return;
                }
            }
            Log.i(TAG, "Denying permission request");
            request.deny();
        }

        @Override
        public boolean onConsoleMessage(ConsoleMessage message) {
            Log.i(TAG, message.messageLevel() + ": " + message.message()
                    + " (" + message.sourceId() + ":" + message.lineNumber() + ")");
            return true;
        }
    };

    private final WebViewClient viewClient = new WebViewClient() {
        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
            String scheme = request.getUrl().getScheme();
            boolean allowed = "https".equalsIgnoreCase(scheme)
                    || "http".equalsIgnoreCase(scheme);
            if (!allowed) {
                Log.i(TAG, "Blocked external navigation scheme: " + scheme);
            }
            return !allowed;
        }

        @Override
        public void onReceivedError(WebView view, WebResourceRequest request,
                WebResourceError error) {
            if (request.isForMainFrame()) {
                Log.e(TAG, "Main frame error " + error.getErrorCode()
                        + ": " + error.getDescription());
            }
        }

        @Override
        public void onReceivedHttpError(WebView view, WebResourceRequest request,
                WebResourceResponse response) {
            if (request.isForMainFrame()) {
                Log.e(TAG, "Main frame HTTP error " + response.getStatusCode()
                        + ": " + response.getReasonPhrase());
            }
        }
    };

    @Override
    protected void onSaveInstanceState(Bundle outState) {
        super.onSaveInstanceState(outState);
        webView.saveState(outState);
    }

    @Override
    public void onBackPressed() {
        if (popup != null) {
            if (popup.canGoBack()) {
                popup.goBack();
            } else {
                closePopup();
            }
        } else if (webView.canGoBack()) {
            webView.goBack();
        } else {
            super.onBackPressed();
        }
    }
}
