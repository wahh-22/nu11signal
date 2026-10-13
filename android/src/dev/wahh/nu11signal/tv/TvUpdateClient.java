package dev.wahh.nu11signal.tv;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.InterruptedIOException;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;
import java.net.URISyntaxException;
import java.net.URL;
import java.nio.ByteBuffer;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/** Blocking, offline-testable native update discovery and private download transport.
 * No installer or UI dependency. Call from an executor, never the Android main thread.
 */
public final class TvUpdateClient {
    public static final long MAX_APK_BYTES = 256L * 1024 * 1024;
    public static final int MAX_PAGE_BYTES = 2 * 1024 * 1024;
    private static final int PAGE_SIZE = 30, MAX_PAGES = 3, MAX_REDIRECTS = 5;
    private static final int TIMEOUT_MS = 10000;
    private static final long DISCOVERY_MS = 90000, DOWNLOAD_MS = 600000;
    private static final String API = "https://api.github.com/repos/wahh-22/nu11signal/releases?per_page=30&page=";
    private final Transport transport;
    private final java.util.function.LongSupplier monotonicNanos;

    /** Injectable trusted transport: no automatic redirects, finite connect/read timeouts.
     * open must return promptly on interruption or within its timeout. Ownership transfers
     * to the client, including for non-200 responses. Production uses HTTPS URLConnection.
     */
    public interface Transport { Response open(String url) throws IOException; }

    public static final class Response implements AutoCloseable {
        public final int status;
        public final String location;
        public final long contentLength;
        public final InputStream body;
        private final Runnable disconnect;
        public Response(int status, String location, long contentLength, InputStream body, Runnable disconnect) {
            this.status = status;
            this.location = location;
            this.contentLength = contentLength;
            this.body = body;
            this.disconnect = disconnect;
        }
        public void close() throws IOException {
            try { if (body != null) body.close(); }
            finally { disconnect.run(); }
        }
    }

    public TvUpdateClient() { this(TvUpdateClient::openHttps); }
    public TvUpdateClient(Transport transport) { this(transport, System::nanoTime); }
    /** Deterministic clock seam; supplied clock must be monotonic nanoseconds. */
    public TvUpdateClient(Transport transport, java.util.function.LongSupplier monotonicNanos) {
        if (transport == null || monotonicNanos == null) throw new IllegalArgumentException("Missing transport or clock");
        this.transport = transport;
        this.monotonicNanos = monotonicNanos;
    }

    /** Null means a completely traversed bounded query found no newer valid TV release.
     * A full final page, malformed page, network error or cancellation is not 'up to date'.
     */
    public TvUpdateRelease check(long installedVersionCode) throws IOException {
        long deadline = deadline(DISCOVERY_MS);
        guard(deadline);
        if (installedVersionCode < 0) throw new IOException("Invalid installed versionCode");
        TvUpdateRelease best = null;
        for (int page = 1; page <= MAX_PAGES; page++) {
            byte[] bytes;
            guard(deadline);
            try (Response response = transport.open(API + page)) {
                guard(deadline);
                requireBody(response);
                if (response.contentLength > MAX_PAGE_BYTES) throw new IOException("Release page too large");
                ByteArrayOutputStream output = new ByteArrayOutputStream();
                copy(response.body, output, MAX_PAGE_BYTES, deadline);
                bytes = output.toByteArray();
                if (response.contentLength >= 0 && bytes.length != response.contentLength) {
                    throw new IOException("Release page length mismatch");
                }
            }
            Object decoded = new Json(decodeUtf8(bytes)).parse();
            guard(deadline);
            if (!(decoded instanceof List)) throw new IOException("Expected release list");
            List<?> releases = (List<?>) decoded;
            if (releases.size() > PAGE_SIZE) throw new IOException("Oversized release list");
            for (Object item : releases) {
                if (!(item instanceof Map)) throw new IOException("Invalid release record");
                TvUpdateRelease candidate = candidate((Map<?, ?>) item);
                if (candidate != null && candidate.versionCode > installedVersionCode
                        && (best == null || candidate.versionCode > best.versionCode)) best = candidate;
            }
            guard(deadline);
            if (releases.size() < PAGE_SIZE) return best;
        }
        throw new IOException("Release discovery incomplete: page bound reached");
    }

