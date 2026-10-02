// Command nu11signal is a neon terminal radio for Apple
// Music and the computer's own music files.
//
// On macOS it starts the signed MusicKit helper, found only through
// $NU11SIGNAL_HELPER (an absolute path) or next to the nu11signal binary (see
// helper.Locate), never in the working directory, and joins it with the
// local files (see package composite). Without the helper, off macOS, or
// with --local, it plays the local files alone. Those are the folders of
// "music_dirs" in config.json (default ~/Music), scanned in the background
// after startup. With --demo it runs against an in-process simulated
// player instead. --calm (or
// NU11SIGNAL_CALM=1) starts with the signal effects off; x toggles them.
// --version prints the release version stamped at link time: under the
// Braille logo on a terminal, the bare version line otherwise. A release
// build checks at every launch for a newer release (see internal/update) unless
// NU11SIGNAL_NO_UPDATE_CHECK=1 or "update_check": false in config.json.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/config"
	"github.com/wahh-22/nu11signal/internal/helper"
	"github.com/wahh-22/nu11signal/internal/history"
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/composite"
	"github.com/wahh-22/nu11signal/internal/playback/demo"
	"github.com/wahh-22/nu11signal/internal/playback/local"
	"github.com/wahh-22/nu11signal/internal/radio"
	"github.com/wahh-22/nu11signal/internal/update"
)

// version is stamped by release builds with -ldflags "-X main.version=x.y.z".
var version = "dev"

// startTimeout bounds launching the helper until it reports ready.
const startTimeout = 10 * time.Second

// calmEnv set to 1 starts with the signal effects off, as --calm does.
const calmEnv = "NU11SIGNAL_CALM"

// noUpdateCheckEnv set to 1 turns the update check off.
const noUpdateCheckEnv = "NU11SIGNAL_NO_UPDATE_CHECK"

func main() {
	os.Exit(run(os.Args[1:], deps{
		stdout: os.Stdout,
		stderr: os.Stderr,
		stdoutTerminal: func() bool {
			return isTerminal(os.Stdout)
		},
		goos:         runtime.GOOS,
		openLocal:    openLocal,
		locateHelper: helper.Locate,
		startHelper:  startHelper,
		runUI:        runUI,
	}))
}

// deps are run's side effects, injected so its exit paths are testable.
type deps struct {
	stdout, stderr io.Writer
	// stdoutTerminal reports whether stdout is a terminal (nil: it is
	// not), which --version draws the emblem on.
	stdoutTerminal func() bool
	// goos is the operating system (runtime.GOOS): the helper is macOS
	// only.
	goos string
	// openLocal returns the local files backend for the folders dirs,
	// without waiting for their scan.
	openLocal func(dirs []string) playback.Player
	// locateHelper finds the helper executable (helper.Locate).
	locateHelper func() (string, error)
	// startHelper launches the helper at path; ctx bounds only startup.
	startHelper func(ctx context.Context, path string) (playback.Player, error)
	// runUI runs the radio UI against player until the user quits; recents
	// stores recent searches (nil keeps them in memory only); settings is
	// the settings file (nil keeps the defaults); calm starts the signal
	// effects off; updates checks for a newer release (nil never checks).
	runUI func(player playback.Player, recents history.Recents, settings config.Source, calm bool, updates update.Checker) error
}

// run executes the command with args (without the program name) and
// returns the process exit code: 0 on success, --help, or an interrupt;
// 2 for a command-line error (the flag package already printed it and the
// usage); 1 for any other failure, reported on stderr.
func run(args []string, d deps) int {
	opts, err := parseFlags(args, d.stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}
	if opts.version {
		printVersion(d.stdout, d.stdoutTerminal != nil && d.stdoutTerminal())
		return 0
	}
	calm := opts.calm || os.Getenv(calmEnv) == "1"
	if err := play(opts, calm, d); err != nil {
		fmt.Fprintln(d.stderr, "nu11signal:", err)
		return 1
	}
	return 0
}

func play(opts options, calm bool, d deps) error {
	demoMode := opts.demo
	// The settings come first: they name the music folders.
	settings := openConfig()
	player, err := openPlayer(opts, settings, d)
	if err != nil {
		return err
	}
	// The UI closes the player on quit but stops waiting after a timeout;
	// this Close (idempotent) covers every other exit path and, once the
	// terminal is restored, waits for the helper, which bounds its own
	// shutdown and kills a helper that does not exit.
	defer player.Close()

	if err := d.runUI(player, openRecents(demoMode), settings, calm, openUpdates(demoMode, settings)); err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return err
	}
	return nil
}

