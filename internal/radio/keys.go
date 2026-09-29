package radio

// Key names as reported by tea.KeyPressMsg.String.
const (
	keyUp      = "up"
	keyUpAlt   = "k"
	keyDown    = "down"
	keyDownAlt = "j"
	keyEnter   = "enter"
	keySpace   = "space"
	keyNext    = "n"
	keyPrev    = "p"
	keyLeft    = "left"
	keyRight   = "right"
	// keySeekBack and keySeekForward seek anywhere; keySeekBackAlt and
	// keySeekForwardAlt wherever typing does not take them.
	keySeekBack       = "shift+left"
	keySeekForward    = "shift+right"
	keySeekBackAlt    = ","
	keySeekForwardAlt = "."
	// keyExpand toggles the full-width player anywhere, the SEARCH input
	// included; keyExpandAlt wherever typing does not take it.
	keyExpand    = "ctrl+f"
	keyExpandAlt = "f"
	keySearch    = "/"
	keyTab       = "tab"
	keyEsc       = "esc"
	keyQuit      = "q"
	keyRetry     = "r"
	keyCtrlC     = "ctrl+c"
	// keyDelete and keyDeleteAlt delete the selected recent search.
	keyDelete    = "delete"
	keyDeleteAlt = "ctrl+d"
)

// hint is one footer key legend entry.
type hint struct{ key, label string }

// playerHints are shown in priority order; the footer drops entries from
// the end (keeping quit) when the terminal is too narrow.
var playerHints = []hint{
	{"SPACE", "PLAY/PAUSE"},
	{"N/P", "NEXT/PREV"},
	{",/.", "SEEK"},
	{"→", "PLAYER"},
	{"F", "EXPAND"},
	{"/", "SCAN"},
	{"ENTER", "TUNE"},
	{"J/K", "MOVE"},
	{"Q", "QUIT"},
}

// playerFocusHints replace the view's hints while the player has the
// focus: ←→ walk the buttons (or seek on the bar), ↑↓ switch between the
// buttons and the bar.
var playerFocusHints = []hint{
	{"←→", "SELECT"},
	{"ENTER", "PRESS"},
	{"↑↓", "BAR"},
	{"ESC", "LIST"},
	{"F", "EXPAND"},
	{"SPACE", "PLAY/PAUSE"},
	{"Q", "QUIT"},
}

// artistHints replace playerHints on an artist page: enter plays a top
// song, opens an album or playlist, or expands the ABOUT notes (MORE).
var artistHints = []hint{
	{"ENTER", "SELECT/MORE"},
	{"J/K", "MOVE"},
	{"SPACE", "PLAY/PAUSE"},
	{"ESC", "BACK"},
	{"N/P", "NEXT/PREV"},
	{"/", "SCAN"},
	{"Q", "QUIT"},
}

// resultsHints replace playerHints on the RESULTS page: enter opens the
// selected artist, album, song or playlist.
var resultsHints = []hint{
	{"ENTER", "OPEN"},
	{"J/K", "MOVE"},
	{"SPACE", "PLAY/PAUSE"},
	{"ESC", "BACK"},
	{"N/P", "NEXT/PREV"},
	{"/", "SCAN"},
	{"Q", "QUIT"},
}

// trackHints replace playerHints on an album, song or playlist page: enter
// plays from the selected track or expands the notes (MORE).
var trackHints = []hint{
	{"ENTER", "PLAY/MORE"},
	{"J/K", "MOVE"},
	{"SPACE", "PLAY/PAUSE"},
	{"ESC", "BACK"},
	{"N/P", "NEXT/PREV"},
	{"/", "SCAN"},
	{"Q", "QUIT"},
}

// searchHints replace playerHints while the search view is open: typing
// goes to the input, so only non-text keys act.
var searchHints = []hint{
	{"ENTER", "SELECT"},
	{"↑↓", "MOVE"},
	{"TAB", "STATIONS"},
	{"ESC", "BACK"},
	{"CTRL+C", "QUIT"},
}

// recentHints replace searchHints while a recent term is selected: the
// delete key comes right after enter, so it is among the last to go when
// the footer is too narrow. recentDeleteKeys names both delete keys when
// the whole footer fits, else only DEL.
var recentHints = []hint{
	{"ENTER", "SELECT"},
	{recentDeleteKeys, "DROP"},
	{"↑↓", "MOVE"},
	{"TAB", "STATIONS"},
	{"ESC", "BACK"},
	{"CTRL+C", "QUIT"},
}

const recentDeleteKeys = "DEL/CTRL+D"
