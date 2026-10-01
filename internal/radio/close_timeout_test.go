package radio

import (
	"testing"

	"github.com/wahh-22/nu11signal/internal/helper"
)

// TestQuitWaitsAtLeastAsLongAsTheHelperClient keeps the end of the timeout
// chain (see helper.DefaultCloseTimeout) ordered: the UI never gives up on
// Player.Close before the client would kill the helper, so the helper's
// fade out is not cut short by the program exiting.
func TestQuitWaitsAtLeastAsLongAsTheHelperClient(t *testing.T) {
	if defaultCloseTimeout < helper.DefaultCloseTimeout {
		t.Fatalf("defaultCloseTimeout %s < helper.DefaultCloseTimeout %s", defaultCloseTimeout, helper.DefaultCloseTimeout)
	}
}
