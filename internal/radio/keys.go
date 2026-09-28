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

var searchHints = []hint{
	{"ENTER", "SCAN"},
	{"ESC", "CANCEL"},
	{"CTRL+C", "QUIT"},
}
