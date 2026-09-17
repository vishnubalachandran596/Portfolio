package handlers

import (
	"encoding/json"
	"html/template"

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

// knownSkills is the fixed keyword set surfaced in the Person schema's
// knowsAbout — the actual, established areas of expertise behind the
// site's real content (courses, experience, consultancy), used for
// search relevance rather than pulled dynamically per page.
var knownSkills = []string{
	"Java", "Technical Architecture", "Software Architecture", "System Design",
	"Microservices", "Event-Driven Architecture", "Apache Kafka", "Spring Boot",
	"Distributed Systems", "Banking & Fintech Platforms", "Cloud Architecture",
	"Engineering Leadership", "Technical Consulting",
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

func personAndWebsiteJSONLD(profile models.Profile, homeURL string, sameAs []string) template.JS {
	person := personSchema{
		Context:     "https://schema.org",
		Type:        "Person",
		Name:        profile.Name,
		JobTitle:    profile.Title,
		Description: profile.HeroSubtitle,
		URL:         homeURL,
		Email:       profile.Email,
		SameAs:      sameAs,
		KnowsAbout:  knownSkills,
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
