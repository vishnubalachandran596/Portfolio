package storage

import (
	"html"
	"regexp"
	"strings"
)

var (
	h2Pattern    = regexp.MustCompile(`(?is)<h2\b[^>]*>(.*?)</h2>`)
	h3Pattern    = regexp.MustCompile(`(?is)<h3\b[^>]*>(.*?)</h3>`)
	tagPattern   = regexp.MustCompile(`(?s)<[^>]+>`)
	spacePattern = regexp.MustCompile(`\s+`)
	// A leading "1." / "2)" / "Step 3:" is dropped: the outline list
	// numbers its own entries.
	numberPrefix    = regexp.MustCompile(`^(?i:(?:part|step|section|lesson|module|chapter)\s+)?\d+(?:\.\d+)*\s*[.):—–-]\s*`)
	scriptPattern   = regexp.MustCompile(`(?is)<(script|style|template)\b.*?</(script|style|template)>`)
	maxOutlineItems = 40
)

// LessonOutline returns a lesson's section titles in document order: its
// <h2> headings, or its <h3> headings when it has no <h2> at all. These
// become the "What's inside" list on the public course detail page
// (courses.outline, migration 079) — a syllabus, not the lesson content.
//
// Inner markup is stripped and entities decoded so a heading like
// `<h2>1. <code>HashMap</code> internals</h2>` reads "1. HashMap
// internals". Script/style blocks are removed first so a heading string
// inside a lab's JavaScript can never leak in. Duplicates are dropped
// and the list is capped, so a 138-problem lab page cannot produce a
// page-long outline.
func LessonOutline(raw []byte) []string {
	doc := scriptPattern.ReplaceAllString(string(raw), "")
	matches := h2Pattern.FindAllStringSubmatch(doc, -1)
	if len(matches) == 0 {
		matches = h3Pattern.FindAllStringSubmatch(doc, -1)
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		text := html.UnescapeString(tagPattern.ReplaceAllString(m[1], " "))
		text = strings.TrimSpace(spacePattern.ReplaceAllString(text, " "))
		text = strings.TrimSpace(numberPrefix.ReplaceAllString(text, ""))
		if text == "" || len(text) > 140 || seen[strings.ToLower(text)] {
			continue
		}
		seen[strings.ToLower(text)] = true
		out = append(out, text)
		if len(out) == maxOutlineItems {
			break
		}
	}
	return out
}
