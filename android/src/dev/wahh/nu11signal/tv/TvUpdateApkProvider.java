package dev.wahh.nu11signal.tv;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.util.concurrent.ConcurrentHashMap;

/** No directory listing, writes, arbitrary files, or persistent grants. */
public final class TvUpdateApkProvider extends ContentProvider {
    static final String AUTHORITY = TvUpdateGuard.PACKAGE + ".update-apk";
    private static final ConcurrentHashMap<String, File> APPROVED = new ConcurrentHashMap<>();

    static Uri authorize(File snapshot) {
        APPROVED.put(snapshot.getName(), snapshot);
        return new Uri.Builder().scheme("content").authority(AUTHORITY)
                .appendPath(snapshot.getName()).build();
    }

    static void forget(File snapshot) { APPROVED.remove(snapshot.getName(), snapshot); }

    @Override public boolean onCreate() { return true; }

    private File resolve(Uri uri, String mode) throws IOException {
        if (!"content".equals(uri.getScheme()) || !AUTHORITY.equals(uri.getAuthority())
                || uri.getQuery() != null || uri.getFragment() != null
                || !uri.getEncodedPath().equals(uri.getPath()))
            throw new IOException("Invalid updater APK URI.");
        File file = TvUpdateGuard.resolveReadPath(new File(getContext().getFilesDir(), "updates"),
                uri.getPath(), mode);
        if (!file.equals(APPROVED.get(file.getName())))
            throw new IOException("APK is not an authorized validated snapshot. Validate again.");
        return file;
    }

    @Override public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        try {
            return ParcelFileDescriptor.open(resolve(uri, mode), ParcelFileDescriptor.MODE_READ_ONLY);
        } catch (IOException e) {
            FileNotFoundException failure = new FileNotFoundException(e.getMessage());
            failure.initCause(e);
            throw failure;
        }
    }

    @Override public String getType(Uri uri) {
        requireReadable(uri);
        return "application/vnd.android.package-archive";
    }

    private void requireReadable(Uri uri) {
        try { resolve(uri, "r"); }
        catch (IOException e) { throw new IllegalArgumentException(e.getMessage(), e); }
    }

    @Override public Cursor query(Uri uri, String[] projection, String selection,
            String[] selectionArgs, String sortOrder) {
        requireReadable(uri);
        String[] columns = projection == null
                ? new String[]{OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE} : projection;
        MatrixCursor cursor = new MatrixCursor(columns);
        Object[] row = new Object[columns.length];
        for (int i = 0; i < columns.length; i++) {
            if (OpenableColumns.DISPLAY_NAME.equals(columns[i])) row[i] = "nu11signal-update.apk";
        }
        cursor.addRow(row);
        return cursor;
    }

    @Override public Uri insert(Uri uri, ContentValues values) { throw readOnly(); }
    @Override public int update(Uri uri, ContentValues values, String where, String[] args) { throw readOnly(); }
    @Override public int delete(Uri uri, String where, String[] args) { throw readOnly(); }
    private UnsupportedOperationException readOnly() {
        return new UnsupportedOperationException("Updater APK provider is read-only.");
    }
}
