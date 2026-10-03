package handlers

import (
	"encoding/json"
	"html/template"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// toJSONLD marshals v to JSON and wraps it as template.JS so html/template
// inserts it verbatim inside a <script type="application/ld+json"> block.
// encoding/json already HTML-escapes '<', '>', and '&' by default, which is
// what makes this safe to embed directly in a <script> tag.
func toJSONLD(v any) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return template.JS(b)
}

type personSchema struct {
	Context     string   `json:"@context"`
	Type        string   `json:"@type"`
	Name        string   `json:"name"`
	JobTitle    string   `json:"jobTitle,omitempty"`
	Description string   `json:"description,omitempty"`
	URL         string   `json:"url"`
	Email       string   `json:"email,omitempty"`
	Image       string   `json:"image,omitempty"`
	SameAs      []string `json:"sameAs,omitempty"`
	KnowsAbout  []string `json:"knowsAbout,omitempty"`
	Address     *struct {
		Type            string `json:"@type"`
		AddressLocality string `json:"addressLocality"`
	} `json:"address,omitempty"`
}

type websiteSchema struct {
	Context string `json:"@context"`
	Type    string `json:"@type"`
	Name    string `json:"name"`
	URL     string `json:"url"`
}

// serviceSchema surfaces the consultancy offering itself as a discoverable
// entity — helps for "hire a technical consultant" / "Java architect
// consulting" style searches, not just the person's own name.
type serviceSchema struct {
	Context     string `json:"@context"`
	Type        string `json:"@type"`
	ServiceType string `json:"serviceType"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Provider    struct {
		Type string `json:"@type"`
		Name string `json:"name"`
	} `json:"provider"`
	AreaServed string `json:"areaServed"`
}

type graphSchema struct {
	Context string `json:"@context"`
	Graph   []any  `json:"@graph"`
}

// professionalServiceSchema is the Consultancy page's own entity — Service
// on the home page covers the general "hire a technical consultant"
// search; this one is more specific (ProfessionalService, with a
// priceRange), attached to the page that actually describes the offer in
// detail, which the generic Service schema never was.
type professionalServiceSchema struct {
	Context     string `json:"@context"`
	Type        string `json:"@type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Provider    struct {
		Type string `json:"@type"`
		Name string `json:"name"`
	} `json:"provider"`
	AreaServed string `json:"areaServed"`
	PriceRange string `json:"priceRange,omitempty"`
	URL        string `json:"url"`
}

// FAQItem is one question/answer pair on the Consultancy page. It is the
// single source for both the visible <details> accordion and the FAQPage
// schema below — building the schema from a second, independently
// maintained copy of the same six questions is exactly the drift CLAUDE.md
// rule #1 warns about (the money-formatting bug), so consultancy.go builds
// one slice and both the template and schema.go read it.
type FAQItem struct {
	Question string
	Answer   string
}

type faqAnswerSchema struct {
	Type string `json:"@type"`
	Text string `json:"text"`
}

type faqQuestionSchema struct {
	Type           string          `json:"@type"`
	Name           string          `json:"name"`
	AcceptedAnswer faqAnswerSchema `json:"acceptedAnswer"`
}

// consultancyJSONLD combines the page's ProfessionalService entity and its
// FAQPage into one @graph, following the exact pattern
// personAndWebsiteJSONLD already uses for Home.
func consultancyJSONLD(pageURL, providerName, description, priceRange string, faq []FAQItem) template.JS {
	service := professionalServiceSchema{
		Context:     "https://schema.org",
		Type:        "ProfessionalService",
		Name:        "Technical Consultancy — Architecture Review & Fractional Advisory",
		Description: description,
		AreaServed:  "Worldwide",
		PriceRange:  priceRange,
		URL:         pageURL,
	}
	service.Provider.Type = "Person"
	service.Provider.Name = providerName

	questions := make([]faqQuestionSchema, 0, len(faq))
	for _, item := range faq {
		q := faqQuestionSchema{Type: "Question", Name: item.Question}
		q.AcceptedAnswer = faqAnswerSchema{Type: "Answer", Text: item.Answer}
		questions = append(questions, q)
	}
	faqPage := struct {
		Type       string              `json:"@type"`
		MainEntity []faqQuestionSchema `json:"mainEntity"`
	}{Type: "FAQPage", MainEntity: questions}

	return toJSONLD(graphSchema{
		Context: "https://schema.org",
		Graph:   []any{service, faqPage},
	})
}

