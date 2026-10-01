package radio

// Key names as reported by tea.KeyPressMsg.String.
const (
	keyUp    = "up"
	keyDown  = "down"
	keyEnter = "enter"
	keySpace = "space"
	keyNext  = "n"
	keyPrev  = "p"
	keyLeft  = "left"
	keyRight = "right"
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
	// keyVolumeUp, keyVolumeUpAlt, keyVolumeUpLetter and their down
	// counterparts step the volume wherever typing does not take them;
	// keyVolumeUpAnywhere and keyVolumeDownAnywhere anywhere, the SEARCH
	// input included.
	keyVolumeUp           = "+"
	keyVolumeUpAlt        = "="
	keyVolumeUpLetter     = "k"
	keyVolumeDown         = "-"
	keyVolumeDownLetter   = "j"
	keyVolumeUpAnywhere   = "shift+up"
	keyVolumeDownAnywhere = "shift+down"
	keySearch             = "/"
	keyTab                = "tab"
	keyEsc                = "esc"
	keyQuit               = "q"
	keyRetry              = "r"
	keyCtrlC              = "ctrl+c"
	// keyLove toggles the favorite state of the selected song, or of the
	// song playing; keyAdd adds it to a library playlist. Both act
	// wherever typing does not take them (on SEARCH, on a song row).
	keyLove = "l"
	keyAdd  = "a"
	// keyAlbum opens the SONG view (the album holding it) of the selected
	// song row of SEARCH or RESULTS, where enter plays the row's list
	// instead. It acts wherever typing does not take it (on SEARCH, on a
	// song row).
	keyAlbum = "g"
	// keyLoop cycles the repeat mode, OFF, ALL, ONE, as the LOOP button
	// does, wherever the player keys act.
	keyLoop = "o"
	// keyEffects toggles the signal effects (glitch bursts and content
	// intros) on the playlists and the pages, not where typing takes it.
	keyEffects = "x"
	// keyDelete and keyDeleteAlt delete the selected recent search.
	keyDelete    = "delete"
	keyDeleteAlt = "ctrl+d"
)

// hint is one footer key legend entry.
type hint struct{ key, label string }

// playerHints are shown in priority order; the footer drops entries from
// the end (keeping quit) when the terminal is too narrow.
var playerHints = []hint{
	{"ENTER", "OPEN"},
	{"/", "SCAN"},
	{"SPACE", "PLAY/PAUSE"},
	{"→", "PLAYER"},
	{",/.", "SEEK"},
	{"N/P", "NEXT/PREV"},
	{"J/K", "VOL"},
	{"F", "EXPAND"},
	{"L", "LOVE"},
	{"A", "ADD"},
	{"O", "LOOP"},
	{"↑↓", "MOVE"},
	{"X", "FX"},
	{"Q", "QUIT"},
}

// playerFocusHints replace the view's hints while the player has the
// focus: ←→ walk a row of buttons (or seek on the bar), ↑↓ switch between
// the bar and the rows of buttons. F restores the expanded player; while the list
// behind is the SEARCH input (typing), q types there, so ctrl+c quits.
func playerFocusHints(expanded, typing bool) []hint {
	expand, quit := hint{"F", "EXPAND"}, hint{"Q", "QUIT"}
	if expanded {
		expand.label = "RESTORE"
	}
	if typing {
		quit.key = "CTRL+C"
	}
	return []hint{
		{"←→", "SELECT"},
		{"ENTER", "PRESS"},
		{"↑↓", "ROW"},
		{"ESC", "LIST"},
		expand,
		{"SPACE", "PLAY/PAUSE"},
		{"L", "LOVE"},
		{"O", "LOOP"},
		quit,
	}
}

// tabsFocusHints replace the view's hints while the nav tabs have the
// focus: ←→ walk the tabs, enter opens one, ↓ or esc go back down. While
// the view is SEARCH (typing), q types there, so ctrl+c quits.
func tabsFocusHints(typing bool) []hint {
	quit := hint{"Q", "QUIT"}
	if typing {
		quit.key = "CTRL+C"
	}
	return []hint{
		{"←→", "SELECT"},
		{"ENTER", "OPEN"},
		{"↓", "RETURN"},
		{"SPACE", "PLAY/PAUSE"},
		{"J/K", "VOL"},
		quit,
	}
}

// artistHints replace playerHints on an artist page: enter plays a top
// song, opens an album or playlist, or expands the ABOUT notes (MORE).
// On the pages under the root, L and A come after ESC BACK, so an
// 80-column footer drops them first.
var artistHints = []hint{
	{"ENTER", "SELECT/MORE"},
	{"↑↓", "MOVE"},
	{"SPACE", "PLAY/PAUSE"},
	{"ESC", "BACK"},
	{"L", "LOVE"},
	{"A", "ADD"},
	{"N/P", "NEXT/PREV"},
	{"/", "SCAN"},
	{"Q", "QUIT"},
}

// resultsHints replace playerHints on the RESULTS page: enter opens the
// selected artist, album or playlist, or plays the selected song with the
// rest of its list; g opens a song's album.
var resultsHints = []hint{
	{"ENTER", "OPEN/PLAY"},
	{"↑↓", "MOVE"},
	{"SPACE", "PLAY/PAUSE"},
	{"ESC", "BACK"},
	{"L", "LOVE"},
	{"A", "ADD"},
	{"G", "ALBUM"},
	{"N/P", "NEXT/PREV"},
	{"/", "SCAN"},
	{"Q", "QUIT"},
}

// trackHints replace playerHints on an album, song or playlist page: enter
// plays from the selected track or expands the notes (MORE).
var trackHints = []hint{
	{"ENTER", "PLAY/MORE"},
	{"↑↓", "MOVE"},
	{"SPACE", "PLAY/PAUSE"},
	{"ESC", "BACK"},
	{"L", "LOVE"},
	{"A", "ADD"},
	{"N/P", "NEXT/PREV"},
	{"/", "SCAN"},
	{"Q", "QUIT"},
}

// pickerHints replace the view's hints while ADD TO PLAYLIST is open, and
// nameHints while NEW PLAYLIST takes a name: every key but enter and esc
// is typed there, so ctrl+c quits.
var pickerHints = []hint{
	{"ENTER", "ADD"},
	{"↑↓", "MOVE"},
	{"ESC", "CANCEL"},
	{"SPACE", "PLAY/PAUSE"},
	{"Q", "QUIT"},
}

var nameHints = []hint{
	{"ENTER", "CREATE"},
	{"ESC", "CANCEL"},
	{"CTRL+C", "QUIT"},
}

// searchSongHints replace searchHints while a song row is selected, where
// l, a and g act instead of being typed: enter plays the song with the
// songs listed after it, g opens its album.
var searchSongHints = []hint{
	{"ENTER", "PLAY"},
	{"↑↓", "MOVE"},
	{"TAB", "PLAYLISTS"},
	{"ESC", "BACK"},
	{"L", "LOVE"},
	{"A", "ADD"},
	{"G", "ALBUM"},
	{"CTRL+C", "QUIT"},
}

// searchHints replace playerHints while the search view is open: typing
// goes to the input, so only non-text keys act.
var searchHints = []hint{
	{"ENTER", "SELECT"},
	{"↑↓", "MOVE"},
	{"TAB", "PLAYLISTS"},
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
	{"TAB", "PLAYLISTS"},
	{"ESC", "BACK"},
	{"CTRL+C", "QUIT"},
}

const recentDeleteKeys = "DEL/CTRL+D"
