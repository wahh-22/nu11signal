package androidtv

import "time"

// Search timing includes the WebView UI queue and JSON/socket round trip,
// not just Apple's network latency. Desktop keeps its shorter defaults.
const (
	CatalogTimeout = 30 * time.Second
	SearchTimeout  = 45 * time.Second
	SearchDebounce = 750 * time.Millisecond
)
