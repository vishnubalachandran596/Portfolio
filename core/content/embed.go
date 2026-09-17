// Package content embeds the full, gated content for paid courses. These
// are large, self-contained interactive HTML documents (their own CSS/JS,
// no dependency on the site's own layout), served only to a verified
// purchaser — never through /static, so there's no public URL for them.
//
// New paid course content is added by committing a new HTML file here and
// adding it to contentFiles; there's no admin-portal upload flow for this
// yet (that would need object storage, a separate decision).
package content

import "embed"

//go:embed courses
var files embed.FS

var contentFiles = map[string]string{
	"java-sse-ultimate-guide":       "courses/java-sse-ultimate-guide.html",
	"java-fundamentals":             "courses/java-fundamentals.html",
	"technical-architecture-design": "courses/technical-architecture-design.html",
	"system-design-deployment":      "courses/system-design-deployment.html",
	"kafka-messaging-caching":       "courses/kafka-messaging-caching.html",
	"resilient-scalable-systems":    "courses/resilient-scalable-systems.html",
}

// Get returns the raw HTML for a course's content_key, or ok=false if no
// content is registered for that key.
func Get(key string) (string, bool) {
	path, ok := contentFiles[key]
	if !ok {
		return "", false
	}
	b, err := files.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(b), true
}
