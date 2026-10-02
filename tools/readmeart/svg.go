package main

import (
	"math"
	"strconv"
	"strings"
)

// The REDSHIFT colors the brand art is drawn in.
const (
	night  = "#0A0A0A"
	red    = "#FF5F57"
	cyan   = "#5EF6FF"
	yellow = "#FCEE0A"
	dimRed = "#9A3B37"
)

// monoFonts is the font stack of every text: Kode Mono, the font the UI
// is designed with, then common monospaced fallbacks.
const monoFonts = `'Kode Mono','JetBrains Mono','SF Mono',Menlo,Consolas,monospace`

// num formats v with at most two decimals and no trailing zeros, so the
// output never depends on floating-point noise.
func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// esc escapes text for an XML text node or attribute.
var esc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace

// svgDoc wraps body in an svg element w x h.
func svgDoc(w, h float64, label, body string) []byte {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="` + num(w) + `" height="` + num(h) +
		`" viewBox="0 0 ` + num(w) + ` ` + num(h) + `" role="img" aria-label="` + esc(label) + `">` + "\n")
	b.WriteString(body)
	b.WriteString("</svg>\n")
	return []byte(b.String())
}
