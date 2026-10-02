package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// A screen is one terminal screenshot: a golden (or one == section == of
// a golden holding several views) drawn in a window.
type screen struct {
	name, golden, section, title string
	bg, accent                   string
}

// The 80x24 goldens' size.
const cols, rows = 80, 24

// screens are the screenshots the README and the docs show.
var screens = []screen{
	{"redshift", "view_redshift_ansi_80x24.golden", "", "REDSHIFT", night, red},
	{"blue", "view_blue_80x24.golden", "", "BLUE", "#05070F", "#347AFF"},
	{"matrix", "view_matrix_80x24.golden", "", "MATRIX", "#000000", "#00C832"},
	{"rose", "view_rose_80x24.golden", "", "ROSE", "#060407", "#F095C8"},
	{"neon-rose", "view_neon_rose_80x24.golden", "", "NEON ROSE", "#060407", "#F43888"},
	{"boot", "boot_blue_80x24.golden", "", "BOOT", "#05070F", "#347AFF"},
	{"search", "redshift_ansi_views_80x24.golden", "search", "SEARCH", night, red},
	{"keys", "redshift_ansi_views_80x24.golden", "help", "KEYS", night, red},
	{"settings", "redshift_ansi_views_80x24.golden", "settings", "SETTINGS", night, red},
	{"no-signal", "redshift_ansi_views_80x24.golden", "no signal", "SIGNAL EFFECTS", night, red},
	{"expanded", "redshift_ansi_views_80x24.golden", "expanded", "PLAYER", night, red},
}

// lines reads the screen's 24 rows from its golden under root.
func (s screen) lines(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "internal", "radio", "testdata", s.golden))
	if err != nil {
		return nil, err
	}
	all := strings.Split(string(data), "\n")
	if s.section != "" {
		start := -1
		for i, l := range all {
			if l == "== "+s.section+" ==" {
				start = i + 1
				break
			}
		}
		if start < 0 {
			return nil, fmt.Errorf("%s: no section %q", s.golden, s.section)
		}
		all = all[start:]
		for i, l := range all {
			if strings.HasPrefix(l, "== ") && strings.HasSuffix(l, " ==") {
				all = all[:i]
				break
			}
		}
	}
	for len(all) < rows {
		all = append(all, "")
	}
	return all[:rows], nil
}

// A span is a run of cells drawn alike.
type span struct {
	col, width int
	text       string
	style
}

// A style is the SGR state a span is drawn in; empty colors are the
// terminal's defaults.
type style struct {
	fg, bg        string
	bold, reverse bool
}

// basic are the 16 ANSI colors, for the SGR 30-37 and 90-97 forms.
var basic = [16]string{
	"#000000", "#CD3131", "#0DBC79", "#E5E510", "#2472C8", "#BC3FBC", "#11A8CD", "#E5E5E5",
	"#666666", "#F14C4C", "#23D18B", "#F5F543", "#3B8EEA", "#D670D6", "#29B8DB", "#FFFFFF",
}

// apply updates st with the parameters of one SGR sequence.
func (st *style) apply(params string) {
	if params == "" {
		*st = style{}
		return
	}
	p := strings.Split(params, ";")
	for i := 0; i < len(p); i++ {
		n, _ := strconv.Atoi(p[i])
		switch {
		case n == 0:
			*st = style{}
		case n == 1:
			st.bold = true
		case n == 7:
			st.reverse = true
		case n == 22:
			st.bold = false
		case n == 27:
			st.reverse = false
		case n >= 30 && n <= 37:
			st.fg = basic[n-30]
		case n >= 90 && n <= 97:
			st.fg = basic[n-90+8]
		case n == 39:
			st.fg = ""
		case n >= 40 && n <= 47:
			st.bg = basic[n-40]
		case n == 49:
			st.bg = ""
		case (n == 38 || n == 48) && i+4 < len(p) && p[i+1] == "2":
			var rgb [3]int
			for k := range rgb {
				rgb[k], _ = strconv.Atoi(p[i+2+k])
			}
			c := fmt.Sprintf("#%02X%02X%02X", rgb[0], rgb[1], rgb[2])
			if n == 38 {
				st.fg = c
			} else {
				st.bg = c
			}
			i += 4
		}
	}
}

