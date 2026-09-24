package storage

import "strings"

// ArticleHTML is the fragment to render inside an article page.
// A full document contributes the contents of its body element, so the
// site chrome, theme, and article layout stay in place. A fragment that
// does not contain a body element is used as-is.
func ArticleHTML(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	lower := strings.ToLower(s)
	start := strings.Index(lower, "<body")
	if start < 0 {
		return s
	}
	rel := strings.Index(s[start:], ">")
	if rel < 0 {
		return s
	}
	inner := s[start+rel+1:]
	if end := strings.LastIndex(strings.ToLower(inner), "</body>"); end >= 0 {
		inner = inner[:end]
	}
	return strings.TrimSpace(inner)
}
