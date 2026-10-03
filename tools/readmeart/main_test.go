package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
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

// The logo raises 650 Braille dots: one circle each.
const emblemDotCount = 650

func TestEmblemArtIsOneDotPerRaisedBrailleDot(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
		want int
	}{
		{"emblem", emblemIcon(), emblemDotCount},
	} {
		wellFormed(t, tt.name, tt.data)
		if n := strings.Count(string(tt.data), `<circle class="dot"`); n != tt.want {
			t.Errorf("%s: %d emblem dots, want %d", tt.name, n, tt.want)
		}
		if !strings.Contains(string(tt.data), `fill="`+cyan+`"`) || !strings.Contains(string(tt.data), `fill="`+red+`"`) || strings.Contains(string(tt.data), yellow) {
			t.Errorf("%s: want the logo in %s and %s alone", tt.name, red, cyan)
		}
		if strings.ContainsFunc(string(tt.data), braille) {
			t.Errorf("%s: Braille drawn as text, not as dots", tt.name)
		}
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

// The committed SVGs are what the generator renders now: rerun
// go run ./tools/readmeart after changing the emblem or this tool.
func TestCommittedAssetsAreUpToDate(t *testing.T) {
	for _, a := range render() {
		got, err := os.ReadFile(filepath.Join(repoRoot, a.path))
		if err != nil {
			t.Fatalf("%s: %v (run go run ./tools/readmeart)", a.path, err)
		}
		if !bytes.Equal(got, a.data) {
			t.Errorf("%s is stale: run go run ./tools/readmeart", a.path)
		}
	}
}

// Every local image the README shows exists, the banner logo above all:
// GitHub renders a missing one as a broken image without any error.
func TestREADMEImagesExist(t *testing.T) {
	const banner = "docs/assets/brand/logo/nu11signal-blueshift.svg"
	data, err := os.ReadFile(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	var local []string
	for _, m := range regexp.MustCompile(`<img[^>]*\ssrc="([^"]+)"`).FindAllStringSubmatch(string(data), -1) {
		if !strings.Contains(m[1], "://") {
			local = append(local, m[1])
		}
	}
	if !slices.Contains(local, banner) {
		t.Errorf("README.md does not show the banner %s; local images: %q", banner, local)
	}
	for _, path := range local {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(path))); err != nil {
			t.Errorf("README.md shows %s: %v", path, err)
		}
	}
}
