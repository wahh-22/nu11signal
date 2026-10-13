package androidtv

import (
	"errors"
	"runtime"
	"testing"

	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

func TestPTYResize(t *testing.T) {
	master, slave, err := openPTY()
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("unsupported PTY = %v", err)
		}
		t.Skip("PTY requires Linux or Android")
	}
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{master: master, Slave: slave}
	defer r.Close()
	cols, rows, err := ptySize(master)
	if err != nil || cols != 80 || rows != 24 {
		t.Fatalf("default = %dx%d, %v", cols, rows, err)
	}
	r.onMessage(webplayer.BridgeMessage{Type: "resize", Cols: 132, Rows: 43})
	cols, rows, err = ptySize(master)
	if err != nil || cols != 132 || rows != 43 {
		t.Fatalf("resize = %dx%d, %v", cols, rows, err)
	}
	for _, size := range [][2]int{{0, 0}, {-1, 30}, {65536, 30}, {80, 65536}} {
		r.onMessage(webplayer.BridgeMessage{Type: "resize", Cols: size[0], Rows: size[1]})
	}
	cols, rows, _ = ptySize(master)
	if cols != 132 || rows != 43 {
		t.Fatalf("invalid resize changed size to %dx%d", cols, rows)
	}
}
