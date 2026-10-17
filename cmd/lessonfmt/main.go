// Command lessonfmt applies storage.NormalizeLesson to lesson or article
// HTML files on disk — the offline half of the content-update strategy in
// docs/DESIGN.md ("Content updates"). Uploads through the admin console
// are normalized automatically; this is for files edited by hand or
// pulled from Storage.
//
//	go run ./cmd/lessonfmt -out ./normalized courses/*.html
//
// Each input is written to -out under the same base name, and a one-line
// report per file says what changed. Inputs are never modified in place.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/storage"
)

var langRe = regexp.MustCompile(`class="[^"]*\blanguage-([a-z]+)`)

func main() {
	out := flag.String("out", "", "directory to write normalized files into (required)")
	flag.Parse()
	if *out == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: lessonfmt -out DIR file.html...")
		os.Exit(2)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	failed := false
	for _, path := range flag.Args() {
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, path, err)
			failed = true
			continue
		}
		norm := storage.NormalizeLesson(raw)
		dst := filepath.Join(*out, filepath.Base(path))
		if err := os.WriteFile(dst, norm, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, dst, err)
			failed = true
			continue
		}
		langs := map[string]int{}
		for _, m := range langRe.FindAllStringSubmatch(string(norm), -1) {
			langs[m[1]]++
		}
		var parts []string
		for l, n := range langs {
			parts = append(parts, fmt.Sprintf("%s=%d", l, n))
		}
		fmt.Printf("%-60s %7d -> %7d bytes  sections=%d  code[%s]\n", filepath.Base(path), len(raw), len(norm), len(storage.LessonOutline(norm)), strings.Join(parts, " "))
	}
	if failed {
		os.Exit(1)
	}
}
