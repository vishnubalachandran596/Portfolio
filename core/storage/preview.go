package storage

import (
	"regexp"
	"strings"
)

// PreviewEndMarker ends the free public preview of a course lesson. The
// author places it in the lesson HTML where the trial should stop;
// NormalizeLesson leaves it untouched (it is an ordinary comment) and
// LessonPreview cuts the document there.
const PreviewEndMarker = "<!-- vb:preview-end -->"

var (
	h2Open     = regexp.MustCompile(`(?i)<h2\b`)
	h3Open     = regexp.MustCompile(`(?i)<h3\b`)
	scriptOnly = regexp.MustCompile(`(?is)<script\b.*?</script>`)
)

// LessonPreview returns the part of a stored lesson that is shown free to
// everyone, and whether the lesson has one.
//
//   - With a PreviewEndMarker the preview is everything before it.
//   - Without one the preview is the first section only: everything before
//     the second <h2> (or the second <h3> when the lesson has no <h2>s).
//   - A lesson with neither a marker nor a second heading has no preview,
//     so a one-section lesson is never given away whole.
//
// The tail is dropped, but its <script> blocks are kept (they usually
// drive the interactive pieces inside the preview) and the document is
// closed, so PrepareLesson, ArticleHTML and ExtractStyles treat the
// result like any other lesson. Content after the cut never reaches the
// browser.
func LessonPreview(raw []byte) ([]byte, bool) {
	s := string(raw)
	cut := strings.Index(s, PreviewEndMarker)
	if cut < 0 {
		cut = secondHeading(s, h2Open)
		if cut < 0 {
			cut = secondHeading(s, h3Open)
		}
	}
	if cut <= 0 {
		return nil, false
	}
	head, tail := s[:cut], s[cut:]
	var b strings.Builder
	b.WriteString(head)
	b.WriteString("\n")
	for _, sc := range scriptOnly.FindAllString(tail, -1) {
		b.WriteString(sc)
		b.WriteString("\n")
	}
	b.WriteString("</body></html>")
	return []byte(b.String()), true
}

// HasPreview reports whether LessonPreview would produce a preview.
func HasPreview(raw []byte) bool {
	_, ok := LessonPreview(raw)
	return ok
}

// secondHeading returns the byte offset of the second match of re that
// sits outside a <script>/<style>/<template> block, or -1.
func secondHeading(s string, re *regexp.Regexp) int {
	skip := scriptPattern.FindAllStringIndex(s, -1)
	seen := 0
	for _, m := range re.FindAllStringIndex(s, -1) {
		inside := false
		for _, sp := range skip {
			if m[0] >= sp[0] && m[0] < sp[1] {
				inside = true
				break
			}
		}
		if inside {
			continue
		}
		seen++
		if seen == 2 {
			return m[0]
		}
	}
	return -1
}
