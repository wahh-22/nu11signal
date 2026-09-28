package helper

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HelperEnv overrides the helper executable path.
const HelperEnv = "SOULKING_HELPER"

// bundleRelPath is the executable inside the signed helper bundle.
var bundleRelPath = filepath.Join("SoulKingHelper.app", "Contents", "MacOS", "soulking-helper")

// ErrHelperNotFound is wrapped when no helper executable can be found.
var ErrHelperNotFound = errors.New("soulking-helper not found")

// Locate finds the helper executable, in order: $SOULKING_HELPER; the
// bundle next to the running executable; ./build in the working directory.
func Locate() (string, error) {
	return locator{getenv: os.Getenv, executable: os.Executable, workDir: os.Getwd}.locate()
}

type locator struct {
	getenv     func(string) string
	executable func() (string, error)
	workDir    func() (string, error)
}

func (l locator) locate() (string, error) {
	// An explicit override must work; falling back would hide the mistake.
	if p := l.getenv(HelperEnv); p != "" {
		if isExecutable(p) {
			return p, nil
		}
		return "", fmt.Errorf("%w: %s=%s is not an executable file", ErrHelperNotFound, HelperEnv, p)
	}
	var tried []string
	if exe, err := l.executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		tried = append(tried, filepath.Join(filepath.Dir(exe), bundleRelPath))
	}
	if wd, err := l.workDir(); err == nil {
		tried = append(tried, filepath.Join(wd, "build", bundleRelPath))
	}
	for _, p := range tried {
		if isExecutable(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: set %s or build the helper; tried: %s",
		ErrHelperNotFound, HelperEnv, strings.Join(tried, ", "))
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
