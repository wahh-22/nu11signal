package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/androidtv"
	"github.com/wahh-22/nu11signal/internal/config"
	"github.com/wahh-22/nu11signal/internal/history"
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/update"
)

// playAndroidTV gives Bubble Tea a real terminal while the app owns only
// pipes. The socket reader also owns resize and sign-in controls; losing
// that connection cancels startup or the UI. Authorization runs alongside
// the UI so Back in the sign-in view leaves a usable, unauthorized TUI.
// Neither a browser subprocess nor a local-files backend is opened here.
func playAndroidTV(socket string, calm bool, d deps) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	r, err := androidtv.Open(ctx, socket)
	if err != nil {
		return err
	}
	defer r.Close()
	go func() {
		select {
		case <-r.Bridge.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	relay := r.Relay(ctx, os.Stdin, d.stdout)
	defer func() { cancel(); r.Close(); <-relay }()
	d.openWebPlayer = func(start context.Context) (playback.Player, error) {
		// The startup bound from openPlayer must also stop on an app exit or
		// interrupt. This context does not own the player's later lifetime.
		start, cancelStart := context.WithCancel(start)
		defer cancelStart()
		stop := context.AfterFunc(ctx, cancelStart)
		defer stop()
		return r.NewPlayer(start)
	}
	d.runUI = func(p playback.Player, recents history.Recents, settings config.Source, calm bool, updates update.Checker) error {
		player := p.(*androidtv.Player)
		loginCtx, stop := context.WithCancel(ctx)
		signedIn := make(chan error, 1)
		go func() { signedIn <- r.SignIn(loginCtx, player) }()
		err := runUIWithOptions(p, recents, settings, calm, updates, tea.WithInput(r.Slave), tea.WithOutput(r.Slave), tea.WithContext(ctx))
		stop()
		if loginErr := <-signedIn; loginErr != nil && !errors.Is(loginErr, context.Canceled) {
			fmt.Fprintln(d.stderr, "nu11signal: Android sign-in:", loginErr)
		}
		return err
	}
	return play(options{androidTV: socket}, calm, d)
}
