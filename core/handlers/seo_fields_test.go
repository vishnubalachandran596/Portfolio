package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

func TestAbsoluteURLAndOGImageFallback(t *testing.T) {
	const base = "https://www.example.com"
	if got := absoluteURL(base, "/img/x.png"); got != base+"/img/x.png" {
		t.Errorf("relative: %q", got)
	}
	if got := absoluteURL(base, "https://cdn.example.com/x.png"); got != "https://cdn.example.com/x.png" {
		t.Errorf("absolute: %q", got)
	}
	if got := absoluteURL(base, ""); got != "" {
		t.Errorf("empty: %q", got)
	}
	// og_image -> cover image -> site default.
	if got := ogImageFor(base, models.SEOFields{OGImage: "/og/a.png"}, "/cover.png"); got != base+"/og/a.png" {
		t.Errorf("og_image: %q", got)
	}
	if got := ogImageFor(base, models.SEOFields{}, "/cover.png"); got != base+"/cover.png" {
		t.Errorf("cover: %q", got)
	}
	if got := ogImageFor(base, models.SEOFields{}, ""); got != base+defaultOGImagePath {
		t.Errorf("default: %q", got)
	}
}

func TestRobotsMetaForRequest(t *testing.T) {
	plain := httptest.NewRequest("GET", "/courses", nil)
	if got := robotsMetaForRequest(plain); got != "" {
		t.Errorf("indexable listing: %q", got)
	}
	if got := robotsMetaForRequest(httptest.NewRequest("GET", "/courses?q=kafka", nil)); got != "noindex, follow" {
		t.Errorf("internal search: %q", got)
	}
	if got := robotsMetaForRequest(httptest.NewRequest("GET", "/articles?q=x", nil)); got != "noindex, follow" {
		t.Errorf("articles search: %q", got)
	}
	if got := robotsMetaForRequest(httptest.NewRequest("GET", "/courses?tag=Java", nil)); got != "" {
		t.Errorf("tag filter is canonicalised, not noindexed: %q", got)
	}
	if got := robotsMetaForRequest(markNoindex(httptest.NewRequest("GET", "/login", nil))); got != "noindex, nofollow" {
		t.Errorf("noindex middleware: %q", got)
	}
}

