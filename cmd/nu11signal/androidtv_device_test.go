package main

import "testing"

// The TV updates by installing a newer APK, never by a desktop upgrade
// command, so the desktop release check stays off there.
func TestAndroidTVHasNoDesktopUpdateCheck(t *testing.T) {
	opts := androidRadioOptions(nil, nil, false, nil)
	if opts.Updates != nil || opts.Upgrade != "" {
		t.Fatalf("Updates = %v, Upgrade = %q; want no update check", opts.Updates, opts.Upgrade)
	}
}

// Go fixes time.Local to UTC on Android; the app passes the device zone.
func TestAndroidTVDeviceZone(t *testing.T) {
	if loc := androidDeviceZone("America/Bogota"); loc == nil || loc.String() != "America/Bogota" {
		t.Fatalf("zone = %v, want America/Bogota", loc)
	}
	for _, name := range []string{"", "Not/AZone"} {
		if loc := androidDeviceZone(name); loc != nil {
			t.Fatalf("zone(%q) = %v, want nil (keep the default)", name, loc)
		}
	}
}
