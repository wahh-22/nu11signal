package dev.wahh.nu11signal.tv;

/** Main-thread lifecycle token. No completion may outlive consent or foreground state. */
public final class TvUpdateState {
    /** A pending search transition owns input focus even before the IME appears. */
    public static boolean inputIdle(boolean keyboardShown, boolean searchPending) {
        return !keyboardShown && !searchPending;
    }
    private long generation;
    private boolean foreground, busy, destroyed;
    public void resume() { if (!destroyed) foreground = true; }
    public void pause() { foreground = false; cancel(); }
    public void destroy() { destroyed = true; pause(); }
    public long begin() {
        if (!foreground || destroyed || busy) return -1;
        busy = true;
        return ++generation;
    }
    public boolean accept(long token) {
        return foreground && !destroyed && busy && token == generation;
    }
    public void finish(long token) { if (accept(token)) busy = false; }
    public void cancel() { generation++; busy = false; }
}
