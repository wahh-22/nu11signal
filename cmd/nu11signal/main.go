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
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/demo"
	"github.com/wahh-22/nu11signal/internal/radio"
)

// version is stamped by release builds with -ldflags "-X main.version=x.y.z".
var version = "dev"

// startTimeout bounds launching the helper until it reports ready.
const startTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "nu11signal:", err)
		os.Exit(1)
	}
}

func run() error {
	opts, err := parseFlags(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		os.Exit(2) // the flag package already printed the error and usage
	}
	if opts.version {
		printVersion(os.Stdout)
		return nil
	}

	player, err := openPlayer(opts.demo)
	if err != nil {
		return err
	}
	// The UI closes the player on quit but stops waiting after a timeout;
	// this Close (idempotent) covers every other exit path and, once the
	// terminal is restored, waits for the helper, which bounds its own
	// shutdown and kills a helper that does not exit.
	defer player.Close()

	model := radio.New(player, radio.Options{Seed: uint64(time.Now().UnixNano())})
	_, err = tea.NewProgram(model).Run()
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return err
	}
	return nil
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

func openPlayer(demoMode bool) (playback.Player, error) {
	if demoMode {
		return demo.New(demo.Options{}), nil
	}
	path, err := helper.Locate()
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
	client, err := helper.Start(ctx, helper.Options{Path: path})
	if err != nil {
		return nil, fmt.Errorf("start helper: %w", err)
	}
	return client, nil
}
