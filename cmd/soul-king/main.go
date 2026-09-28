// Command soul-king is a Cyberpunk 2077 style terminal radio for Apple Music.
//
// By default it starts the signed MusicKit helper, found only through
// $SOULKING_HELPER (an absolute path) or next to the soul-king binary (see
// helper.Locate), never in the working directory; with --demo it runs
// against an in-process simulated player instead.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	tea "charm.land/bubbletea/v2"

	"soulking/internal/helper"
	"soulking/internal/playback"
	"soulking/internal/playback/demo"
	"soulking/internal/radio"
)

// startTimeout bounds launching the helper until it reports ready.
const startTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "soul-king:", err)
		os.Exit(1)
	}
}

func run() error {
	demoMode := flag.Bool("demo", false, "run against a simulated player (no Apple Music, no sound)")
	flag.Parse()

	player, err := openPlayer(*demoMode)
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
