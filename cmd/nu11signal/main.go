// Command nu11signal is a Cyberpunk 2077 style terminal radio for Apple Music.
//
// By default it starts the signed MusicKit helper, found only through
// $NU11SIGNAL_HELPER (an absolute path) or next to the nu11signal binary (see
// helper.Locate), never in the working directory; with --demo it runs
// against an in-process simulated player instead. --version prints the
// release version stamped at link time.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	tea "charm.land/bubbletea/v2"

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

func main() {
	os.Exit(run(os.Args[1:], deps{
		stdout:       os.Stdout,
		stderr:       os.Stderr,
		locateHelper: helper.Locate,
		startHelper:  startHelper,
		runUI:        runUI,
	}))
}

// deps are run's side effects, injected so its exit paths are testable.
type deps struct {
	stdout, stderr io.Writer
	// locateHelper finds the helper executable (helper.Locate).
	locateHelper func() (string, error)
	// startHelper launches the helper at path; ctx bounds only startup.
	startHelper func(ctx context.Context, path string) (playback.Player, error)
	// runUI runs the radio UI against player until the user quits; recents
	// stores recent searches (nil keeps them in memory only).
	runUI func(player playback.Player, recents history.Recents) error
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
		printVersion(d.stdout)
		return 0
	}
	if err := play(opts.demo, d); err != nil {
		fmt.Fprintln(d.stderr, "nu11signal:", err)
		return 1
	}
	return 0
}

func play(demoMode bool, d deps) error {
	player, err := openPlayer(demoMode, d)
	if err != nil {
		return err
	}
	// The UI closes the player on quit but stops waiting after a timeout;
	// this Close (idempotent) covers every other exit path and, once the
	// terminal is restored, waits for the helper, which bounds its own
	// shutdown and kills a helper that does not exit.
	defer player.Close()

	if err := d.runUI(player, openRecents(demoMode)); err != nil && !errors.Is(err, tea.ErrInterrupted) {
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

func runUI(player playback.Player, recents history.Recents) error {
	model := radio.New(player, radio.Options{Seed: uint64(time.Now().UnixNano()), Recents: recents})
	_, err := tea.NewProgram(model).Run()
	return err
}

// options are the parsed command-line flags.
type options struct {
	demo    bool
	version bool
}

// parseFlags parses args (without the program name); errors and usage go
// to output.
func parseFlags(args []string, output io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("nu11signal", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.BoolVar(&opts.demo, "demo", false, "run against a simulated player (no Apple Music, no sound)")
	fs.BoolVar(&opts.version, "version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	return opts, nil
}

func printVersion(w io.Writer) {
	fmt.Fprintln(w, version)
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
	// Return a nil interface, not a typed nil *helper.Client, on failure.
	client, err := helper.Start(ctx, helper.Options{Path: path})
	if err != nil {
		return nil, err
	}
	return client, nil
}