    /** Each successful call returns its own .apk snapshot. This does not verify signing;
     * the installer guard and Android package installer are mandatory next boundaries.
     */
    public File download(TvUpdateRelease release, File privateDirectory) throws IOException {
        long deadline = deadline(DOWNLOAD_MS);
        guard(deadline);
        validateRelease(release);
        if (privateDirectory == null || !privateDirectory.isDirectory()) throw new IOException("Missing private directory");
        // No release-controlled pathname; createTempFile atomically reserves a fresh name.
        File part = File.createTempFile("tv-update-", ".part", privateDirectory);
        File apk = null;
        boolean complete = false;
        try {
            String address = release.url;
            for (int redirects = 0; ; redirects++) {
                guard(deadline);
                try (Response response = transport.open(address)) {
                    guard(deadline);
                    if (isRedirect(response.status)) {
                        if (redirects >= MAX_REDIRECTS) throw new IOException("Too many APK redirects");
                        address = redirectUrl(response.location, release);
                        continue;
                    }
                    requireBody(response);
                    if (response.contentLength >= 0 && response.contentLength != release.size) {
                        throw new IOException("APK Content-Length mismatch");
                    }
                    try (OutputStream output = Files.newOutputStream(part.toPath())) {
                        if (copy(response.body, output, release.size, deadline) != release.size) {
                            throw new IOException("Truncated APK");
                        }
                    }
                }
                break;
            }
            guard(deadline);
            // Reserve a unique final name too: concurrent downloads cannot overwrite it.
            apk = File.createTempFile("tv-update-", ".apk", privateDirectory);
            Files.move(part.toPath(), apk.toPath(), java.nio.file.StandardCopyOption.REPLACE_EXISTING);
            complete = true;
            return apk;
        } finally {
            if (!complete) {
                Files.deleteIfExists(part.toPath());
                if (apk != null) Files.deleteIfExists(apk.toPath());
            }
        }
    }

    private static TvUpdateRelease candidate(Map<?, ?> record) {
        if (!Boolean.FALSE.equals(record.get("draft")) || !Boolean.FALSE.equals(record.get("prerelease"))) return null;
        if (!(record.get("tag_name") instanceof String) || !(record.get("assets") instanceof List)) return null;
        String tag = (String) record.get("tag_name");
        String version = tag.startsWith("v") ? tag.substring(1) : tag;
        long code;
        try { code = TvUpdateRelease.versionCode(version); }
        catch (IllegalArgumentException invalid) { return null; }
        String name = "nu11signal-tv-" + version + ".apk";
        Map<?, ?> match = null;
        for (Object item : (List<?>) record.get("assets")) {
            if (!(item instanceof Map)) return null;
            Map<?, ?> asset = (Map<?, ?>) item;
            if (name.equals(asset.get("name"))) {
                if (match != null) return null; // Duplicate exact assets are ambiguous, not offers.
                match = asset;
            }
        }
        if (match == null || !(match.get("size") instanceof Long) || !(match.get("browser_download_url") instanceof String)) return null;
        TvUpdateRelease release = new TvUpdateRelease(version, code, (String) match.get("browser_download_url"), (Long) match.get("size"));
        try { validateRelease(release); return release; }
        catch (IOException invalid) { return null; }
    }

    private static void validateRelease(TvUpdateRelease release) throws IOException {
        if (release == null) throw new IOException("Missing release");
        try {
            if (TvUpdateRelease.versionCode(release.version) != release.versionCode) throw new IOException("Advertised versionCode mismatch");
        } catch (IllegalArgumentException invalid) { throw new IOException("Invalid release version", invalid); }
        if (release.size <= 0 || release.size > MAX_APK_BYTES) throw new IOException("APK size out of bounds");
        if (!isAssetUrl(safeUri(release.url), release)) throw new IOException("Not an official TV asset URL");
    }

