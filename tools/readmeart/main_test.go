package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
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

// The large emblem raises 292 Braille dots and the bars beside it 60
// (five ⣠⡾⠋, 12 dots each): one circle each.
const emblemDotCount, barDotCount = 292, 60

func TestEmblemArtIsOneDotPerRaisedBrailleDot(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
		want int
	}{
		{"banner", banner(), emblemDotCount + barDotCount},
		{"emblem", emblemIcon(), emblemDotCount},
	} {
		wellFormed(t, tt.name, tt.data)
		if n := strings.Count(string(tt.data), `<circle class="dot"`); n != tt.want {
			t.Errorf("%s: %d emblem dots, want %d", tt.name, n, tt.want)
		}
		if !strings.Contains(string(tt.data), yellow) || !strings.Contains(string(tt.data), red) {
			t.Errorf("%s: want the ring in %s and the slash in %s", tt.name, red, yellow)
		}
		if strings.ContainsFunc(string(tt.data), braille) {
			t.Errorf("%s: Braille drawn as text, not as dots", tt.name)
		}
	}
	if strings.Contains(string(banner()), "<polygon") {
		t.Error("banner: still draws the slants")
	}
}

// brailleDots places the raised dots of a Braille cell on its 2 x 4
// grid, dot 1 top left down to dot 7, dot 4 top right down to dot 8.
func TestBrailleDotsFollowTheBrailleNumbering(t *testing.T) {
	for _, tt := range []struct {
		r    rune
		want [][2]int
	}{
		{'⠁', [][2]int{{0, 0}}},
		{'⠈', [][2]int{{1, 0}}},
		{'⡀', [][2]int{{0, 3}}},
		{'⢀', [][2]int{{1, 3}}},
		{'⠋', [][2]int{{0, 0}, {0, 1}, {1, 0}}},
	} {
		if got := dotsOf(tt.r); !slices.Equal(got, tt.want) {
			t.Errorf("dotsOf(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
	if len(dotsOf('⣿')) != 8 || len(dotsOf('⠀')) != 0 || dotsOf('A') != nil {
		t.Error("dotsOf: want 8 dots for ⣿, none for the blank pattern or a letter")
	}
}

// braille reports whether r is a Braille pattern.
func braille(r rune) bool { return r >= 0x2800 && r <= 0x28FF }

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
		// Braille cells (the emblem) are drawn as dots, so the screens
		// read right without a font that has them.
		if strings.ContainsFunc(string(data), braille) {
			t.Errorf("%s: Braille drawn as text, not as dots", s.name)
		}
		if s.name == "boot" && !strings.Contains(string(data), `<circle class="dot"`) {
			t.Errorf("%s: the emblem draws no dots", s.name)
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
