package dev.wahh.nu11signal.vizspike;

import android.app.Activity;
import android.media.audiofx.Visualizer;
import android.os.Bundle;
import android.util.Log;

// Logs, twice a second for 20 s, how loud the output mix is according to
// Visualizer (session 0): peak waveform swing and the strongest FFT bin.
// All zeros while music plays means the protected audio is not exposed.
public class MainActivity extends Activity {
    private static final String TAG = "Nu11VizSpike";
    private Visualizer visualizer;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        try {
            visualizer = new Visualizer(0);
            visualizer.setEnabled(false);
            visualizer.setCaptureSize(Visualizer.getCaptureSizeRange()[1]);
            visualizer.setScalingMode(Visualizer.SCALING_MODE_AS_PLAYED);
            visualizer.setEnabled(true);
        } catch (Throwable e) {
            Log.e(TAG, "Visualizer unavailable: " + e);
            finish();
            return;
        }
        new Thread(this::sample, "viz-sample").start();
    }

    private void sample() {
        int size = visualizer.getCaptureSize();
        byte[] wave = new byte[size];
        byte[] fft = new byte[size];
        for (int i = 0; i < 40; i++) {
            int w = visualizer.getWaveForm(wave);
            int f = visualizer.getFft(fft);
            int swing = 0;
            for (byte b : wave) swing = Math.max(swing, Math.abs((b & 0xff) - 128));
            int peak = 0;
            for (int k = 2; k + 1 < size; k += 2) {
                peak = Math.max(peak, (int) Math.hypot(fft[k], fft[k + 1]));
            }
            Log.i(TAG, "sample " + i + " status=" + w + "/" + f + " swing=" + swing + " fftPeak=" + peak);
            try { Thread.sleep(500); } catch (InterruptedException e) { return; }
        }
        visualizer.release();
        Log.i(TAG, "done");
        runOnUiThread(this::finish);
    }
}
