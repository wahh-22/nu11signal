package dev.wahh.nu11signal.tv;

import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayDeque;
import java.util.Arrays;
import java.util.Queue;

public final class TvUpdateClientTest {
    private static final String BASE = "https://github.com/wahh-22/nu11signal/releases/download/v";
    private static int assertions;
    private interface Action { void run() throws Exception; }
    private static void yes(boolean value) { assertions++; if (!value) throw new AssertionError(); }
    private static void fails(Action action) throws Exception {
        assertions++;
        try { action.run(); } catch (IOException | IllegalArgumentException expected) { return; }
        throw new AssertionError("Expected rejection");
    }
    private static String url(String version) { return BASE + version + "/nu11signal-tv-" + version + ".apk"; }
    private static String asset(String version, long size) {
        return "{\"name\":\"nu11signal-tv-" + version + ".apk\",\"size\":" + size
                + ",\"browser_download_url\":\"" + url(version) + "\"}";
    }
    private static String release(String version, String assets) {
        return "{\"tag_name\":\"v" + version + "\",\"draft\":false,\"prerelease\":false,\"assets\":[" + assets + "]}";
    }
    private static final class Fake implements TvUpdateClient.Transport {
        final Queue<TvUpdateClient.Response> responses = new ArrayDeque<>();
        int opens, closes;
        String lastUrl;
        Fake add(int status, String location, byte[] bytes, long length) {
            responses.add(new TvUpdateClient.Response(status, location, length,
                    new ByteArrayInputStream(bytes), () -> closes++));
            return this;
        }
        Fake json(String json) { return add(200, null, json.getBytes(StandardCharsets.UTF_8), -1); }
        public TvUpdateClient.Response open(String address) throws IOException {
            opens++; lastUrl = address;
            if (responses.isEmpty()) throw new IOException("Fixture exhausted");
            return responses.remove();
        }
    }
    private static TvUpdateRelease candidate(String address, long size) {
        return new TvUpdateRelease("1.2.3", 10203, address, size);
    }
    private static void versions() throws Exception {
        yes(TvUpdateRelease.versionCode("1.2.3") == 10203);
        yes(TvUpdateRelease.versionCode("0.0.1") == 1);
        yes(TvUpdateRelease.versionCode("210000.0.0") == 2100000000L);
        for (String v : Arrays.asList("0.0.0", "01.2.3", "1.02.3", "1.2.03", "1.100.0", "1.0.100",
                "210000.0.1", "999999.0.0", "1000000.0.0", "v1.2.3", "1.2.3-beta", "1.2.3+build", "1.2", "-1.2.3", " 1.2.3", "1.2.3\n")) {
            fails(() -> TvUpdateRelease.versionCode(v));
        }
        fails(() -> TvUpdateRelease.versionCode(null));
    }
    private static void discovery() throws Exception {
        String valid = release("1.2.3", asset("1.2.3", 3));
        Fake f = new Fake().json("[" + release("9.0.0", "") + "," + valid + "]");
        TvUpdateRelease r = new TvUpdateClient(f).check(10202);
        yes(r.versionCode == 10203 && r.size == 3 && r.url.equals(url("1.2.3")));
        yes(f.opens == 1 && f.closes == 1);
        yes(f.lastUrl.equals("https://api.github.com/repos/wahh-22/nu11signal/releases?per_page=30&page=1"));
        yes(new TvUpdateClient(new Fake().json("[" + valid + "]")).check(10203) == null);
        yes(new TvUpdateClient(new Fake().json("[]")).check(0) == null);
        yes(new TvUpdateClient(new Fake().json("[" + valid + "]")).check(20000) == null);
        String escaped = valid.replace("nu11signal-tv-", "nu11signal\\u002Dtv-")
                .replace("\"draft\":false", "\"note\":\"quote\\\" slash\\\\ tab\\t\\n\\r\\b\\f\\/\",\"draft\":false");
        yes(new TvUpdateClient(new Fake().json(" \n [" + escaped + "] \t")).check(0).version.equals("1.2.3"));
        yes(new TvUpdateClient(new Fake().json("[" + valid.replace("v1.2.3", "1.2.3") + "]")).check(0) != null);
        String invalid = valid.replace("\"draft\":false", "\"draft\":true") + ","
                + valid.replace("\"prerelease\":false", "\"prerelease\":true") + ","
                + release("1.2.3", asset("1.2.3", 3) + "," + asset("1.2.3", 3)) + ","
                + release("1.2.3", asset("1.2.3", 0)) + ","
                + release("1.2.3", asset("1.2.3", TvUpdateClient.MAX_APK_BYTES + 1)) + ","
                + valid.replace("nu11signal-tv-", "nu11signal-desktop-") + ","
                + valid.replace("github.com/wahh-22", "github.com/attacker") + ","
                + valid.replace("v1.2.3", "v1.2.3-beta") + ","
                + valid.replace("\"draft\":false,", "") + ","
                + valid.replace("\"size\":3", "\"size\":3.0");
        yes(new TvUpdateClient(new Fake().json("[" + invalid + "]")).check(0) == null);
        String page = "[" + String.join(",", java.util.Collections.nCopies(30, release("9.0.0", ""))) + "]";
        f = new Fake().json(page).json("[" + valid + "," + release("2.0.0", asset("2.0.0", 3)) + "]");
        yes(new TvUpdateClient(f).check(0).version.equals("2.0.0"));
        yes(f.opens == 2 && f.closes == 2);
        fails(() -> new TvUpdateClient(new Fake().json(page).json(page).json(page)).check(0));
        fails(() -> new TvUpdateClient(new Fake().json(page)).check(0));
        fails(() -> new TvUpdateClient(new Fake().json("[" + String.join(",", java.util.Collections.nCopies(31, valid)) + "]")).check(0));
        for (String json : Arrays.asList("{}", "[", "[] junk", "[{\"x\":1,\"x\":2}]", "[01]", "[NaN]",
                "[{},]", "[{\"x\":true,}]", "[1.]", "[1e]", "[+1]", "[\"\\q\"]", "[\"line\nfeed\"]")) {
            fails(() -> new TvUpdateClient(new Fake().json(json)).check(0));
        }
        String invalidHex = "[" + valid.replace("\"draft\":false", "\"note\":\"\\uＦＦＦＦ\",\"draft\":false") + "]";
        fails(() -> new TvUpdateClient(new Fake().json(invalidHex)).check(0));
        fails(() -> new TvUpdateClient(new Fake().add(403, null, new byte[0], 0)).check(0));
        fails(() -> new TvUpdateClient(new Fake().add(302, "https://evil.test", new byte[0], 0)).check(0));
        fails(() -> new TvUpdateClient(new Fake().add(200, null, new byte[TvUpdateClient.MAX_PAGE_BYTES + 1], -1)).check(0));
        fails(() -> new TvUpdateClient(new Fake().add(200, null, new byte[]{(byte) 0xff}, -1)).check(0));
        fails(() -> new TvUpdateClient(new Fake().add(200, null, "[]".getBytes(StandardCharsets.UTF_8), 3)).check(0));
        String nested = "[".repeat(34) + "0" + "]".repeat(34);
        fails(() -> new TvUpdateClient(new Fake().json(nested)).check(0));
        String many = "[" + valid.replace("\"draft\":false", "\"note\":[" + String.join(",", java.util.Collections.nCopies(100001, "null")) + "],\"draft\":false") + "]";
        fails(() -> new TvUpdateClient(new Fake().json(many)).check(0));
    }
    private static void downloads() throws Exception {
        File dir = new File(System.getProperty("test.work"), "downloads");
        yes(dir.mkdir());
        File sentinel = new File(dir, "keep.apk");
        Files.write(sentinel.toPath(), new byte[]{42});
        for (String address : Arrays.asList("http://github.com/wahh-22/nu11signal/releases/download/v1.2.3/nu11signal-tv-1.2.3.apk",
                "https://evil.test/a.apk", "https://github.com/other/nu11signal/releases/download/v1.2.3/nu11signal-tv-1.2.3.apk",
                "https://user@github.com/wahh-22/nu11signal/releases/download/v1.2.3/nu11signal-tv-1.2.3.apk",
                url("1.2.3") + "?x=1", url("1.2.3") + "#fragment", url("1.2.4"),
                url("1.2.3").replace("github.com", "127.0.0.1"), url("1.2.3").replace("github.com", "github.com:444"))) {
            Fake f = new Fake();
            fails(() -> new TvUpdateClient(f).download(candidate(address, 3), dir));
            yes(f.opens == 0 && dir.list().length == 1);
        }
        for (long size : new long[]{0, -1, TvUpdateClient.MAX_APK_BYTES + 1}) {
            fails(() -> new TvUpdateClient(new Fake()).download(candidate(url("1.2.3"), size), dir));
        }
        fails(() -> new TvUpdateClient(new Fake()).download(new TvUpdateRelease("1.2.3", 1, url("1.2.3"), 3), dir));
        byte[] bytes = {1, 2, 3};
        Fake f = new Fake().add(302, "https://release-assets.githubusercontent.com/release.bin?sig=x", new byte[0], 0)
                .add(200, null, bytes, 3).add(200, null, bytes, -1);
        TvUpdateClient client = new TvUpdateClient(f);
        File first = client.download(candidate(url("1.2.3"), 3), dir);
        File second = client.download(candidate(url("1.2.3"), 3), dir);
        yes(!first.equals(second) && first.getName().endsWith(".apk"));
        yes(Arrays.equals(Files.readAllBytes(first.toPath()), bytes));
        yes(Arrays.equals(Files.readAllBytes(second.toPath()), bytes));
        yes(f.closes == 3 && first.getCanonicalFile().getParentFile().equals(dir.getCanonicalFile()));
        int baseline = dir.list().length;
        for (String redirect : Arrays.asList("https://evil.test/a", "http://objects.githubusercontent.com/a",
                "https://objects.githubusercontent.com.evil.test/a", "https://user@objects.githubusercontent.com/a",
                "https://objects.githubusercontent.com:444/a", "https://127.0.0.1/a", "/other", "https://github.com/other/repo/a")) {
            Fake rejected = new Fake().add(302, redirect, new byte[0], 0);
            fails(() -> new TvUpdateClient(rejected).download(candidate(url("1.2.3"), 3), dir));
            yes(rejected.opens == 1 && rejected.closes == 1 && dir.list().length == baseline);
        }
        for (Fake bad : Arrays.asList(new Fake().add(200, null, bytes, 4),
                new Fake().add(200, null, new byte[]{1}, -1), new Fake().add(200, null, new byte[]{1,2,3,4}, -1),
                new Fake().add(404, null, bytes, 3))) {
            fails(() -> new TvUpdateClient(bad).download(candidate(url("1.2.3"), 3), dir));
            yes(bad.closes == 1 && dir.list().length == baseline);
        }
        Fake loop = new Fake();
        for (int i = 0; i < 7; i++) loop.add(302, "https://objects.githubusercontent.com/a", new byte[0], 0);
        fails(() -> new TvUpdateClient(loop).download(candidate(url("1.2.3"), 3), dir));
        yes(loop.opens <= 6 && loop.opens == loop.closes && dir.list().length == baseline);
        TvUpdateClient.Transport broken = address -> new TvUpdateClient.Response(200, null, -1, new InputStream() {
            public int read() throws IOException { throw new IOException("read failed"); }
        }, () -> {});
        fails(() -> new TvUpdateClient(broken).download(candidate(url("1.2.3"), 3), dir));
        yes(dir.list().length == baseline);
        TvUpdateClient.Transport cancel = address -> new TvUpdateClient.Response(200, null, -1, new InputStream() {
            public int read() { Thread.currentThread().interrupt(); return 1; }
        }, () -> {});
        fails(() -> new TvUpdateClient(cancel).download(candidate(url("1.2.3"), 3), dir));
        yes(Thread.interrupted() && dir.list().length == baseline);
        Thread.currentThread().interrupt();
        Fake untouched = new Fake();
        fails(() -> new TvUpdateClient(untouched).check(0));
        yes(Thread.interrupted() && untouched.opens == 0);
        Thread.currentThread().interrupt();
        fails(() -> new TvUpdateClient(untouched).download(candidate(url("1.2.3"), 3), dir));
        yes(Thread.interrupted() && untouched.opens == 0);
        yes(Arrays.equals(Files.readAllBytes(sentinel.toPath()), new byte[]{42}));
    }
    private static void elapsedBounds() throws Exception {
        long[] now = {0};
        File dir = new File(System.getProperty("test.work"), "elapsed");
        yes(dir.mkdir());
        TvUpdateClient.Transport slow = address -> new TvUpdateClient.Response(200, null, -1, new InputStream() {
            public int read() { now[0] = 601000000000L; return 1; }
        }, () -> {});
        fails(() -> new TvUpdateClient(slow, () -> now[0]).download(candidate(url("1.2.3"), 3), dir));
        yes(dir.list().length == 0);
        now[0] = 0;
        fails(() -> new TvUpdateClient(slow, () -> now[0]).check(0));
    }
    public static void main(String[] args) throws Exception {
        versions(); discovery(); downloads(); elapsedBounds();
        System.out.println("TvUpdateClient tests passed: " + assertions + " assertions");
    }
}