    private static URI safeUri(String address) throws IOException {
        try {
            if (address == null) throw new IOException("Missing URL");
            URI uri = new URI(address);
            if (!"https".equals(uri.getScheme()) || uri.getRawUserInfo() != null || uri.getRawFragment() != null
                    || uri.getHost() == null || (uri.getPort() != -1 && uri.getPort() != 443)) {
                throw new IOException("Unsafe HTTPS URL");
            }
            return uri;
        } catch (URISyntaxException invalid) { throw new IOException("Invalid URL", invalid); }
    }

    private static boolean isAssetUrl(URI uri, TvUpdateRelease release) {
        String base = "/wahh-22/nu11signal/releases/download/";
        String suffix = "/nu11signal-tv-" + release.version + ".apk";
        return "github.com".equals(uri.getHost()) && uri.getRawQuery() == null
                && (uri.getRawPath().equals(base + release.version + suffix)
                || uri.getRawPath().equals(base + "v" + release.version + suffix));
    }

    private static String redirectUrl(String address, TvUpdateRelease release) throws IOException {
        URI uri = safeUri(address);
        String host = uri.getHost();
        if (!isAssetUrl(uri, release) && !"release-assets.githubusercontent.com".equals(host)
                && !"objects.githubusercontent.com".equals(host)) throw new IOException("Untrusted APK redirect");
        return address;
    }

    private static boolean isRedirect(int status) {
        return status == 301 || status == 302 || status == 303 || status == 307 || status == 308;
    }
    private static void requireBody(Response response) throws IOException {
        if (response.status != 200 || response.body == null) throw new IOException("Unexpected HTTP response: " + response.status);
    }
    private long deadline(long milliseconds) { return monotonicNanos.getAsLong() + milliseconds * 1000000L; }
    private void guard(long deadline) throws IOException {
        if (Thread.currentThread().isInterrupted()) throw new InterruptedIOException("Update cancelled");
        if (monotonicNanos.getAsLong() - deadline >= 0) throw new IOException("Update elapsed time limit exceeded");
    }
    private long copy(InputStream input, OutputStream output, long limit, long deadline) throws IOException {
        byte[] buffer = new byte[16384];
        long total = 0;
        while (true) {
            guard(deadline);
            int count = input.read(buffer);
            guard(deadline);
            if (count < 0) return total;
            total += count;
            if (total > limit) throw new IOException("Response byte limit exceeded");
            output.write(buffer, 0, count);
        }
    }
    private static Response openHttps(String address) throws IOException {
        HttpURLConnection connection = (HttpURLConnection) new URL(address).openConnection();
        connection.setInstanceFollowRedirects(false);
        connection.setConnectTimeout(TIMEOUT_MS);
        connection.setReadTimeout(TIMEOUT_MS);
        connection.setUseCaches(false);
        connection.setRequestProperty("Accept", address.startsWith(API) ? "application/vnd.github+json" : "application/octet-stream");
        connection.setRequestProperty("Accept-Encoding", "identity");
        connection.setRequestProperty("User-Agent", "nu11signal-tv-updater");
        try {
            int status = connection.getResponseCode();
            InputStream body = status == 200 ? connection.getInputStream() : connection.getErrorStream();
            return new Response(status, connection.getHeaderField("Location"), connection.getContentLengthLong(), body, connection::disconnect);
        } catch (IOException | RuntimeException failure) {
            connection.disconnect();
            throw failure;
        }
    }
    private static String decodeUtf8(byte[] bytes) throws IOException {
        try { return StandardCharsets.UTF_8.newDecoder().decode(ByteBuffer.wrap(bytes)).toString(); }
        catch (CharacterCodingException invalid) { throw new IOException("Invalid release JSON encoding", invalid); }
    }

