//go:build linux || android

package androidtv

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

func TestResizeNotifiesTUI(t *testing.T) {
	master, slave, err := openPTY()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{master: master, Slave: slave}
	defer r.Close()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	defer signal.Stop(signals)
	r.onMessage(webplayer.BridgeMessage{Type: "resize", Cols: 100, Rows: 30})
	select {
	case <-signals:
	case <-time.After(time.Second):
		t.Fatal("resize did not notify Bubble Tea")
	}
}
