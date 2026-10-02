package main

import (
	"fmt"
	"strings"

	"github.com/wahh-22/nu11signal/internal/radio"
)

// brailleBits maps the bits of a Braille pattern (U+2800 + bits), dot 1
// first, to their column and row on its 2 x 4 grid: dots 1-3 down the
// left, 4-6 down the right, then 7 and 8 under them.
var brailleBits = [8][2]int{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 1}, {1, 2}, {0, 3}, {1, 3}}

// dotsOf is the raised dots of the Braille pattern r as column, row
// pairs on its 2 x 4 grid, in dot order; nil when r is not Braille.
func dotsOf(r rune) [][2]int {
	if r < 0x2800 || r > 0x28FF {
		return nil
	}
	dots := [][2]int{}
	for bit, at := range brailleBits {
		if (r-0x2800)&(1<<bit) != 0 {
			dots = append(dots, at)
		}
	}
	return dots
}

// brailleCell draws the raised dots of r as circles of class dot, radius
// rad, on a grid of pitch dx x dy from the cell's top left (x, y).
func brailleCell(b *strings.Builder, r rune, x, y, dx, dy, rad float64, fill string) {
	for _, d := range dotsOf(r) {
		fmt.Fprintf(b, `<circle class="dot" cx="%s" cy="%s" r="%s" fill="%s"/>`+"\n",
			num(x+(float64(d[0])+0.5)*dx), num(y+(float64(d[1])+0.5)*dy), num(rad), fill)
	}
}

// emblemDots draws the emblem, the logo, at (x0, y0) as dots: each
// Braille cell its 2 x 4 grid of pitch d (a cell 2d wide and 4d tall,
// a terminal cell's shape), one circle per raised dot, in the logo's
// REDSHIFT colors as the app paints it: the primary (r, red) for the
// head's frame, the headphones, the top LED row, SIGNAL and the bars,
// the secondary (s, cyan) for NU11 and the other LED rows.
func emblemDots(x0, y0, d float64) string {
	rows, mask := radio.EmblemArt()
	var b strings.Builder
	for y, row := range rows {
		kinds := []rune(mask[y])
		for x, c := range []rune(row) {
			fill := red
			if kinds[x] == 's' {
				fill = cyan
			}
			brailleCell(&b, c, x0+float64(2*x)*d, y0+float64(4*y)*d, d, d, 0.4*d, fill)
		}
	}
	return b.String()
}

// emblemSize is the logo's size in cells.
func emblemSize() (w, h int) {
	rows, _ := radio.EmblemArt()
	return len([]rune(rows[0])), len(rows)
}

// emblemIcon is the emblem alone on a night-black tile as wide as the
// logo needs: the art at dot pitch d, pad around it on every side.
func emblemIcon() []byte {
	const d, pad = 5.0, 40.0
	ew, eh := emblemSize()
	w, h := float64(ew)*2*d+2*pad, float64(eh)*4*d+2*pad
	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="1.5" y="1.5" width="%s" height="%s" rx="28" fill="%s" stroke="%s" stroke-opacity="0.45" stroke-width="3"/>`+"\n",
		num(w-3), num(h-3), night, red)
	b.WriteString(emblemDots(pad, pad, d))
	return svgDoc(w, h, "nu11signal emblem", b.String())
}