// openRecents returns the recent-searches file in the user's config
// directory. The demo, and a system without a config directory, keep
// recent searches in memory instead (nil).
func openRecents(demoMode bool) history.Recents {
	if demoMode {
		return nil
	}
	path, err := history.DefaultPath()
	if err != nil {
		return nil
	}
	return history.NewFile(path)
}

// openConfig returns the settings file in the user's config directory,
// read at startup and written when SETTINGS chooses a theme, by the demo
// too (it only chooses how the UI looks); nil, the defaults for the
// session, on a system without a config directory.
func openConfig() config.Source {
	path, err := config.DefaultPath()
	if err != nil {
		return nil
	}
	return config.NewFile(path)
}

// openUpdates returns the update checker: GitHub's latest release, asked
// at every launch, the last answer kept in update.json beside the settings
// file for an offline launch. nil, no check,
// for the demo (a simulated session stays offline), a build whose
// version does not compare ("dev"), NU11SIGNAL_NO_UPDATE_CHECK=1,
// "update_check": false in the settings (a settings file that cannot be
// read leaves the check on; the UI reports the file), or a system without
// a config directory to cache in.
func openUpdates(demoMode bool, settings config.Source) update.Checker {
	if demoMode || !update.Valid(version) || os.Getenv(noUpdateCheckEnv) == "1" {
		return nil
	}
	if settings != nil {
		if c, err := settings.Load(); err == nil && !c.UpdateCheckOn() {
			return nil
		}
	}
	path, err := config.DefaultPath()
	if err != nil {
		return nil
	}
	return update.NewCached(update.NewGitHub(version), update.CachePath(path))
}

// upgradeCommand is the command that upgrades this binary (see
// update.UpgradeCommand), from its path with symlinks resolved; "" when
// it cannot be found.
func upgradeCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return update.UpgradeCommand(exe)
}

func runUI(player playback.Player, recents history.Recents, settings config.Source, calm bool, updates update.Checker) error {
	model := radio.New(player, radio.Options{
		Seed:    uint64(time.Now().UnixNano()),
		Recents: recents,
		Config:  settings,
		Effects: !calm,
		Updates: updates,
		Version: version,
		Upgrade: upgradeCommand(),
	})
	_, err := tea.NewProgram(model, tea.WithFPS(radio.RenderFPS)).Run()
	return err
}

// options are the parsed command-line flags.
type options struct {
	demo    bool
	local   bool
	calm    bool
	version bool
}