func TestPageSEOApplies(t *testing.T) {
	cases := map[string]bool{
		"/":                    true,
		"/about":               true,
		"/courses":             true,
		"/courses?tag=Java":    false, // keeps its tag-bearing title pattern
		"/courses?page=2":      false,
		"/articles?q=x":        false,
		"/courses/some-course": false, // detail pages use their own columns
		"/login":               false,
		"/does-not-exist":      false,
	}
	for url, want := range cases {
		if got := pageSEOApplies(httptest.NewRequest("GET", url, nil)); got != want {
			t.Errorf("pageSEOApplies(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestStaticLastmodAndSitemapRender(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }
	dates := repo.SitemapDates{
		LatestCourse: day(5), LatestArticle: day(7), LatestSoftware: day(3), ProfileUpdated: day(2),
	}
	if got := staticLastmod("/courses", dates, nil); !got.Equal(day(5)) {
		t.Errorf("/courses: %v", got)
	}
	if got := staticLastmod("/", dates, nil); !got.Equal(day(7)) {
		t.Errorf("/ takes the newest content date: %v", got)
	}
	if got := staticLastmod("/about", dates, &models.PageSEO{UpdatedAt: day(9)}); !got.Equal(day(9)) {
		t.Errorf("a page_seo edit counts as a real edit: %v", got)
	}
	if got := staticLastmod("/privacy", dates, nil); !got.IsZero() {
		t.Errorf("unknown lastmod must stay zero, got %v", got)
	}

	xml := renderSitemap([]sitemapEntry{
		{"https://x.test/courses", day(5)},
		{"https://x.test/privacy", time.Time{}},
		{"https://x.test/a?b=1&c=2", day(1)},
	})
	if strings.Contains(xml, "priority") {
		t.Error("priority must be omitted")
	}
	if !strings.Contains(xml, "<loc>https://x.test/courses</loc><lastmod>2026-10-05</lastmod>") {
		t.Errorf("lastmod missing: %s", xml)
	}
	if !strings.Contains(xml, "<url><loc>https://x.test/privacy</loc></url>") {
		t.Errorf("unknown lastmod should omit the tag: %s", xml)
	}
	if !strings.Contains(xml, "b=1&amp;c=2") {
		t.Errorf("loc must be XML-escaped: %s", xml)
	}
}

func TestSitemapStaticPathsExcludeThinPages(t *testing.T) {
	for _, p := range sitemapStaticPaths {
		if p == "/ai-assistant" || p == "/resume.pdf" {
			t.Errorf("%s must not be in the sitemap", p)
		}
	}
}

func decode(t *testing.T, js string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatalf("invalid JSON-LD: %v\n%s", err, js)
	}
	return m
}

func TestCourseJSONLDFromDBFields(t *testing.T) {
	c := models.Course{Title: "Kafka in Depth", Level: "Intermediate", Tags: []string{"Kafka", "Streaming"},
		IsPaid: true, MinPriceCents: 49900}
	m := decode(t, string(courseJSONLD(c, "Learn Kafka.", "https://x.test/courses/kafka", "https://x.test/og.png", "Jane Doe")))
	if m["@type"] != "Course" || m["name"] != "Kafka in Depth" || m["description"] != "Learn Kafka." {
		t.Errorf("core fields: %v", m)
	}
	if m["educationalLevel"] != "Intermediate" || m["keywords"] != "Kafka, Streaming" {
		t.Errorf("level/keywords: %v", m)
	}
	offer := m["offers"].(map[string]any)
	if offer["price"] != "499" || offer["priceCurrency"] != "INR" {
		t.Errorf("offer: %v", offer)
	}
	if m["provider"].(map[string]any)["name"] != "Jane Doe" {
		t.Errorf("provider: %v", m["provider"])
	}
	// A free course is priced 0, not omitted.
	free := decode(t, string(courseJSONLD(models.Course{Title: "Free"}, "d", "https://x.test/c", "", "J")))
	if free["offers"].(map[string]any)["price"] != "0" {
		t.Errorf("free offer: %v", free["offers"])
	}
}

func TestSoftwareAndArticleAndBreadcrumbJSONLD(t *testing.T) {
	sw := decode(t, string(softwareApplicationJSONLD(models.Software{Title: "Temple", PriceCents: 499950, Version: "2"},
		"https://x.test/software/temple", "Desc", "https://x.test/i.png")))
	if sw["@type"] != "SoftwareApplication" || sw["offers"].(map[string]any)["price"] != "4999.50" {
		t.Errorf("software: %v", sw)
	}

	art := decode(t, string(articleJSONLD(models.Article{Title: "Post", Tags: []string{"a", "b"}},
		"https://x.test/articles/post", "Jane", "Meta desc", "https://x.test/og.png")))
	if art["@type"] != "BlogPosting" || art["description"] != "Meta desc" || art["keywords"] != "a, b" {
		t.Errorf("article: %v", art)
	}

	bc := decode(t, string(breadcrumbJSONLD([]crumb{{"Home", "https://x.test/"}, {"Courses", "https://x.test/courses"}})))
	items := bc["itemListElement"].([]any)
	if bc["@type"] != "BreadcrumbList" || len(items) != 2 || items[1].(map[string]any)["position"].(float64) != 2 {
		t.Errorf("breadcrumb: %v", bc)
	}
}

func TestPersonJSONLDDerivedFromProfile(t *testing.T) {
	profile := models.Profile{Name: "Jane Doe", Title: "Engineer", HeroSubtitle: "Hero line", Summary: "Profile summary."}
	out := string(personAndWebsiteJSONLD(profile, "https://x.test/", "", nil, []models.SkillGroup{
		{Category: "Languages & Frameworks", Items: []string{"Java", "Go"}},
	}))
	if !strings.Contains(out, `"@id":"https://x.test/#person"`) {
		t.Errorf("person @id missing: %s", out)
	}
	if strings.Contains(out, "Languages \\u0026 Frameworks") || strings.Contains(out, "Languages & Frameworks") {
		t.Errorf("knowsAbout must not include group headings: %s", out)
	}
	if !strings.Contains(out, `"Java"`) || !strings.Contains(out, "Profile summary.") {
		t.Errorf("skills/summary not derived from the profile: %s", out)
	}
}
