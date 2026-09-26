package storage

import (
	"strings"
)

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

// ExtractStyles returns the concatenated contents of every <style> block
// in raw. A full uploaded document keeps its look after ArticleHTML
// drops the <head>; without this, those rules never reach the page.
func ExtractStyles(raw []byte) string {
	s := string(raw)
	var b strings.Builder
	for {
		lower := strings.ToLower(s)
		i := strings.Index(lower, "<style")
		if i < 0 {
			break
		}
		gt := strings.Index(s[i:], ">")
		if gt < 0 {
			break
		}
		start := i + gt + 1
		endRel := strings.Index(strings.ToLower(s[start:]), "</style>")
		if endRel < 0 {
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s[start : start+endRel])
		s = s[start+endRel+len("</style>"):]
	}
	return strings.TrimSpace(b.String())
}

// ScopeCSS rewrites :root / html / body selectors so they apply only
// under root (e.g. ".article-body"). Full uploaded documents reset
// body and :root; without this those rules restyle the site header.
func ScopeCSS(css, root string) string {
	if css == "" || root == "" {
		return css
	}
	css = strings.ReplaceAll(css, ":root", root)
	css = replaceBareIdent(css, "html", root)
	return replaceBareIdent(css, "body", root)
}

// replaceBareIdent rewrites a CSS type selector without a lookahead
// (Go's regexp does not support (?=)). "html" in a URL and "body" in
// "somebody" stay put because they are not bounded as selectors.
func replaceBareIdent(css, ident, repl string) string {
	var b strings.Builder
	lower := strings.ToLower(css)
	identL := strings.ToLower(ident)
	i := 0
	for {
		j := strings.Index(lower[i:], identL)
		if j < 0 {
			b.WriteString(css[i:])
			return b.String()
		}
		j += i
		after := j + len(ident)
		prevOK := j == 0 || isSelectorSep(css[j-1])
		nextOK := after == len(css) || isSelectorEnd(css[after])
		if prevOK && nextOK {
			b.WriteString(css[i:j])
			b.WriteString(repl)
			i = after
			continue
		}
		b.WriteString(css[i:after])
		i = after
	}
}

func isSelectorSep(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', ',', '{', '}', '>':
		return true
	}
	return false
}

func isSelectorEnd(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', ',', '{', ':', '.', '#', '[', '>':
		return true
	}
	return false
}

// StripStyleTags drops every <style>…</style> block so uploaded
// documents cannot override the site's typography and layout. Scripts
// and markup stay; the page CSS restyles whatever tags remain.
func StripStyleTags(html string) string {
	s := html
	var b strings.Builder
	for {
		lower := strings.ToLower(s)
		i := strings.Index(lower, "<style")
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		endRel := strings.Index(lower[i:], "</style>")
		if endRel < 0 {
			break
		}
		s = s[i+endRel+len("</style>"):]
	}
	return strings.TrimSpace(b.String())
}
