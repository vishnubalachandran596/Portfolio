package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestSkillsGridCollapsedHasNoFaceSummary(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatalf("view.New: %v", err)
	}

	base := BaseData{
		SiteName: "Vishnu Balachandran", Title: "Test", MetaDescription: "desc",
		Active: "home", CanonicalURL: "https://example.com/", OGImageURL: "https://example.com/og.png",
		AssetVersion: "test",
		Nav: NavLinks{
			Home: "/", About: "/about", Courses: "/courses", Consultancy: "/consultancy",
			Articles: "/articles", Contact: "/contact", Search: "/search", Login: "/login",
			Privacy: "/privacy", Terms: "/terms", Refund: "/refund", Cookies: "/cookies",
		},
	}
	skills := []models.SkillGroup{
		{ID: 1, Category: "Java & Spring", Level: "Expert", Achievement: "Shipped pricing engines.", Items: []string{"Java", "Spring", "Kafka", "Postgres", "Redis"}},
		{ID: 2, Category: "Short", Level: "Advanced", Achievement: "Four tags only.", Items: []string{"A", "B", "C", "D"}},
	}
	profile := models.Profile{
		Name: "Vishnu Balachandran", Title: "Technical Consultant", HeroSubtitle: "subtitle",
		Summary: "Paragraph one of the professional summary.\n\nParagraph two of the professional summary.",
	}

	home := renderPage(t, r, "home", HomeData{BaseData: base, Profile: profile, Skills: skills})
	aboutBase := base
	aboutBase.Active = "about"
	about := renderPage(t, r, "about", AboutData{BaseData: aboutBase, Profile: profile, Skills: skills})

	for name, body := range map[string]string{"home": home, "about": about} {
		if strings.Contains(body, "skill-achievement-face") {
			t.Errorf("%s: collapsed face still carries skill-achievement-face", name)
		}
		if strings.Contains(body, "SHOW THE DETAIL") {
			t.Errorf("%s: SHOW THE DETAIL still present", name)
		}
		if !strings.Contains(body, `class="skill-card-toggle"`) {
			t.Errorf("%s: missing skill-card-toggle button", name)
		}
		if !strings.Contains(body, `aria-expanded="false"`) {
			t.Errorf("%s: missing aria-expanded", name)
		}
		if !strings.Contains(body, "and 1 more") {
			t.Errorf("%s: missing and N more for the 5-item group", name)
		}
		if strings.Count(body, `class="skill-card"`) != 2 {
			t.Errorf("%s: expected 2 skill cards", name)
		}
		if !strings.Contains(body, "Shipped pricing engines.") {
			t.Errorf("%s: achievement text missing entirely", name)
		}
		i := strings.Index(body, `class="skill-card-toggle"`)
		if i >= 0 {
			rest := body[i:]
			end := strings.Index(rest, `</button>`)
			if end >= 0 && strings.Contains(rest[:end], "Shipped pricing engines.") {
				t.Errorf("%s: achievement leaked into the toggle button", name)
			}
		}
		idx := strings.Index(body, "Shipped pricing engines.")
		if idx >= 0 {
			window := body[:idx]
			if !strings.Contains(window[max(0, len(window)-400):], "skill-card-detail") {
				t.Errorf("%s: achievement does not sit inside skill-card-detail", name)
			}
		}
	}

	if !strings.Contains(home, "What I bring") {
		t.Error("home: missing What I bring heading")
	}
	if !strings.Contains(home, "Paragraph one of the professional summary.") {
		t.Error("home: missing professional summary")
	}
	if strings.Contains(home, "feature-card") || strings.Contains(home, "What that has actually produced") {
		t.Error("home: leftover proof-point cards or lead copy")
	}
	bring := between(home, "What I bring", "From the same work I do for clients")
	if !strings.Contains(bring, "home-bring-summary") || !strings.Contains(bring, "skills-grid") {
		t.Error("home: What I bring must hold the summary then the skills grid")
	}
	if strings.Contains(bring, "feature-grid") {
		t.Error("home: extra feature-grid inside What I bring")
	}
	if !strings.Contains(about, "Skills") {
		t.Error("about: Skills heading missing")
	}
}

func renderPage(t *testing.T, r *view.Renderer, page string, data any) string {
	t.Helper()
	rec := httptest.NewRecorder()
	r.Render(rec, 200, page, data)
	body := rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatalf("%s render error: %s", page, body[strings.Index(body, "<!-- render error:"):])
	}
	return body
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	j := strings.Index(s, b)
	if i < 0 || j < 0 || j <= i {
		return s
	}
	return s[i:j]
}
