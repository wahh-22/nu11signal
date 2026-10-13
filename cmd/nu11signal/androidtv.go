package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/androidtv"
	"github.com/wahh-22/nu11signal/internal/config"
	"github.com/wahh-22/nu11signal/internal/history"
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/radio"
	"github.com/wahh-22/nu11signal/internal/update"
)

// androidTypingHook coalesces queued transitions to the latest state. One
// worker preserves write order while the UI never waits for the socket.
func androidTypingHook(ctx context.Context, bridge *webplayer.Bridge) func(bool) {
	states := make(chan bool, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case show := <-states:
				sendCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				err := bridge.Send(sendCtx, webplayer.BridgeMessage{Type: "keyboard", Show: show})
				cancel()
				if err != nil {
					bridge.Close()
					return
				}
			}
		}
	}()
	return func(show bool) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		select {
		case states <- show:
		default:
			select {
			case <-states:
			default:
			}
			select {
			case states <- show:
			default:
			}
		}
	}
}

// androidRadioOptions keeps slower search timing out of ordinary player calls.
// The TV updates by installing a newer APK (docs/android-tv.md), so it gets
// no update check: the desktop release check would compare the APK with
// desktop releases and offer a desktop upgrade command.
func androidRadioOptions(recents history.Recents, settings config.Source, calm bool, typing func(bool)) radio.Options {
	return radio.Options{
		OnTypingChanged: typing,
		SearchTimeout:   androidtv.SearchTimeout,
		SearchDebounce:  androidtv.SearchDebounce,
		Seed:            uint64(time.Now().UnixNano()),
		Recents:         recents,
		Config:          settings,
		Effects:         !calm,
		Version:         version,
	}
}

// playAndroidTV gives Bubble Tea a real terminal while the app owns only
// pipes. The socket reader also owns resize and sign-in controls; losing
// that connection cancels startup or the UI. Authorization runs alongside
// the UI so Back in the sign-in view leaves a usable, unauthorized TUI.
// No browser subprocess is opened; play joins the app player with the
// private local library, whose audio device stays unopened until playback.
func playAndroidTV(socket string, calm bool, d deps) error {
	if loc := androidDeviceZone(os.Getenv("TZ")); loc != nil {
		time.Local = loc
	}
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
	// The boot splash shows from the first frame and holds while Apple
	// Music loads (see radio.Startup); runUI hands the program over to the
	// UI built on the opened player. Without that handover (the player did
	// not open) the splash quits before play's error is reported.
	next := make(chan radio.Model, 1)
	startup := tea.NewProgram(radio.NewStartup(radio.Options{Seed: uint64(time.Now().UnixNano()), Effects: !calm}, next),
		tea.WithInput(r.Slave), tea.WithOutput(r.Slave), tea.WithContext(ctx), tea.WithFPS(radio.RenderFPS))
	ui := make(chan error, 1)
	go func() {
		_, err := startup.Run()
		ui <- err
	}()
	handedOver := false
	defer func() {
		if !handedOver {
			close(next)
			<-ui
		}
	}()
	var player *androidtv.Player
	d.openWebPlayer = func(start context.Context) (playback.Player, error) {
		// The startup bound from openPlayer must also stop on an app exit or
		// interrupt. This context does not own the player's later lifetime.
		start, cancelStart := context.WithCancel(start)
		defer cancelStart()
		stop := context.AfterFunc(ctx, cancelStart)
		defer stop()
		var err error
		player, err = r.NewPlayer(start)
		if err != nil {
			return nil, err
		}
		return player, nil
	}
	d.runUI = func(p playback.Player, recents history.Recents, settings config.Source, calm bool, updates update.Checker) error {
		loginCtx, stop := context.WithCancel(ctx)
		signedIn := make(chan error, 1)
		go func() { signedIn <- r.SignIn(loginCtx, player) }()
		handedOver = true
		next <- radio.New(p, androidRadioOptions(recents, settings, calm, androidTypingHook(loginCtx, r.Bridge)))
		err := <-ui
		stop()
		if loginErr := <-signedIn; loginErr != nil && !errors.Is(loginErr, context.Canceled) {
			fmt.Fprintln(d.stderr, "nu11signal: Android sign-in:", loginErr)
		}
		return err
	}
	return play(options{androidTV: socket}, calm, d)
}

// androidDeviceZone loads the device time zone the app passes in TZ. Go
// fixes time.Local to UTC on Android and never reads TZ there, so the clock
// would otherwise show UTC. nil keeps the default: no zone, or one this
// device's zoneinfo does not know.
func androidDeviceZone(name string) *time.Location {
	if name == "" {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}
