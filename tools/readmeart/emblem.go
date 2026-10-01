package main

import (
	"fmt"
	"strings"

	"github.com/wahh-22/nu11signal/internal/radio"
)

// emblemPixels draws the large null emblem at (x0, y0) as rects of class
// px, one per drawn cell: a full block is a square of two pixels, an
// upper or lower half block one pixel, each pixel p wide and p tall, the
// ring in red and the slash in yellow, as the app paints it.
func emblemPixels(x0, y0, p float64) string {
	rows, mask := radio.EmblemArt()
	var b strings.Builder
	for y, row := range rows {
		kinds := []rune(mask[y])
		for x, c := range []rune(row) {
			top, h := float64(2*y)*p, p
			switch c {
			case '█':
				h = 2 * p
			case '▀':
			case '▄':
				top += p
			default:
				continue
			}
			fill := red
			if kinds[x] == 's' {
				fill = yellow
			}
			fmt.Fprintf(&b, `<rect class="px" x="%s" y="%s" width="%s" height="%s" fill="%s"/>`+"\n",
				num(x0+float64(x)*p), num(y0+top), num(p), num(h), fill)
		}
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
// beside it over the accent slants, as on the boot splash, in a framed
// night-black panel.
func banner() []byte {
	const (
		w, h  = 1200.0, 360.0
		p     = 20.0 // emblem pixel
		gap   = 48.0 // emblem to text
		font  = 44.0
		cellW = font * 0.6 // monospace advance
		rowH  = 2 * p      // one emblem row
	)
	ew, eh := emblemSize()
	text := []string{"N U 1 1", "S I G N A L"}
	textW := float64(len([]rune(text[1]))) * cellW
	x0 := (w - (float64(ew)*p + gap + textW)) / 2
	y0 := (h - float64(eh)*rowH) / 2
	tx := x0 + float64(ew)*p + gap

	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="1.5" y="1.5" width="%s" height="%s" rx="16" fill="%s" stroke="%s" stroke-opacity="0.45" stroke-width="3"/>`+"\n",
		num(w-3), num(h-3), night, red)
	b.WriteString(corners(w, h, 20, 28, cyan))
	fmt.Fprintf(&b, `<text x="44" y="52" font-family="%s" font-size="15" letter-spacing="3" fill="%s">SIG ▂▄▆█  //  AUTH OK</text>`+"\n", monoFonts, dimRed)
	fmt.Fprintf(&b, `<text x="%s" y="52" text-anchor="end" font-family="%s" font-size="15" letter-spacing="3" fill="%s">NODE 7F // NC-GRID</text>`+"\n", num(w-44), monoFonts, dimRed)
	b.WriteString(emblemPixels(x0, y0, p))
	// The name on the emblem's rows 1 and 2, the slants on row 3.
	for i, line := range text {
		fmt.Fprintf(&b, `<text x="%s" y="%s" textLength="%s" lengthAdjust="spacingAndGlyphs" font-family="%s" font-size="%s" font-weight="bold" fill="%s">%s</text>`+"\n",
			num(tx), num(y0+float64(i+1)*rowH+rowH*0.82), num(float64(len([]rune(line)))*cellW), monoFonts, num(font), red, esc(line))
	}
	b.WriteString(slants(tx, y0+3*rowH+6, cellW, rowH-12, 10))
	fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="middle" font-family="%s" font-size="16" letter-spacing="6" fill="%s">APPLE MUSIC  //  TERMINAL RADIO</text>`+"\n",
		num(w/2), num(h-40), monoFonts, cyan)
	return svgDoc(w, h, "nu11signal: the null emblem beside N U 1 1 / S I G N A L", b.String())
}

// slants draws n accent cells from (x, y), each cw x ch, alternating ◢
// and ◤ as triangles so they look the same in any font.
func slants(x, y, cw, ch float64, n int) string {
	var b strings.Builder
	for i := range n {
		l, r, t, bt := x+float64(i)*cw, x+float64(i+1)*cw, y, y+ch
		pts := fmt.Sprintf("%s,%s %s,%s %s,%s", num(r), num(t), num(r), num(bt), num(l), num(bt)) // ◢
		if i%2 == 1 {
			pts = fmt.Sprintf("%s,%s %s,%s %s,%s", num(l), num(t), num(r), num(t), num(l), num(bt)) // ◤
		}
		fmt.Fprintf(&b, `<polygon points="%s" fill="%s"/>`+"\n", pts, yellow)
	}
	return b.String()
}

// emblemIcon is the emblem alone on a square night-black tile.
func emblemIcon() []byte {
	const size, p = 240.0, 16.0
	ew, eh := emblemSize()
	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="1.5" y="1.5" width="%s" height="%s" rx="28" fill="%s" stroke="%s" stroke-opacity="0.45" stroke-width="3"/>`+"\n",
		num(size-3), num(size-3), night, red)
	b.WriteString(emblemPixels((size-float64(ew)*p)/2, (size-float64(eh)*2*p)/2, p))
	return svgDoc(size, size, "nu11signal emblem", b.String())
}
