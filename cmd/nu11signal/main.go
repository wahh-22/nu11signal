// Command nu11signal is a Cyberpunk 2077 style terminal radio for Apple Music.
//
// By default it starts the signed MusicKit helper, found only through
// $NU11SIGNAL_HELPER (an absolute path) or next to the nu11signal binary (see
// helper.Locate), never in the working directory; with --demo it runs
// against an in-process simulated player instead. --calm (or
// NU11SIGNAL_CALM=1) starts with the signal effects off; x toggles them.
// --version prints the release version stamped at link time: beside the
// null emblem on a terminal, the bare version line otherwise.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/config"
	"github.com/wahh-22/nu11signal/internal/helper"
	"github.com/wahh-22/nu11signal/internal/history"
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/demo"
	"github.com/wahh-22/nu11signal/internal/radio"
)

// version is stamped by release builds with -ldflags "-X main.version=x.y.z".
var version = "dev"

// startTimeout bounds launching the helper until it reports ready.
const startTimeout = 10 * time.Second

// calmEnv set to 1 starts with the signal effects off, as --calm does.
const calmEnv = "NU11SIGNAL_CALM"

func main() {
	os.Exit(run(os.Args[1:], deps{
		stdout: os.Stdout,
		stderr: os.Stderr,
		stdoutTerminal: func() bool {
			return isTerminal(os.Stdout)
		},
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
	// locateHelper finds the helper executable (helper.Locate).
	locateHelper func() (string, error)
	// startHelper launches the helper at path; ctx bounds only startup.
	startHelper func(ctx context.Context, path string) (playback.Player, error)
	// runUI runs the radio UI against player until the user quits; recents
	// stores recent searches (nil keeps them in memory only); settings is
	// the settings file (nil keeps the defaults); calm starts the signal
	// effects off.
	runUI func(player playback.Player, recents history.Recents, settings config.Source, calm bool) error
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
	if err := play(opts.demo, calm, d); err != nil {
		fmt.Fprintln(d.stderr, "nu11signal:", err)
		return 1
	}
	return 0
}

func play(demoMode, calm bool, d deps) error {
	player, err := openPlayer(demoMode, d)
	if err != nil {
		return err
	}
	// The UI closes the player on quit but stops waiting after a timeout;
	// this Close (idempotent) covers every other exit path and, once the
	// terminal is restored, waits for the helper, which bounds its own
	// shutdown and kills a helper that does not exit.
	defer player.Close()

	if err := d.runUI(player, openRecents(demoMode), openConfig(), calm); err != nil && !errors.Is(err, tea.ErrInterrupted) {
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

func runUI(player playback.Player, recents history.Recents, settings config.Source, calm bool) error {
	model := radio.New(player, radio.Options{Seed: uint64(time.Now().UnixNano()), Recents: recents, Config: settings, Effects: !calm})
	_, err := tea.NewProgram(model, tea.WithFPS(radio.RenderFPS)).Run()
	return err
}

// options are the parsed command-line flags.
type options struct {
	demo    bool
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
	fs.BoolVar(&opts.calm, "calm", false, "start with the signal effects (glitches, text glitches, alerts) off; also "+calmEnv+"=1")
	fs.BoolVar(&opts.version, "version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	return opts, nil
}

// printVersion writes the version: on a terminal, the compact Braille
// null emblem with NU11SIGNAL and the version (v-prefixed when it is a
// number) beside it on its middle rows, 3 cells right of it; elsewhere
// the bare version line, which scripts and release.sh read.
func printVersion(w io.Writer, terminal bool) {
	if !terminal {
		fmt.Fprintln(w, version)
		return
	}
	shown := version
	if shown != "" && shown[0] >= '0' && shown[0] <= '9' {
		shown = "v" + shown
	}
	text := []string{"NU11SIGNAL", shown}
	var b strings.Builder
	rows := radio.EmblemRows()
	top := (len(rows) - len(text)) / 2
	for i, row := range rows {
		if t := i - top; t >= 0 && t < len(text) {
			row += "   " + text[t]
		}
		b.WriteString(strings.TrimRight(row, " ") + "\n")
	}
	io.WriteString(w, b.String())
}

// isTerminal reports whether f is a terminal (a character device).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func openPlayer(demoMode bool, d deps) (playback.Player, error) {
	if demoMode {
		return demo.New(demo.Options{}), nil
	}
	path, err := d.locateHelper()
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
	player, err := d.startHelper(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("start helper: %w", err)
	}
	return player, nil
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
