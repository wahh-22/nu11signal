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
	keyBack    = "left"
	keyForward = "right"
	keySearch  = "/"
	keyTab     = "tab"
	keyEsc     = "esc"
	keyQuit    = "q"
	keyRetry   = "r"
	keyCtrlC   = "ctrl+c"
)

// hint is one footer key legend entry.
type hint struct{ key, label string }

// playerHints are shown in priority order; the footer drops entries from
// the end (keeping quit) when the terminal is too narrow.
var playerHints = []hint{
	{"SPACE", "PLAY/PAUSE"},
	{"N/P", "NEXT/PREV"},
	{"←→", "SEEK"},
	{"/", "SCAN"},
	{"ENTER", "TUNE"},
	{"TAB", "LIST"},
	{"J/K", "MOVE"},
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

// searchHints replace playerHints while the search view is open: typing
// goes to the input, so only non-text keys act.
var searchHints = []hint{
	{"ENTER", "SELECT"},
	{"↑↓", "MOVE"},
	{"TAB", "STATIONS"},
	{"ESC", "BACK"},
	{"CTRL+C", "QUIT"},
}
