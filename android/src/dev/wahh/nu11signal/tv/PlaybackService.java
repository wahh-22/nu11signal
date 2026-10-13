package dev.wahh.nu11signal.tv;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.media.MediaMetadata;
import android.media.session.MediaSession;
import android.media.session.PlaybackState;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.os.SystemClock;
import android.view.KeyEvent;
import org.json.JSONObject;

/** In-process only: the Activity owns Go and WebViews; this service owns
 * system media integration, not an independent player. All bridge access is
 * on the main thread. detach clears the Activity reference on destruction. */
public final class PlaybackService extends Service {
    private static MainActivity owner;
    private static PlaybackService instance;
    private static JSONObject latest;
    private static boolean foregroundActivity, requested;
    private MediaSession session;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private boolean playing, foreground, pausedDeadline, hasStarted;
    private final Runnable demote = () -> {
        pausedDeadline = false;
        if (!playing) { stopForeground(STOP_FOREGROUND_REMOVE); foreground = false; }
    };

    static void attach(MainActivity activity) { owner = activity; }
    static void activityForeground(boolean value) { foregroundActivity = value; }
    static void detach(MainActivity activity) {
        if (owner != activity) return;
        owner = null;
        latest = null;
        requested = false;
        foregroundActivity = false;
        activity.stopService(new Intent(activity, PlaybackService.class));
    }
    static void update(JSONObject message) {
        if (owner == null) return;
        latest = message;
        if (instance != null) instance.apply();
        else if (message.optBoolean("playing") && !requested) {
            requested = true;
            owner.startForegroundService(new Intent(owner, PlaybackService.class));
        }
    }
    private void command(String text) {
        if (owner != null) owner.mediaInput(text);
    }
    private void play() { if (!playing) command(" "); }
    private void pause() { if (playing) command(" "); }

    @Override public void onCreate() {
        super.onCreate();
        instance = this;
        NotificationManager manager = getSystemService(NotificationManager.class);
        manager.createNotificationChannel(new NotificationChannel("playback", "Playback", NotificationManager.IMPORTANCE_LOW));
        session = new MediaSession(this, "nu11signal");
        session.setCallback(new MediaSession.Callback() {
            @Override public void onPlay() { play(); }
            @Override public void onPause() { pause(); }
            @Override public void onSkipToNext() { command("n"); }
            @Override public void onSkipToPrevious() { command("p"); }
            @Override public boolean onMediaButtonEvent(Intent intent) {
                // The resumed Activity consumes physical media keys itself.
                // Never dispatch the same event through both input paths.
                if (foregroundActivity) return true;
                KeyEvent event = intent.getParcelableExtra(Intent.EXTRA_KEY_EVENT);
                if (event == null) return false;
                if (event.getAction() != KeyEvent.ACTION_DOWN || event.getRepeatCount() != 0) return true;
                switch (event.getKeyCode()) {
                    case KeyEvent.KEYCODE_MEDIA_PLAY: play(); break;
                    case KeyEvent.KEYCODE_MEDIA_PAUSE: pause(); break;
                    case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE: command(" "); break;
                    case KeyEvent.KEYCODE_MEDIA_NEXT: case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD: command("n"); break;
                    case KeyEvent.KEYCODE_MEDIA_PREVIOUS: case KeyEvent.KEYCODE_MEDIA_REWIND: command("p"); break;
                    default: return false;
                }
                return true;
            }
        }, handler);
        session.setActive(true);
        apply();
    }
    @Override public int onStartCommand(Intent intent, int flags, int id) {
        if (owner == null) { stopSelf(); return START_NOT_STICKY; }
        if (intent != null) {
            String action = intent.getAction();
            if ("previous".equals(action)) command("p");
            else if ("next".equals(action)) command("n");
            else if ("toggle".equals(action)) command(" ");
        }
        apply();
        return START_NOT_STICKY;
    }
    private PendingIntent action(String name, int code) {
        return PendingIntent.getService(this, code, new Intent(this, PlaybackService.class).setAction(name),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }
    private void apply() {
        if (latest == null || session == null) return;
        playing = latest.optBoolean("playing");
        long position = Math.max(0, latest.optLong("position_ms"));
        long duration = Math.max(0, latest.optLong("duration_ms"));
        session.setMetadata(new MediaMetadata.Builder()
                .putString(MediaMetadata.METADATA_KEY_TITLE, latest.optString("title"))
                .putString(MediaMetadata.METADATA_KEY_ARTIST, latest.optString("artist"))
                .putString(MediaMetadata.METADATA_KEY_ALBUM, latest.optString("album"))
                .putLong(MediaMetadata.METADATA_KEY_DURATION, duration).build());
        session.setPlaybackState(new PlaybackState.Builder()
                .setActions(PlaybackState.ACTION_PLAY | PlaybackState.ACTION_PAUSE | PlaybackState.ACTION_PLAY_PAUSE
                        | PlaybackState.ACTION_SKIP_TO_NEXT | PlaybackState.ACTION_SKIP_TO_PREVIOUS)
                .setState(playing ? PlaybackState.STATE_PLAYING : PlaybackState.STATE_PAUSED,
                        position, playing ? 1f : 0f, SystemClock.elapsedRealtime()).build());
        PendingIntent open = PendingIntent.getActivity(this, 0, new Intent(this, MainActivity.class),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        Notification notification = new Notification.Builder(this, "playback")
                .setSmallIcon(R.drawable.icon).setContentTitle(latest.optString("title"))
                .setContentText(latest.optString("artist")).setContentIntent(open)
                .setCategory(Notification.CATEGORY_TRANSPORT).setOnlyAlertOnce(true).setOngoing(playing)
                .addAction(android.R.drawable.ic_media_previous, "Previous", action("previous", 1))
                .addAction(playing ? android.R.drawable.ic_media_pause : android.R.drawable.ic_media_play,
                        playing ? "Pause" : "Play", action("toggle", 2))
                .addAction(android.R.drawable.ic_media_next, "Next", action("next", 3))
                .setStyle(new Notification.MediaStyle().setMediaSession(session.getSessionToken()).setShowActionsInCompactView(0, 1, 2))
                .build();
        if (playing || !hasStarted) {
            startForeground(1, notification);
            foreground = true;
            hasStarted = true;
        } else if (foreground) getSystemService(NotificationManager.class).notify(1, notification);
        if (playing) {
            handler.removeCallbacks(demote);
            pausedDeadline = false;
        } else if (foreground && !pausedDeadline) {
            pausedDeadline = true;
            handler.postDelayed(demote, 5 * 60 * 1000);
        }
    }
    @Override public IBinder onBind(Intent intent) { return null; }
    @Override public void onDestroy() {
        handler.removeCallbacksAndMessages(null);
        session.setActive(false);
        session.release();
        stopForeground(STOP_FOREGROUND_REMOVE);
        instance = null;
        requested = false;
        super.onDestroy();
    }
}
