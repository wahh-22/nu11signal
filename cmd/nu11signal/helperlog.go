package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// helperLogMax is the size past which the helper log is rotated when the
// app starts: the previous log moves to helper.log.1, replacing an older
// one, so the logs never hold much more than twice this.
const helperLogMax = 1 << 20

// helperLogPath is the helper's log file under the user's home directory,
// where Console.app lists it: ~/Library/Logs/nu11signal/helper.log.
func helperLogPath(home string) string {
	return filepath.Join(home, "Library", "Logs", "nu11signal", "helper.log")
}

// openHelperLog opens the log at path for appending, creating its
// directory (0700) and the file (0600) as needed. A log larger than limit
// bytes is first rotated to path+".1".
func openHelperLog(path string, limit int64) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil && info.Size() > limit {
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
}

// openDefaultHelperLog opens the helper log at helperLogPath and marks the
// start of this session in it; nil when it cannot be opened, in which case
// the helper's diagnostics are only kept for a crash report. The file stays
// open for the life of the process: the helper may still write while it
// shuts down.
func openDefaultHelperLog() *os.File {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	f, err := openHelperLog(helperLogPath(home), helperLogMax)
	if err != nil {
		return nil
	}
	fmt.Fprintf(f, "--- nu11signal %s started %s ---\n", version, time.Now().Format(time.RFC3339))
	return f
}
