package radio

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The emblem is the logo drawn in Braille: the masked head with its
// headphones and LED eyes, the wordmark NU11SIGNAL beside it and five
// slanted bars under the wordmark, in the theme's logo colors (logo, the
// primary, for the head's frame, the headphones, the top LED row, SIGNAL
// and the bars; logoAlt, the secondary, for NU11 and the other LED rows,
// see theme). The boot and shutdown splashes draw it (see splash,
// boot.go and shutdown.go), the spectrum area shows it while no music
// plays (see idle.go), and nu11signal --version prints the compact one
// (see EmblemRows).
//
//	  ⢀⡴⠟⣉⡥⠤⠤⢬⣉⠻⢦⡀
//	 ⣰⠏⡠⠊⠁    ⠈⠑⢄⠹⣆
//	⣰⣏⢰⠁        ⠈⡆⣹⣆
//	⣿⣿⣿⠠⢀⢀    ⡀⡀⠄⣿⣿⣿ ⣿⣆⢿⢸⡇⢸⡇⠚⣿ ⠐⢻⡇ ⢾⣉⡉⠈⢹⡏⠁⣾⢉⣉⢸⣷⡸⡇⣾⠉⣷⢸⡇
//	⣿⣿⣿⠨⢐⢐⠨  ⠅⡂⡂⠅⣿⣿⣿ ⣿⠘⣿⠸⣧⣼⠇⣤⣿⣤⢠⣼⣧⡄⣀⣀⡿⢀⣸⣇⡀⢿⣀⣿⢸⡇⢻⡇⣿⠉⣿⢸⣇⣀⡀
//	⠛⠛⠻⡈⠐⠐⠨  ⠅⠂⠂⢁⠟⠛⠛             ⣠⡶⢂⣴⠖⣠⡶⢂⣴⠖⣠⡶⠂
//	   ⠙⢆      ⡰⠋               ⠚⠋⠐⠛⠁⠚⠋⠐⠛⠁⠚⠋
//	    ⠈⠣⠤⠤⠤⠤⠜⠁
//
//	     B O O T I N G   N U 1 1 S I G N A L . . .
//
// The compact one writes the wordmark as text and the bars as five
// Braille slants:
//
//	 ⢀⡴⢋⡩⠥⠬⢍⡙⢦⡀
//	⢠⠏⡴⠉    ⠉⢦⠹⡄
//	⣿⣧⡇⠄⡀⡀⢀⢀⠠⢸⣼⣿  NU11SIGNAL
//	⣿⣿⡇⠅⡂⡂⢐⢐⠨⢸⣿⣿    ⡾⡾⡾⡾⡾
//	  ⠱⡄    ⢠⠎
//	   ⠘⠦⠤⠤⠴⠃

// An emblem is the logo drawn cell by cell: rows of one-cell Braille
// patterns (U+2800..U+28FF, each a 2 x 4 dot grid) or, in the compact
// wordmark, letters, all as wide, and for each a mask as wide naming what
// each cell draws, r the logo's primary color, s its secondary one and a
// space nothing.
type emblem struct {
	rows, mask []string
}

// emblemLarge (52 x 8 cells: the head 16 x 8, a 32 x 32 dot grid, the
// wordmark in a bold 6 x 8 dot font on rows 3-4 and the bars on rows
// 5-6) and emblemCompact (24 x 6: the head 12 x 6, NU11SIGNAL as text on
// row 2 and the bars on row 3) are the two sizes of the logo.
// emblemLargeHead and emblemCompactHead are their heads alone, the
// first headLarge and headCompact cells of their rows, for the areas too
// narrow for the wordmark (see idleArtFor).
var (
	emblemLarge = emblem{
		rows: []string{
			"  ⢀⡴⠟⣉⡥⠤⠤⢬⣉⠻⢦⡀                                      ",
			" ⣰⠏⡠⠊⠁    ⠈⠑⢄⠹⣆                                     ",
			"⣰⣏⢰⠁        ⠈⡆⣹⣆                                    ",
			"⣿⣿⣿⠠⢀⢀    ⡀⡀⠄⣿⣿⣿ ⣿⣆⢿⢸⡇⢸⡇⠚⣿ ⠐⢻⡇ ⢾⣉⡉⠈⢹⡏⠁⣾⢉⣉⢸⣷⡸⡇⣾⠉⣷⢸⡇  ",
			"⣿⣿⣿⠨⢐⢐⠨  ⠅⡂⡂⠅⣿⣿⣿ ⣿⠘⣿⠸⣧⣼⠇⣤⣿⣤⢠⣼⣧⡄⣀⣀⡿⢀⣸⣇⡀⢿⣀⣿⢸⡇⢻⡇⣿⠉⣿⢸⣇⣀⡀",
			"⠛⠛⠻⡈⠐⠐⠨  ⠅⠂⠂⢁⠟⠛⠛             ⣠⡶⢂⣴⠖⣠⡶⢂⣴⠖⣠⡶⠂          ",
			"   ⠙⢆      ⡰⠋               ⠚⠋⠐⠛⠁⠚⠋⠐⠛⠁⠚⠋            ",
			"    ⠈⠣⠤⠤⠤⠤⠜⠁                                        ",
		},
		mask: []string{
			"  rrrrrrrrrrrr                                      ",
			" rrrrr    rrrrr                                     ",
			"rrrr        rrrr                                    ",
			"rrrrrr    rrrrrr sssssssss sss rrrrrrrrrrrrrrrrrrr  ",
			"rrrssss  ssssrrr ssssssssssssssrrrrrrrrrrrrrrrrrrrrr",
			"rrrssss  ssssrrr             rrrrrrrrrrrrr          ",
			"   rr      rr               rrrrrrrrrrrr            ",
			"    rrrrrrrr                                        ",
		},
	}
	emblemCompact = emblem{
		rows: []string{
			" ⢀⡴⢋⡩⠥⠬⢍⡙⢦⡀             ",
			"⢠⠏⡴⠉    ⠉⢦⠹⡄            ",
			"⣿⣧⡇⠄⡀⡀⢀⢀⠠⢸⣼⣿  NU11SIGNAL",
			"⣿⣿⡇⠅⡂⡂⢐⢐⠨⢸⣿⣿    ⡾⡾⡾⡾⡾   ",
			"  ⠱⡄    ⢠⠎              ",
			"   ⠘⠦⠤⠤⠴⠃               ",
		},
		mask: []string{
			" rrrrrrrrrr             ",
			"rrrr    rrrr            ",
			"rrrrrrrrrrrr  ssssrrrrrr",
			"rrrssssssrrr    rrrrr   ",
			"  rr    rr              ",
			"   rrrrrr               ",
		},
	}
	emblemLargeHead   = emblemLarge.head(headLarge)
	emblemCompactHead = emblemCompact.head(headCompact)
)

