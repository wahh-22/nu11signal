package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is the repository root, two levels above this package.
const repoRoot = "../.."

// wellFormed fails t unless data parses as XML with an svg root.
func wellFormed(t *testing.T, name string, data []byte) {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(data))
	root := ""
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("%s: not well-formed XML: %v", name, err)
		}
		if se, ok := tok.(xml.StartElement); ok && root == "" {
			root = se.Name.Local
		}
	}
	if root != "svg" {
		t.Fatalf("%s: root element %q, want svg", name, root)
	}
}

// The large emblem has 36 drawn cells (8+7+6+7+8), one rect each.
const emblemCells = 36

func TestEmblemArtIsOneRectPerDrawnCell(t *testing.T) {
	for name, data := range map[string][]byte{"banner": banner(), "emblem": emblemIcon()} {
		wellFormed(t, name, data)
		if n := strings.Count(string(data), `<rect class="px"`); n != emblemCells {
			t.Errorf("%s: %d emblem rects, want %d", name, n, emblemCells)
		}
		if !strings.Contains(string(data), yellow) || !strings.Contains(string(data), red) {
			t.Errorf("%s: want the ring in %s and the slash in %s", name, red, yellow)
		}
	}
	if n := strings.Count(string(banner()), "<polygon"); n != 10 {
		t.Errorf("banner: %d slants, want 10", n)
	}
}

func TestParseLineSplitsSpansBySGRStyle(t *testing.T) {
	line := "ab\x1b[1;38;2;255;95;87mN\x1b[m \x1b[38;2;1;2;3;48;2;4;5;6mx<\x1b[m"
	got := parseLine(line)
	want := []span{
		{col: 0, width: 2, text: "ab"},
		{col: 2, width: 1, text: "N", style: style{fg: "#FF5F57", bold: true}},
		{col: 3, width: 1, text: " "},
		{col: 4, width: 2, text: "x<", style: style{fg: "#010203", bg: "#040506"}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d spans %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestScreensRenderWellFormedFromTheGoldens(t *testing.T) {
	for _, s := range screens {
		data, err := s.render(repoRoot)
		if err != nil {
			t.Fatal(err)
		}
		wellFormed(t, s.name, data)
		if !strings.Contains(string(data), "<text") {
			t.Errorf("%s: no text drawn", s.name)
		}
	}
}

// The committed SVGs are what the generator renders now: rerun
// go run ./tools/readmeart after changing the emblem, a golden, or this
// tool.
func TestCommittedAssetsAreUpToDate(t *testing.T) {
	assets, err := render(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range assets {
		got, err := os.ReadFile(filepath.Join(repoRoot, a.path))
		if err != nil {
			t.Fatalf("%s: %v (run go run ./tools/readmeart)", a.path, err)
		}
		if !bytes.Equal(got, a.data) {
			t.Errorf("%s is stale: run go run ./tools/readmeart", a.path)
		}
	}
}
