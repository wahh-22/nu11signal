package helper

import (
	"os"
	"strings"
	"testing"
)

// TestUnknownOutcomeMarkMatchesTheHelper pins the phrase OutcomeUnknown
// looks for to the Swift helper's timeout messages for library writes, so
// rewording one side fails here instead of silently disabling the guard.
func TestUnknownOutcomeMarkMatchesTheHelper(t *testing.T) {
	src, err := os.ReadFile("../../helper/Sources/Nu11SignalProtocol/LibraryEdit.swift")
	if err != nil {
		t.Fatalf("read helper source: %v", err)
	}
	for _, write := range []string{"may or may not have been created", "may or may not have been added"} {
		if !strings.Contains(write, unknownOutcomeMark) {
			t.Fatalf("%q does not carry the mark %q", write, unknownOutcomeMark)
		}
		if !strings.Contains(string(src), write) {
			t.Errorf("LibraryEdit.swift no longer says %q; keep it in step with unknownOutcomeMark", write)
		}
	}
}
