package view

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// The page policy has no 'unsafe-inline' for scripts, so a template that adds
// an inline <script> without the request nonce, or an on*= attribute, would
// silently stop working in the browser. This fails the build instead.
func TestTemplatesRespectScriptPolicy(t *testing.T) {
	script := regexp.MustCompile(`<script\b[^>]*>`)
	handler := regexp.MustCompile(`(?i)\son(click|submit|change|input|load|error|keydown|keyup|focus|blur|mouseover)\s*=`)
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, _ := templateFS.ReadFile(path)
		src := string(b)
		for _, tag := range script.FindAllString(src, -1) {
			if strings.Contains(tag, "src=") || strings.Contains(tag, `type="application/ld+json"`) || strings.Contains(tag, "nonce=") {
				continue
			}
			t.Errorf("%s: inline script without nonce: %s", path, tag)
		}
		if handler.MatchString(src) {
			t.Errorf("%s: inline on* attribute (use a data- attribute handled in static/js/actions.js)", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