// parseLine splits one rendered row into spans of one style, by cell.
// Only SGR sequences are expected; any other escape is skipped.
func parseLine(line string) []span {
	var (
		spans []span
		st    style
		cur   *span
		col   int
	)
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == 0x1b && i+1 < len(rs) && rs[i+1] == '[' {
			j := i + 2
			for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
				j++
			}
			if j < len(rs) && rs[j] == 'm' {
				st.apply(string(rs[i+2 : j]))
			}
			i = j
			cur = nil
			continue
		}
		w := ansi.StringWidth(string(r))
		if w == 0 {
			if cur != nil {
				cur.text += string(r)
			}
			continue
		}
		if cur == nil || cur.style != st {
			spans = append(spans, span{col: col, style: st})
			cur = &spans[len(spans)-1]
		}
		cur.text += string(r)
		cur.width += w
		col += w
	}
	return spans
}

// Window and cell metrics, in pixels.
const (
	cellW, cellH = 10.0, 20.0
	fontSize     = 16.0
	pad          = 20.0
	titleBar     = 36.0
	defaultFG    = "#C8C8C8"
)

// render draws the screen as an SVG terminal window.
func (s screen) render(root string) ([]byte, error) {
	lines, err := s.lines(root)
	if err != nil {
		return nil, err
	}
	w, h := cols*cellW+2*pad, titleBar+rows*cellH+2*pad
	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="1" y="1" width="%s" height="%s" rx="12" fill="%s" stroke="%s" stroke-opacity="0.5" stroke-width="2"/>`+"\n",
		num(w-2), num(h-2), s.bg, s.accent)
	for i, c := range []string{"#FF5F57", "#FEBC2E", "#28C840"} {
		fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="6" fill="%s"/>`+"\n", num(pad+float64(i)*20), num(titleBar/2+2), c)
	}
	fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="middle" font-family="%s" font-size="13" fill="#8A8A8A">nu11signal — %s — 80×24</text>`+"\n",
		num(w/2), num(titleBar/2+6), monoFonts, esc(s.title))
	fmt.Fprintf(&b, `<g font-family="%s" font-size="%s" xml:space="preserve">`+"\n", monoFonts, num(fontSize))
	top := titleBar + pad
	for y, line := range lines {
		for _, sp := range parseLine(line) {
			fg, bg := sp.fg, sp.bg
			if fg == "" {
				fg = defaultFG
			}
			if sp.reverse {
				fg, bg = orDefault(bg, s.bg), fg
			}
			x, ry := pad+float64(sp.col)*cellW, top+float64(y)*cellH
			if bg != "" {
				fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`+"\n",
					num(x), num(ry), num(float64(sp.width)*cellW), num(cellH), bg)
			}
			for _, sg := range segments(sp.text) {
				sx := x + float64(sg.col)*cellW
				if sg.braille {
					for i, r := range []rune(sg.text) {
						brailleCell(&b, r, sx+float64(i)*cellW, ry, cellW/2, cellH/4, 1.9, fg)
					}
					continue
				}
				text := strings.TrimLeft(sg.text, " ")
				lead := len(sg.text) - len(text)
				text = strings.TrimRight(text, " ")
				if text == "" {
					continue
				}
				tw := ansi.StringWidth(text)
				weight := ""
				if sp.bold {
					weight = ` font-weight="bold"`
				}
				fmt.Fprintf(&b, `<text x="%s" y="%s" textLength="%s" lengthAdjust="spacingAndGlyphs" fill="%s"%s>%s</text>`+"\n",
					num(sx+float64(lead)*cellW), num(ry+15), num(float64(tw)*cellW), fg, weight, esc(text))
			}
		}
	}
	b.WriteString("</g>\n")
	return svgDoc(w, h, "nu11signal "+s.title+" screen, 80x24", b.String()), nil
}

// A segment is a run of a span's text, col cells into it: Braille
// cells, which are drawn as dots so a screen reads right without a font
// that has them, or anything else, drawn as text.
type segment struct {
	col     int
	text    string
	braille bool
}

// segments splits text into its runs of Braille and other cells.
func segments(text string) []segment {
	var out []segment
	col := 0
	for _, r := range text {
		br := r >= 0x2800 && r <= 0x28FF
		if n := len(out); n == 0 || out[n-1].braille != br {
			out = append(out, segment{col: col, braille: br})
		}
		out[len(out)-1].text += string(r)
		col += ansi.StringWidth(string(r))
	}
	return out
}

// orDefault is c, or def when c is the terminal default.
func orDefault(c, def string) string {
	if c == "" {
		return def
	}
	return c
}
