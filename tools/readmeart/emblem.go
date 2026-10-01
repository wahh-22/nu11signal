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

// emblemDots draws the large null emblem at (x0, y0) as dots: each
// Braille cell its 2 x 4 grid of pitch d (a cell 2d wide and 4d tall,
// a terminal cell's shape), one circle per raised dot, the ring in red
// and the slash in yellow, as the app paints it.
func emblemDots(x0, y0, d float64) string {
	rows, mask := radio.EmblemArt()
	var b strings.Builder
	for y, row := range rows {
		kinds := []rune(mask[y])
		for x, c := range []rune(row) {
			fill := red
			if kinds[x] == 's' {
				fill = yellow
			}
			brailleCell(&b, c, x0+float64(2*x)*d, y0+float64(4*y)*d, d, d, 0.4*d, fill)
		}
	}
	return b.String()
}

// emblemBars are the five thin Braille bars under the name, as the app
// draws them beside the emblem.
var emblemBars = strings.Repeat("⣠⡾⠋", 5)

// bars draws emblemBars from (x, y) as yellow dots of pitch d, one cell
// 2d wide.
func bars(x, y, d float64) string {
	var b strings.Builder
	for i, c := range []rune(emblemBars) {
		brailleCell(&b, c, x+float64(2*i)*d, y, d, d, 0.4*d, yellow)
	}
	return b.String()
}

// emblemSize is the large emblem's size in cells.
func emblemSize() (w, h int) {
	rows, _ := radio.EmblemArt()
	return len([]rune(rows[0])), len(rows)
}

// corners draws HUD brackets of arm length a at the corners of the box
// inset i from a w x h frame.
func corners(w, h, i, a float64, color string) string {
	var b strings.Builder
	for _, c := range [][4]float64{{i, i, 1, 1}, {w - i, i, -1, 1}, {i, h - i, 1, -1}, {w - i, h - i, -1, -1}} {
		x, y, dx, dy := c[0], c[1], c[2], c[3]
		fmt.Fprintf(&b, `<path d="M%s %sV%sH%s" fill="none" stroke="%s" stroke-width="3"/>`+"\n",
			num(x), num(y+dy*a), num(y), num(x+dx*a), color)
	}
	return b.String()
}

// banner is the README header: the emblem with the name spaced out
// beside it over the bars, on the emblem's rows 2..4 and textGap cells
// right of it as on the boot splash, in a framed night-black panel.
func banner() []byte {
	const (
		w, h  = 1200.0, 360.0
		d     = 8.0   // dot pitch
		cellW = 2 * d // one cell, emblem or text
		rowH  = 4 * d // one row
		font  = cellW / 0.6
		gap   = 3 * cellW // emblem to text
	)
	ew, eh := emblemSize()
	text := []string{"N U 1 1", "S I G N A L"}
	textW := float64(len([]rune(emblemBars))) * cellW
	x0 := (w - (float64(ew)*cellW + gap + textW)) / 2
	y0 := (h - float64(eh)*rowH) / 2
	tx := x0 + float64(ew)*cellW + gap

	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="1.5" y="1.5" width="%s" height="%s" rx="16" fill="%s" stroke="%s" stroke-opacity="0.45" stroke-width="3"/>`+"\n",
		num(w-3), num(h-3), night, red)
	b.WriteString(corners(w, h, 20, 28, cyan))
	fmt.Fprintf(&b, `<text x="44" y="52" font-family="%s" font-size="15" letter-spacing="3" fill="%s">SIG ▂▄▆█  //  AUTH OK</text>`+"\n", monoFonts, dimRed)
	fmt.Fprintf(&b, `<text x="%s" y="52" text-anchor="end" font-family="%s" font-size="15" letter-spacing="3" fill="%s">NODE 7F // NC-GRID</text>`+"\n", num(w-44), monoFonts, dimRed)
	b.WriteString(emblemDots(x0, y0, d))
	// The name on the emblem's rows 2 and 3, the bars on row 4.
	for i, line := range text {
		fmt.Fprintf(&b, `<text x="%s" y="%s" textLength="%s" lengthAdjust="spacingAndGlyphs" font-family="%s" font-size="%s" font-weight="bold" fill="%s">%s</text>`+"\n",
			num(tx), num(y0+float64(i+2)*rowH+rowH*0.8), num(float64(len([]rune(line)))*cellW), monoFonts, num(font), red, esc(line))
	}
	b.WriteString(bars(tx, y0+4*rowH, d))
	fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="middle" font-family="%s" font-size="16" letter-spacing="6" fill="%s">APPLE MUSIC  //  TERMINAL RADIO</text>`+"\n",
		num(w/2), num(h-40), monoFonts, cyan)
	return svgDoc(w, h, "nu11signal: the null emblem beside N U 1 1 / S I G N A L", b.String())
}

// emblemIcon is the emblem alone on a square night-black tile.
func emblemIcon() []byte {
	const size, d = 240.0, 6.0
	ew, eh := emblemSize()
	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="1.5" y="1.5" width="%s" height="%s" rx="28" fill="%s" stroke="%s" stroke-opacity="0.45" stroke-width="3"/>`+"\n",
		num(size-3), num(size-3), night, red)
	b.WriteString(emblemDots((size-float64(ew)*2*d)/2, (size-float64(eh)*4*d)/2, d))
	return svgDoc(size, size, "nu11signal emblem", b.String())
}
