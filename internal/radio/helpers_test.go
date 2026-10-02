package radio

import (
	"os"
	"testing"
)

// Test helpers shared by the view tests.

// shippedDefault is the default theme the app ships with, kept aside
// while the tests run on REDSHIFT.
var shippedDefault = defaultTheme

// TestMain pins REDSHIFT as the default theme: the goldens were drawn
// under it, and every test that changes the theme puts the default back
// (see useTheme).
func TestMain(m *testing.M) {
	defaultTheme = redshift
	applyTheme(defaultTheme)
	os.Exit(m.Run())
}

// linesOf drops the zones of a body renderer.
func linesOf(lines []string, _ zones) []string { return lines }
