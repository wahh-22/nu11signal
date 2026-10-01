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
	// keyHelp opens the KEYS overlay (helpGroups) and closes it, wherever
	// q quits: SEARCH and the NEW PLAYLIST name type it (see keysTyped).
	keyHelp = "?"
)

// helpEntry is one line of the KEYS overlay: the keys as shown, what
// they do, and the key names above it stands for.
type helpEntry struct {
	show, label string
	keys        []string
}

// helpGroup is a titled block of the KEYS overlay.
type helpGroup struct {
	name    string
	entries []helpEntry
}

// helpGroups is the one table of the KEYS overlay: every key above, in
// at least one entry (a test reads the constants of this file to check).
// Where typing takes a key, its entry names the alternative that acts.
var helpGroups = []helpGroup{
	{"PLAYBACK", []helpEntry{
		{"SPACE", "PLAY / PAUSE", []string{keySpace}},
		{"N / P", "NEXT / PREVIOUS SONG", []string{keyNext, keyPrev}},
		{", / .", "SEEK -10 S / +10 S", []string{keySeekBackAlt, keySeekForwardAlt}},
		{"SHIFT+← / →", "SEEK, ALSO IN SEARCH", []string{keySeekBack, keySeekForward}},
		{"J / K", "VOLUME DOWN / UP", []string{keyVolumeDownLetter, keyVolumeUpLetter}},
		{"- / + =", "VOLUME DOWN / UP", []string{keyVolumeDown, keyVolumeUp, keyVolumeUpAlt}},
		{"SHIFT+↑ / ↓", "VOLUME, ALSO IN SEARCH", []string{keyVolumeUpAnywhere, keyVolumeDownAnywhere}},
		{"O", "LOOP OFF / ALL / ONE", []string{keyLoop}},
		{"L", "LOVE THE SONG", []string{keyLove}},
		{"A", "ADD TO A PLAYLIST", []string{keyAdd}},
	}},
	{"NAVIGATION", []helpEntry{
		{"↑ / ↓", "MOVE, ↑ TO THE TABS", []string{keyUp, keyDown}},
		{"ENTER", "OPEN / PLAY / PRESS", []string{keyEnter}},
		{"ESC", "BACK / CANCEL", []string{keyEsc}},
		{"TAB", "PLAYLISTS ⇄ SEARCH", []string{keyTab}},
		{"/", "SEARCH", []string{keySearch}},
		{"→ / ←", "PLAYER FOCUS / LIST", []string{keyRight, keyLeft}},
		{"G", "OPEN THE SONG'S ALBUM", []string{keyAlbum}},
	}},
	{"VIEW", []helpEntry{
		{"F / CTRL+F", "EXPAND / RESTORE PLAYER", []string{keyExpandAlt, keyExpand}},
		{"X", "SIGNAL FX ON / OFF", []string{keyEffects}},
	}},
	{"SEARCH", []helpEntry{
		{"DEL / CTRL+D", "DROP A RECENT SEARCH", []string{keyDelete, keyDeleteAlt}},
	}},
	{"APP", []helpEntry{
		{"R", "RETRY A FAILED LOAD", []string{keyRetry}},
		{"Q / CTRL+C", "QUIT (CTRL+C IN TEXT)", []string{keyQuit, keyCtrlC}},
		{"?", "KEYS (THIS LIST)", []string{keyHelp}},
	}},
}

// helpHint is the footer's entry for keyHelp, kept with quit when the
// footer is too narrow (see fitHints). The footers where ? is typed
// (SEARCH, the NEW PLAYLIST name) leave it out.
var helpHint = hint{"?", "KEYS"}

// hint is one footer key legend entry.
type hint struct{ key, label string }

// playerHints are shown in priority order; the footer drops entries from
// the end (keeping KEYS and quit) when the terminal is too narrow.
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
	helpHint,
	{"Q", "QUIT"},
}

// playerFocusHints replace the view's hints while the player has the
// focus: ←→ walk a row of buttons (or seek on the bar), ↑↓ switch between
// the bar and the rows of buttons. F restores the expanded player; while the list
// behind is the SEARCH input (typing), q and ? type there, so ctrl+c
// quits and the KEYS hint is left out.
func playerFocusHints(expanded, typing bool) []hint {
	expand := hint{"F", "EXPAND"}
	if expanded {
		expand.label = "RESTORE"
	}
	return append([]hint{
		{"←→", "SELECT"},
		{"ENTER", "PRESS"},
		{"↑↓", "ROW"},
		{"ESC", "LIST"},
		expand,
		{"SPACE", "PLAY/PAUSE"},
		{"L", "LOVE"},
		{"O", "LOOP"},
	}, quitHints(typing)...)
}

// quitHints end a footer: KEYS and Q QUIT, or only CTRL+C QUIT where
// typing takes q and ?.
func quitHints(typing bool) []hint {
	if typing {
		return []hint{{"CTRL+C", "QUIT"}}
	}
	return []hint{helpHint, {"Q", "QUIT"}}
}

// tabsFocusHints replace the view's hints while the nav tabs have the
// focus: ←→ walk the tabs, enter opens one, ↓ or esc go back down. While
// the view is SEARCH (typing), q and ? type there, so ctrl+c quits.
func tabsFocusHints(typing bool) []hint {
	return append([]hint{
		{"←→", "SELECT"},
		{"ENTER", "OPEN"},
		{"↓", "RETURN"},
		{"SPACE", "PLAY/PAUSE"},
		{"J/K", "VOL"},
	}, quitHints(typing)...)
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
	helpHint,
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
	helpHint,
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
	helpHint,
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
	helpHint,
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