// parseFlags parses args (without the program name); errors and usage go
// to output.
func parseFlags(args []string, output io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("nu11signal", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.BoolVar(&opts.demo, "demo", false, "run against a simulated player (no Apple Music, no sound)")
	fs.BoolVar(&opts.local, "local", false, "play the local music files only (music_dirs in config.json, default ~/Music), without Apple Music")
	fs.BoolVar(&opts.calm, "calm", false, "start with the signal effects (glitches, text glitches, alerts) off; also "+calmEnv+"=1")
	fs.BoolVar(&opts.version, "version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	return opts, nil
}

// printVersion writes the version: on a terminal, the Braille logo and
// the version (v-prefixed when it is a number) under the bars, in the
// wordmark's column (see logoWithVersion); elsewhere the bare version line,
// which scripts and release.sh read.
func printVersion(w io.Writer, terminal bool) {
	if !terminal {
		fmt.Fprintln(w, version)
		return
	}
	shown := version
	if shown != "" && shown[0] >= '0' && shown[0] <= '9' {
		shown = "v" + shown
	}
	io.WriteString(w, logoWithVersion(radio.EmblemRows(), shown))
}

// logoWithVersion draws rows with shown under the wordmark, in its column
// (see wordmarkAt), in a row added for it when the wordmark reaches the
// last row. Art without a wordmark to align to cannot host it: then shown
// goes on its own line under the art, so the version is never dropped.
func logoWithVersion(rows []string, shown string) string {
	rows = append([]string(nil), rows...)
	if col, below, ok := wordmarkAt(rows); ok {
		if below == len(rows) {
			rows = append(rows, "")
		}
		under := []rune(rows[below])
		under = append(under, []rune(strings.Repeat(" ", max(col-len(under), 0)))...)
		rows[below] = string(under[:col]) + shown
	} else {
		rows = append(rows, shown)
	}
	var b strings.Builder
	for _, row := range rows {
		b.WriteString(strings.TrimRight(row, " ") + "\n")
	}
	return b.String()
}

// wordmarkAt finds the wordmark in the logo's rows: col, the first
// column after the blank column that parts it from the head, and below,
// the first row under everything drawn from col on (len(rows) when the
// wordmark reaches the last row). ok is false when no blank column parts
// a head from a wordmark (or there is no art).
func wordmarkAt(rows []string) (col, below int, ok bool) {
	cells := make([][]rune, len(rows))
	width := 0
	for i, row := range rows {
		cells[i] = []rune(row)
		width = max(width, len(cells[i]))
	}
	drawn := func(x int) bool {
		for _, r := range cells {
			if x < len(r) && r[x] != ' ' {
				return true
			}
		}
		return false
	}
	for x := 1; x < width; x++ {
		if !drawn(x-1) && drawn(x) {
			col, ok = x, true
			break
		}
	}
	if !ok {
		return 0, 0, false
	}
	for i, r := range cells {
		if col < len(r) && strings.TrimSpace(string(r[col:])) != "" {
			below = i + 1
		}
	}
	return col, below, true
}

// isTerminal reports whether f is a terminal (a character device).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// openPlayer returns the player: the demo; the local files alone (with
// --local, off macOS, or when the helper is not found); or Apple Music
// through the helper joined with the local files. A helper that is found
// but fails to start is an error: it is installed, so it is broken.
func openPlayer(opts options, settings config.Source, d deps) (playback.Player, error) {
	if opts.demo {
		return demo.New(demo.Options{}), nil
	}
	dirs := musicDirs(settings)
	if opts.local || d.goos != "darwin" {
		return composite.New(nil, d.openLocal(dirs)), nil
	}
	path, err := d.locateHelper()
	if errors.Is(err, helper.ErrHelperNotFound) {
		return composite.New(nil, d.openLocal(dirs)), nil
	}
	if err != nil {
		return nil, err
	}
	// ctx bounds only the startup handshake (helper.Start does not tie the
	// process to it), so cancelling it on return is correct and leaks
	// nothing: the helper lives until the player is closed. An interrupt
	// during startup aborts it and kills the half-started helper.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	apple, err := d.startHelper(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("start helper: %w", err)
	}
	return composite.New(apple, d.openLocal(dirs)), nil
}

// defaultMusicDir is the music folder when the settings name none, in the
// home directory.
const defaultMusicDir = "Music"

// musicDirs are the folders of "music_dirs" in the settings, ~ and ~/ the
// home directory, or ~/Music when it names none (or cannot be read: the UI
// reports the file).
func musicDirs(settings config.Source) []string {
	var dirs []string
	if settings != nil {
		if c, err := settings.Load(); err == nil {
			dirs = c.MusicDirs
		}
	}
	home, _ := os.UserHomeDir()
	if len(dirs) == 0 {
		if home == "" {
			return nil
		}
		return []string{filepath.Join(home, defaultMusicDir)}
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		switch {
		case d == "~" && home != "":
			d = home
		case strings.HasPrefix(d, "~/") && home != "":
			d = filepath.Join(home, d[2:])
		}
		out = append(out, d)
	}
	return out
}

// openLocal returns the local files player, empty until the scan of dirs,
// started here in the background, ends: reading every file's tags must
// not hold up the UI. A scan that fails leaves it empty.
func openLocal(dirs []string) playback.Player {
	p := local.New(nil, local.Options{})
	go func() {
		if lib, err := local.Scan(context.Background(), dirs); err == nil {
			p.SetLibrary(lib)
		}
	}()
	return p
}

func startHelper(ctx context.Context, path string) (playback.Player, error) {
	// The helper's diagnostics are appended to its log file as well as
	// kept (their tail) to explain a crash; without the file, only the
	// tail.
	opts := helper.Options{Path: path}
	if log := openDefaultHelperLog(); log != nil {
		opts.Stderr = log
	}
	// Return a nil interface, not a typed nil *helper.Client, on failure.
	client, err := helper.Start(ctx, opts)
	if err != nil {
		return nil, err
	}
	return client, nil
}
