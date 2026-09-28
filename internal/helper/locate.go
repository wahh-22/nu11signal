package helper

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HelperEnv overrides the helper executable path. It must be absolute.
const HelperEnv = "SOULKING_HELPER"

// bundleRelPath is the executable inside the signed helper bundle.
var bundleRelPath = filepath.Join("SoulKingHelper.app", "Contents", "MacOS", "soulking-helper")

// ErrHelperNotFound is wrapped when no helper executable can be found.
var ErrHelperNotFound = errors.New("soulking-helper not found")

// Locate finds the helper executable. Only trusted locations are searched,
// never the working directory (a helper planted there would run with the
// user's Apple Music access). In order:
//
//  1. $SOULKING_HELPER, which must be an absolute path to an executable;
//  2. <exeDir>/SoulKingHelper.app/... (bundle next to the binary);
//  3. <exeDir>/../libexec/SoulKingHelper.app/... (Homebrew-style install);
//  4. <exeDir>/../build/SoulKingHelper.app/... (dev layout: bin/ + build/).
//
// exeDir is the directory of the running executable with symlinks resolved.
func Locate() (string, error) {
	return locator{getenv: os.Getenv, executable: os.Executable}.locate()
}

type locator struct {
	getenv     func(string) string
	executable func() (string, error)
}

func (l locator) locate() (string, error) {
	// An explicit override must work; falling back would hide the mistake.
	if p := l.getenv(HelperEnv); p != "" {
		if !filepath.IsAbs(p) {
			return "", fmt.Errorf("%w: %s=%s must be an absolute path", ErrHelperNotFound, HelperEnv, p)
		}
		if isExecutable(p) {
			return p, nil
		}
		return "", fmt.Errorf("%w: %s=%s is not an executable file", ErrHelperNotFound, HelperEnv, p)
	}

	exeDir, err := l.executableDir()
	if err != nil {
		return "", fmt.Errorf("%w: set %s: %v", ErrHelperNotFound, HelperEnv, err)
	}
	tried := []string{
		filepath.Join(exeDir, bundleRelPath),
		filepath.Join(exeDir, "..", "libexec", bundleRelPath),
		filepath.Join(exeDir, "..", "build", bundleRelPath),
	}
	for _, p := range tried {
		if isExecutable(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: set %s or build the helper; tried: %s",
		ErrHelperNotFound, HelperEnv, strings.Join(tried, ", "))
}

// executableDir is the absolute, symlink-free directory of the running
// executable, so a symlinked binary still finds the helper installed
// beside the real one.
func (l locator) executableDir() (string, error) {
	exe, err := l.executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine own executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("cannot resolve own executable: %w", err)
	}
	if !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("own executable path %q is not absolute", resolved)
	}
	return filepath.Dir(resolved), nil
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