// knowsAbout is the keyword set surfaced in the Person schema's
// knowsAbout, derived from the skill groups the home page has already
// loaded — each group's category plus its individual skills.
//
// This used to be a hardcoded slice maintained separately in Go, and it
// had drifted: the skill groups were re-cut and an AI/agentic-engineering
// group added, while the slice still listed the old vocabulary, so a
// crawler read a different skill set than a visitor. There is nothing to
// keep in sync now — skill_groups is the single source, as it is for the
// grid the page renders.
func knowsAbout(skills []models.SkillGroup) []string {
	out := make([]string, 0, len(skills)*4)
	for _, g := range skills {
		if g.Category != "" {
			out = append(out, g.Category)
		}
		out = append(out, g.Items...)
	}
	return out
}

type courseSchema struct {
	Context     string `json:"@context"`
	Type        string `json:"@type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Provider    struct {
		Type string `json:"@type"`
		Name string `json:"name"`
	} `json:"provider"`
}

func personAndWebsiteJSONLD(profile models.Profile, homeURL, imageURL string, sameAs []string, skills []models.SkillGroup) template.JS {
	person := personSchema{
		Context:     "https://schema.org",
		Type:        "Person",
		Name:        profile.Name,
		JobTitle:    profile.Title,
		Description: profile.HeroSubtitle,
		URL:         homeURL,
		Email:       profile.Email,
		Image:       imageURL,
		SameAs:      sameAs,
		KnowsAbout:  knowsAbout(skills),
	}
	if profile.Location != "" {
		person.Address = &struct {
			Type            string `json:"@type"`
			AddressLocality string `json:"addressLocality"`
		}{Type: "PostalAddress", AddressLocality: profile.Location}
	}
	website := websiteSchema{
		Context: "https://schema.org",
		Type:    "WebSite",
		Name:    profile.Name,
		URL:     homeURL,
	}
	service := serviceSchema{
		Context:     "https://schema.org",
		Type:        "Service",
		ServiceType: "Technical Consulting",
		Name:        "Technical Consulting — Architecture, System Design & Engineering Leadership",
		Description: "1:1 technical consultancy for architecture reviews, system design decisions, and engineering leadership, from a Java technical expert with 8+ years in banking and fintech platforms.",
		AreaServed:  "Worldwide",
	}
	service.Provider.Type = "Person"
	service.Provider.Name = profile.Name
	return toJSONLD(graphSchema{
		Context: "https://schema.org",
		Graph:   []any{person, website, service},
	})
}

// articleSchema marks up a blog post as a BlogPosting — the page search
// engines index, and the page the writing actually lives on.
type articleSchema struct {
	Context       string `json:"@context"`
	Type          string `json:"@type"`
	Headline      string `json:"headline"`
	Description   string `json:"description"`
	URL           string `json:"url"`
	Image         string `json:"image,omitempty"`
	DatePublished string `json:"datePublished,omitempty"`
	Keywords      string `json:"keywords,omitempty"`
	Author        struct {
		Type string `json:"@type"`
		Name string `json:"name"`
	} `json:"author"`
}

// articleJSONLD marks an article up as a BlogPosting. This used to be
// gated on the article having a body of its own, because the site also
// carried curated link-outs — a one-line summary of someone else's post,
// which would have been thin and misleading structured data to claim as
// original. That second kind of article no longer exists (migration 040),
// so every article is genuinely this site's own writing and the gate is
// gone with it.
func articleJSONLD(a models.Article, pageURL, authorName string) template.JS {
	s := articleSchema{
		Context:     "https://schema.org",
		Type:        "BlogPosting",
		Headline:    a.Title,
		Description: a.Summary,
		URL:         pageURL,
		Image:       a.CoverImageURL,
	}
	if a.PublishedDate != nil {
		s.DatePublished = a.PublishedDate.Format("2006-01-02")
	}
	if len(a.Tags) > 0 {
		s.Keywords = strings.Join(a.Tags, ", ")
	}
	s.Author.Type = "Person"
	s.Author.Name = authorName
	return toJSONLD(s)
}

func courseJSONLD(title, description, url, providerName string) template.JS {
	c := courseSchema{
		Context:     "https://schema.org",
		Type:        "Course",
		Name:        title,
		Description: description,
		URL:         url,
	}
	c.Provider.Type = "Person"
	c.Provider.Name = providerName
	return toJSONLD(c)
}
