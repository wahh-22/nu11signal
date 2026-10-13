package dev.wahh.nu11signal.tv;

public final class TvUpdateStateTest {
    public static void main(String[] args) {
        TvUpdateState state = new TvUpdateState();
        state.resume();
        long download = state.begin();
        require(download > 0 && state.begin() == -1, "double OK must not start concurrent work");
        state.pause();
        require(!state.accept(download), "paused completion cannot open installer");
        state.resume();
        long replacement = state.begin();
        require(!state.accept(download), "stale callback must not replace current work");
        require(state.accept(replacement), "current foreground completion accepted");
        state.finish(replacement);
        require(!state.accept(replacement), "completion can only be applied once");
        long cancelled = state.begin();
        state.cancel();
        require(!state.accept(cancelled), "Back invalidates callback");
        state.destroy();
        state.resume();
        require(state.begin() == -1, "destroyed activity cannot restart");
        require(TvUpdateState.inputIdle(false, false), "idle input permits an offer");
        require(!TvUpdateState.inputIdle(true, false), "visible keyboard retains focus");
        require(!TvUpdateState.inputIdle(false, true), "pending short-Menu SEARCH retains focus before IME acknowledgement");
        require(!TvUpdateState.inputIdle(true, true), "pending search and visible keyboard retain focus");
        System.out.println("PASS: 11 update lifecycle/input assertions");
    }
    private static void require(boolean value, String message) {
        if (!value) throw new AssertionError(message);
    }
}
