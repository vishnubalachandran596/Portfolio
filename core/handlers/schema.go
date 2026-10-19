package handlers

import (
	"encoding/json"
	"html/template"
	"strconv"
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
	ID          string   `json:"@id,omitempty"`
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
// loaded — each group's individual skills. The group headings (e.g.
// "Languages & Frameworks") are section labels, not skills, so they are
// left out.
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
		out = append(out, g.Items...)
	}
	return out
}

type schemaRef struct {
	Type string `json:"@type"`
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type offerSchema struct {
	Type          string `json:"@type"`
	Price         string `json:"price"`
	PriceCurrency string `json:"priceCurrency"`
	Availability  string `json:"availability,omitempty"`
	URL           string `json:"url,omitempty"`
}

type courseInstanceSchema struct {
	Type       string `json:"@type"`
	CourseMode string `json:"courseMode"`
}

type courseSchema struct {
	Context           string                 `json:"@context"`
	Type              string                 `json:"@type"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	URL               string                 `json:"url"`
	Image             string                 `json:"image,omitempty"`
	Keywords          string                 `json:"keywords,omitempty"`
	EducationalLevel  string                 `json:"educationalLevel,omitempty"`
	Provider          schemaRef              `json:"provider"`
	Offers            *offerSchema           `json:"offers,omitempty"`
	HasCourseInstance []courseInstanceSchema `json:"hasCourseInstance,omitempty"`
}

// priceString renders a minor-unit amount the way schema.org wants a
// price: a plain decimal, no currency symbol or grouping.
func priceString(cents int) string {
	if cents%100 == 0 {
		return strconv.Itoa(cents / 100)
	}
	return strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}

func personAndWebsiteJSONLD(profile models.Profile, homeURL, imageURL string, sameAs []string, skills []models.SkillGroup) template.JS {
	person := personSchema{
		Context:     "https://schema.org",
		Type:        "Person",
		ID:          homeURL + "#person",
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
	// The Service block is derived from the profile rather than carrying
	// its own copy: the description is the profile's summary (falling back
	// to the hero subtitle), so editing the profile in the admin console
	// updates the structured data too.
	serviceDescription := profile.Summary
	if serviceDescription == "" {
		serviceDescription = profile.HeroSubtitle
	}
	service := serviceSchema{
		Context:     "https://schema.org",
		Type:        "Service",
		ServiceType: "Technical Consulting",
		Name:        "Technical consulting by " + profile.Name,
		Description: serviceDescription,
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
	Context          string    `json:"@context"`
	Type             string    `json:"@type"`
	Headline         string    `json:"headline"`
	Description      string    `json:"description"`
	URL              string    `json:"url"`
	MainEntityOfPage schemaRef `json:"mainEntityOfPage"`
	Image            string    `json:"image,omitempty"`
	DatePublished    string    `json:"datePublished,omitempty"`
	Keywords         string    `json:"keywords,omitempty"`
	Author           schemaRef `json:"author"`
	Publisher        schemaRef `json:"publisher"`
}

// articleJSONLD marks an article up as a BlogPosting. description and
// image are the already-resolved SEO values (meta_description fallback
// chain, og_image fallback), so the structured data always agrees with the
// tags in the page head. This used to be gated on the article having a
// body of its own, because the site also carried curated link-outs — that
// second kind of article no longer exists (migration 040), so every
// article is genuinely this site's own writing and the gate is gone.
func articleJSONLD(a models.Article, pageURL, authorName, description, image string) template.JS {
	s := articleSchema{
		Context:          "https://schema.org",
		Type:             "BlogPosting",
		Headline:         a.Title,
		Description:      description,
		URL:              pageURL,
		MainEntityOfPage: schemaRef{Type: "WebPage", Name: a.Title, URL: pageURL},
		Image:            image,
		Author:           schemaRef{Type: "Person", Name: authorName},
		Publisher:        schemaRef{Type: "Person", Name: authorName},
	}
	if a.PublishedDate != nil {
		s.DatePublished = a.PublishedDate.Format("2006-01-02")
	}
	if len(a.Tags) > 0 {
		s.Keywords = strings.Join(a.Tags, ", ")
	}
	return toJSONLD(s)
}

// courseJSONLD builds the Course block from the course row itself: title,
// resolved description and image, tags as keywords, level, an INR Offer
// priced from the stored minimum, and an online CourseInstance. Nothing
// here is literal copy.
func courseJSONLD(c models.Course, description, pageURL, image, providerName string) template.JS {
	s := courseSchema{
		Context:          "https://schema.org",
		Type:             "Course",
		Name:             c.Title,
		Description:      description,
		URL:              pageURL,
		Image:            image,
		EducationalLevel: c.Level,
		Provider:         schemaRef{Type: "Person", Name: providerName},
		HasCourseInstance: []courseInstanceSchema{
			{Type: "CourseInstance", CourseMode: "online"},
		},
	}
	if len(c.Tags) > 0 {
		s.Keywords = strings.Join(c.Tags, ", ")
	}
	price := 0
	if c.IsPaid {
		price = c.MinPriceCents
		if price == 0 {
			price = c.PriceCents
		}
	}
	s.Offers = &offerSchema{
		Type:          "Offer",
		Price:         priceString(price),
		PriceCurrency: models.CurrencyINR,
		Availability:  "https://schema.org/InStock",
		URL:           pageURL,
	}
	return toJSONLD(s)
}

type softwareApplicationSchema struct {
	Context         string      `json:"@context"`
	Type            string      `json:"@type"`
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	URL             string      `json:"url"`
	Image           string      `json:"image,omitempty"`
	SoftwareVersion string      `json:"softwareVersion,omitempty"`
	Offers          offerSchema `json:"offers"`
}

func softwareApplicationJSONLD(sw models.Software, pageURL, description, image string) template.JS {
	s := softwareApplicationSchema{
		Context:         "https://schema.org",
		Type:            "SoftwareApplication",
		Name:            sw.Title,
		Description:     description,
		URL:             pageURL,
		Image:           image,
		SoftwareVersion: sw.Version,
		Offers: offerSchema{
			Type:          "Offer",
			Price:         priceString(sw.PriceCents),
			PriceCurrency: models.CurrencyINR,
			Availability:  "https://schema.org/InStock",
			URL:           pageURL,
		},
	}
	return toJSONLD(s)
}

// crumb is one BreadcrumbList step.
type crumb struct {
	Name string
	URL  string
}

type breadcrumbItemSchema struct {
	Type     string `json:"@type"`
	Position int    `json:"position"`
	Name     string `json:"name"`
	Item     string `json:"item"`
}

type breadcrumbSchema struct {
	Context         string                 `json:"@context"`
	Type            string                 `json:"@type"`
	ItemListElement []breadcrumbItemSchema `json:"itemListElement"`
}

// breadcrumbJSONLD builds a BreadcrumbList from the trail a handler
// already knows (Home > Courses > this course). Names come from the
// database rows or the nav labels; URLs are absolute.
func breadcrumbJSONLD(trail []crumb) template.JS {
	items := make([]breadcrumbItemSchema, 0, len(trail))
	for i, c := range trail {
		items = append(items, breadcrumbItemSchema{Type: "ListItem", Position: i + 1, Name: c.Name, Item: c.URL})
	}
	return toJSONLD(breadcrumbSchema{
		Context:         "https://schema.org",
		Type:            "BreadcrumbList",
		ItemListElement: items,
	})
}
