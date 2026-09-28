// Command soul-king is a Cyberpunk 2077 style terminal radio for Apple Music.
//
// By default it starts the signed MusicKit helper (see helper.Locate); with
// --demo it runs against an in-process simulated player instead.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"soulking/internal/helper"
	"soulking/internal/playback"
	"soulking/internal/playback/demo"
	"soulking/internal/radio"
)

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
	// The UI closes the player on quit; closing again is harmless and
	// covers every other exit path.
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := helper.Start(ctx, helper.Options{Path: path})
	if err != nil {
		return nil, fmt.Errorf("start helper: %w", err)
	}
	return client, nil
}
