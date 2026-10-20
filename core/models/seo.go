package models

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Length guidance shown as live counters in the admin SEO fields. They are
// advice for the author, not hard limits the server enforces: a search
// engine truncates, it does not reject.
const (
	SEOTitleMax       = 60
	SEODescriptionMax = 160
)

// SEOFields are the four optional per-item SEO columns carried by
// courses, articles and software (migration 085). An empty field means
// "use the fallback chain" — see Title, Description and Image — so newly
// created content inherits sensible values with no code change.
type SEOFields struct {
	SEOTitle        string
	MetaDescription string
	PrimaryKeyword  string
	OGImage         string
}

// PageSEO is one static page's override row in page_seo, keyed by URL path.
type PageSEO struct {
	Path string
	SEOFields
	Noindex   bool
	UpdatedAt time.Time
}

// StaticSEOPage names a static page that has a page_seo row to edit.
type StaticSEOPage struct {
	Path  string
	Label string
}

// StaticSEOPages is the set of static pages whose title, description and
// OG image can be overridden from the admin SEO page. It is the only place
// the list lives; the request path is matched against it before any
// page_seo lookup, so other routes never pay for the query.
var StaticSEOPages = []StaticSEOPage{
	{"/", "Home"},
	{"/about", "About"},
	{"/courses", "Courses"},
	{"/software", "Software"},
	{"/consultancy", "Consultancy"},
	{"/articles", "Articles"},
	{"/contact", "Contact"},
	{"/ai-assistant", "AI Assistant"},
	{"/privacy", "Privacy Policy"},
	{"/temple-accounts/privacy", "Temple Accounts Privacy Policy"},
	{"/terms", "Terms of Service"},
	{"/refund", "Refund Policy"},
	{"/cookies", "Cookie Policy"},
}

// IsStaticSEOPath reports whether path is one of StaticSEOPages.
func IsStaticSEOPath(path string) bool {
	for _, p := range StaticSEOPages {
		if p.Path == path {
			return true
		}
	}
	return false
}

var (
	seoBoldPattern   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	seoItalicPattern = regexp.MustCompile(`\*(.+?)\*`)
	seoTagPattern    = regexp.MustCompile(`<[^>]*>`)
)

// SEOPlainText flattens an admin-authored field to plain text for a meta
// tag: the **bold** / *italic* markers that Summary fields allow, any
// stray HTML tags, and runs of whitespace are all removed.
func SEOPlainText(s string) string {
	s = seoTagPattern.ReplaceAllString(s, " ")
	s = seoBoldPattern.ReplaceAllString(s, "$1")
	s = seoItalicPattern.ReplaceAllString(s, "$1")
	return strings.Join(strings.Fields(s), " ")
}

// TruncateSEO shortens s to at most max characters, cutting at a word
// boundary and ending with an ellipsis. A string already within max is
// returned unchanged.
func TruncateSEO(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max || max < 2 {
		return s
	}
	runes := []rune(s)
	cut := string(runes[:max-1])
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.-—–") + "…"
}

// Title is the browser/search title: the explicit seo_title when set,
// otherwise fallback (the page's existing title pattern).
func (f SEOFields) Title(fallback string) string {
	if t := strings.TrimSpace(f.SEOTitle); t != "" {
		return t
	}
	return fallback
}

// Description is the meta description: meta_description when set, then the
// item's summary, then its description truncated to SEODescriptionMax.
func (f SEOFields) Description(summary, description string) string {
	if d := strings.TrimSpace(f.MetaDescription); d != "" {
		return d
	}
	if s := SEOPlainText(summary); s != "" {
		return s
	}
	return TruncateSEO(SEOPlainText(description), SEODescriptionMax)
}

// Image is the Open Graph image reference: og_image when set, otherwise
// fallback (the item's own cover image, or "" for the site default). The
// result may be site-relative; callers make it absolute.
func (f SEOFields) Image(fallback string) string {
	if i := strings.TrimSpace(f.OGImage); i != "" {
		return i
	}
	return strings.TrimSpace(fallback)
}