    /** Small strict JSON decoder, avoiding Android-only org.json or downloaded JVM jars.
     * Input bytes, nesting and value count are bounded. Duplicate keys are rejected.
     * Only integer JSON tokens can represent an advertised asset size.
     */
    private static final class Json {
        private final String text;
        private int pos, values;
        Json(String text) { this.text = text; }
        Object parse() throws IOException {
            Object value = value(0);
            space();
            if (pos != text.length()) throw bad();
            return value;
        }
        private IOException bad() { return new IOException("Malformed release JSON at offset " + pos); }
        private void space() { while (pos < text.length() && " \r\n\t".indexOf(text.charAt(pos)) >= 0) pos++; }
        private boolean take(char c) {
            space();
            if (pos < text.length() && text.charAt(pos) == c) { pos++; return true; }
            return false;
        }
        private Object value(int depth) throws IOException {
            if (depth > 32 || ++values > 100000) throw bad();
            space();
            if (pos >= text.length()) throw bad();
            char c = text.charAt(pos);
            if (c == '"') return string();
            if (take('{')) {
                Map<String, Object> map = new HashMap<>();
                if (take('}')) return map;
                do {
                    space();
                    String key = string();
                    if (!take(':') || map.containsKey(key)) throw bad();
                    map.put(key, value(depth + 1));
                } while (take(','));
                if (!take('}')) throw bad();
                return map;
            }
            if (take('[')) {
                List<Object> list = new ArrayList<>();
                if (take(']')) return list;
                do { list.add(value(depth + 1)); } while (take(','));
                if (!take(']')) throw bad();
                return list;
            }
            for (String literal : new String[]{"true", "false", "null"}) {
                if (text.startsWith(literal, pos)) {
                    pos += literal.length();
                    return literal.equals("null") ? null : Boolean.valueOf(literal);
                }
            }
            int start = pos;
            if (c == '-') pos++;
            if (pos >= text.length()) throw bad();
            if (text.charAt(pos) == '0') pos++;
            else {
                if (text.charAt(pos) < '1' || text.charAt(pos) > '9') throw bad();
                digits();
            }
            boolean integer = true;
            if (pos < text.length() && text.charAt(pos) == '.') {
                integer = false; pos++; requireDigits();
            }
            if (pos < text.length() && (text.charAt(pos) == 'e' || text.charAt(pos) == 'E')) {
                integer = false; pos++;
                if (pos < text.length() && (text.charAt(pos) == '+' || text.charAt(pos) == '-')) pos++;
                requireDigits();
            }
            String number = text.substring(start, pos);
            // Non-integral or overflowing numbers are valid JSON, but never valid sizes.
            if (integer) {
                try { return Long.valueOf(number); } catch (NumberFormatException overflow) { return number; }
            }
            return number;
        }
        private void digits() { while (pos < text.length() && text.charAt(pos) >= '0' && text.charAt(pos) <= '9') pos++; }
        private void requireDigits() throws IOException {
            int start = pos; digits(); if (start == pos) throw bad();
        }
        private String string() throws IOException {
            if (pos >= text.length() || text.charAt(pos++) != '"') throw bad();
            StringBuilder result = new StringBuilder();
            while (pos < text.length()) {
                char c = text.charAt(pos++);
                if (c == '"') return result.toString();
                if (c < 32) throw bad();
                if (c == '\\') {
                    if (pos >= text.length()) throw bad();
                    c = text.charAt(pos++);
                    switch (c) {
                        case '"': case '\\': case '/': break;
                        case 'b': c = '\b'; break;
                        case 'f': c = '\f'; break;
                        case 'n': c = '\n'; break;
                        case 'r': c = '\r'; break;
                        case 't': c = '\t'; break;
                        case 'u':
                            if (pos + 4 > text.length()) throw bad();
                            int code = 0;
                            for (int i = 0; i < 4; i++) {
                                char hex = text.charAt(pos++);
                                int digit = "0123456789abcdef".indexOf(hex);
                                if (digit < 0) digit = "0123456789ABCDEF".indexOf(hex);
                                if (digit < 0) throw bad();
                                code = code * 16 + digit;
                            }
                            c = (char) code; break;
                        default: throw bad();
                    }
                }
                result.append(c);
            }
            throw bad();
        }
    }
}