// headLarge and headCompact are the widths of the head in the large and
// the compact emblem; a blank column parts it from the wordmark.
const (
	headLarge   = 16
	headCompact = 12
)

// EmblemRows are the plain rows of the compact emblem, NU11SIGNAL and
// the bars included, as wide as each other, for the command line to
// print (nu11signal --version).
func EmblemRows() []string { return append([]string(nil), emblemCompact.rows...) }

// EmblemArt is the large emblem cell by cell, for the README art
// (tools/readmeart): its rows of Braille cells and, as wide, their masks
// (r the logo's primary color, s its secondary one, a space nothing).
func EmblemArt() (rows, mask []string) {
	return append([]string(nil), emblemLarge.rows...), append([]string(nil), emblemLarge.mask...)
}

// head is the emblem cut to its first w cells: the head without the
// wordmark and the bars.
func (e emblem) head(w int) emblem {
	var h emblem
	for i, row := range e.rows {
		h.rows = append(h.rows, string([]rune(row)[:w]))
		h.mask = append(h.mask, string([]rune(e.mask[i])[:w]))
	}
	return h
}

// width is the emblem's width in cells.
func (e emblem) width() int { return ansi.StringWidth(e.rows[0]) }

// block renders the emblem, one line per row, painted through its mask
// (see paintRow). The lines are not padded on the right.
func (e emblem) block() []string {
	lines := make([]string, len(e.rows))
	for i, row := range e.rows {
		lines[i] = strings.TrimRight(paintRow(row, e.mask[i]), " ")
	}
	return lines
}

// paintRow styles row through mask, one style per run of cells: r in
// the logo's primary color, s in its secondary one, the spaces left
// plain.
func paintRow(row, mask string) string {
	cells, kinds := []rune(row), []rune(mask)
	var b strings.Builder
	for x := 0; x < len(cells); {
		end := x + 1
		for end < len(cells) && kinds[end] == kinds[x] {
			end++
		}
		run := string(cells[x:end])
		switch kinds[x] {
		case 'r':
			run = stLogo.Render(run)
		case 's':
			run = stLogoAlt.Render(run)
		}
		b.WriteString(run)
		x = end
	}
	return b.String()
}

// splash renders the body of the boot or shutdown splash, w x h cells:
// the large emblem, centered on the width, and one blank row under it
// the line text (bootText or shutdownText, see
// splashText), centered on the width on its own; together they are
// centered on the height. The compact emblem takes the large one's place
// where that does not fit, and only the line is left where neither does.
// The line is spaced out where it fits, plain otherwise, and drawn bright
// (stHiBold, the transport's cyan in bold) so it reads clearly under the
// emblem.
func splash(w, h int, text string) []string {
	line := spaced(text)
	if ansi.StringWidth(line) > w {
		line = text
	}
	var block []string
	for _, e := range []emblem{emblemLarge, emblemCompact} {
		if e.width() <= w && len(e.rows)+2 <= h {
			pad := strings.Repeat(" ", (w-e.width())/2)
			for _, l := range e.block() {
				block = append(block, pad+l)
			}
			block = append(block, "")
			break
		}
	}
	block = append(block, strings.Repeat(" ", max((w-ansi.StringWidth(line))/2, 0))+stHiBold.Render(line))
	lines := make([]string, max((h-len(block))/2, 0), max(h, 0))
	lines = append(lines, block...)
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}

// splashTop is the first row of the splash's body, under the
// title line and the nav bar in the full layout and under the wordmark
// and the nav bar in the compact one; the body ends over the status line
// and the footer.
const splashTop = 2

// splashRegion is the body of the splash, compared for new
// content like a list (see introRegions): its text scrambles in.
func (m Model) splashRegion() []rowSpan {
	var rows []rowSpan
	for y := splashTop; y < m.height-2; y++ {
		rows = append(rows, rowSpan{y, 0, m.width})
	}
	return rows
}
