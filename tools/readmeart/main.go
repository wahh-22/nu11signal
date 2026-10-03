// Command readmeart renders the README's SVG art from the app itself: the
// emblem, the logo (radio.EmblemArt), as an icon tile. The output is
// deterministic, so a rerun changes no bytes. Run it from the repository root:
//
//	go run ./tools/readmeart
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// An asset is one generated file, its path relative to the repository root.
type asset struct {
	path string
	data []byte
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	assets := render()
	for _, a := range assets {
		p := filepath.Join(*root, a.path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "readmeart:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(p, a.data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "readmeart:", err)
			os.Exit(1)
		}
		fmt.Println(a.path)
	}
}

// render builds every asset.
func render() []asset {
	return []asset{
		{"docs/assets/brand/nu11signal-emblem.svg", emblemIcon()},
	}
}
