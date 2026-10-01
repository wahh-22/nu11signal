package radio

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The null emblem: a block-drawn Ø, its ring in the label color and its
// slash in the accent color, with the name beside it. The boot splash
// draws it (see splash and boot.go), and nu11signal --version prints the
// compact one (see EmblemRows).
//
//	  ▄████▄▄▀
//	▄█▀   ▄▀█▄   N U 1 1
//	██  ▄▀  ██   S I G N A L
//	▀█▄▀   ▄█▀   ◢◤◢◤◢◤◢◤◢◤
//	▄▀▀████▀
//
//	        B O O T I N G   N U 1 1 S I G N A L . . .

// An emblem is the Ø drawn cell by cell: rows of block glyphs, all as
// wide, and for each a mask as wide naming what each cell draws, r the
// ring, s the slash and a space nothing. Its text sits textGap cells
// right of it, from row textRow down.
type emblem struct {
	rows, mask []string
	textRow    int
}

// emblemLarge and emblemCompact are the two sizes of the emblem; the
// text sits on the rows the doc of the feature names, centered on the
// emblem's height.
var (
	emblemLarge = emblem{
		rows: []string{
			"  ▄████▄▄▀",
			"▄█▀   ▄▀█▄",
			"██  ▄▀  ██",
			"▀█▄▀   ▄█▀",
			"▄▀▀████▀  ",
		},
		mask: []string{
			"  rrrrrrss",
			"rrr   srrr",
			"rr  ss  rr",
			"rrrs   rrr",
			"ssrrrrrr  ",
		},
		textRow: 1,
	}
	emblemCompact = emblem{
		rows: []string{
			" ▄▀▀▄▀",
			"█ ▄▀ █",
			"▄▀▄▄▀ ",
		},
		mask: []string{
			" rrrrs",
			"r ss r",
			"srrrr ",
		},
		textRow: 0,
	}
)

// The name beside the emblem, its spaced lines over the accent slants,
// textGap cells right of it.
const (
	textGap    = 3
	emblemName = "NU11 SIGNAL"
	emblemMark = "◢◤◢◤◢◤◢◤◢◤"
)

// EmblemRows are the plain rows of the compact emblem, as wide as each
// other, for the command line to print (nu11signal --version).
func EmblemRows() []string { return append([]string(nil), emblemCompact.rows...) }

// textLines are the lines beside the emblem, plain: the name spaced out
// on two lines, then the slants.
func (e emblem) textLines() []string {
	words := strings.Fields(emblemName)
	return []string{spaced(words[0]), spaced(words[1]), emblemMark}
}

// width is the emblem's own width in cells.
func (e emblem) width() int { return ansi.StringWidth(e.rows[0]) }

// blockWidth is the width of the emblem with its text: the widest of its
// rows, text included.
func (e emblem) blockWidth() int {
	w := 0
	for _, l := range e.textLines() {
		w = max(w, ansi.StringWidth(l))
	}
	return e.width() + textGap + w
}

// block renders the emblem and its text, one line per emblem row: the
// ring in the label color, the slash in the accent color, the name in
// bold label and the slants in the accent color. The lines are not
// padded on the right.
func (e emblem) block() []string {
	text := e.textLines()
	styles := []func(...string) string{stLabelBold.Render, stLabelBold.Render, stAccent.Render}
	lines := make([]string, len(e.rows))
	for i, row := range e.rows {
		line := paintRow(row, e.mask[i])
		if t := i - e.textRow; t >= 0 && t < len(text) {
			line += strings.Repeat(" ", textGap) + styles[t](text[t])
		} else {
			line = strings.TrimRight(line, " ")
		}
		lines[i] = line
	}
	return lines
}

// paintRow styles row through mask, one style per run of cells: the ring
// (r) in the label color, the slash (s) in the accent color, the spaces
// left plain.
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
			run = stLabel.Render(run)
		case 's':
			run = stAccent.Render(run)
		}
		b.WriteString(run)
		x = end
	}
	return b.String()
}

// splash renders the body of the boot splash, w x h cells: the large
// emblem with its text, centered on the width as a block, and one blank
// row under it the BOOTING line (bootText), centered
// on the width on its own; together they are centered on the height. The
// compact emblem takes the large one's place where that does not fit, and
// only the line is left where neither does. The line is spaced out where
// it fits, plain otherwise.
func splash(w, h int) []string {
	line := spaced(bootText)
	if ansi.StringWidth(line) > w {
		line = bootText
	}
	var block []string
	for _, e := range []emblem{emblemLarge, emblemCompact} {
		if e.blockWidth() <= w && len(e.rows)+2 <= h {
			pad := strings.Repeat(" ", (w-e.blockWidth())/2)
			for _, l := range e.block() {
				block = append(block, pad+l)
			}
			block = append(block, "")
			break
		}
	}
	block = append(block, strings.Repeat(" ", max((w-ansi.StringWidth(line))/2, 0))+stMuted.Render(line))
	lines := make([]string, max((h-len(block))/2, 0), max(h, 0))
	lines = append(lines, block...)
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}

// splashTop is the first row of the boot splash's body, under the
// title line and the nav bar in the full layout and under the wordmark
// and the nav bar in the compact one; the body ends over the status line
// and the footer.
const splashTop = 2

// splashRegion is the body of the boot splash, compared for new
// content like a list (see introRegions): its text scrambles in.
func (m Model) splashRegion() []rowSpan {
	var rows []rowSpan
	for y := splashTop; y < m.height-2; y++ {
		rows = append(rows, rowSpan{y, 0, m.width})
	}
	return rows
}
