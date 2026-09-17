package handlers

import (
	"encoding/json"
	"html/template"
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
	Context  string   `json:"@context"`
	Type     string   `json:"@type"`
	Name     string   `json:"name"`
	JobTitle string   `json:"jobTitle,omitempty"`
	URL      string   `json:"url"`
	SameAs   []string `json:"sameAs,omitempty"`
	Address  *struct {
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

type graphSchema struct {
	Context string `json:"@context"`
	Graph   []any  `json:"@graph"`
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

func personAndWebsiteJSONLD(name, title, homeURL, location string, sameAs []string) template.JS {
	person := personSchema{
		Context:  "https://schema.org",
		Type:     "Person",
		Name:     name,
		JobTitle: title,
		URL:      homeURL,
		SameAs:   sameAs,
	}
	if location != "" {
		person.Address = &struct {
			Type            string `json:"@type"`
			AddressLocality string `json:"addressLocality"`
		}{Type: "PostalAddress", AddressLocality: location}
	}
	website := websiteSchema{
		Context: "https://schema.org",
		Type:    "WebSite",
		Name:    name,
		URL:     homeURL,
	}
	return toJSONLD(graphSchema{
		Context: "https://schema.org",
		Graph:   []any{person, website},
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
