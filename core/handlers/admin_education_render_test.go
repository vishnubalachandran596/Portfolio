package handlers

import (
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestEducationAdminAndAboutRender(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatalf("view.New: %v", err)
	}
	base := BaseData{
		SiteName: "Vishnu Balachandran", Title: "Education — Admin",
		Nav: NavLinks{Home: "/"},
	}
	entry := models.Education{
		ID: 4, Institution: "Anna University", Degree: "B.E.", Field: "Computer Science",
		StartYear: 2012, EndYear: 2016, Details: "First class", SortOrder: 1,
	}

	list := renderPage(t, r, "admin_education_list", AdminEducationListData{
		BaseData:  base,
		Education: []models.Education{entry},
	})
	if !strings.Contains(list, `href="/admin/education/4/edit"`) {
		t.Fatal("list missing edit link")
	}
	if !strings.Contains(list, "Anna University") || !strings.Contains(list, "2012") || !strings.Contains(list, "2016") {
		t.Fatal("list missing education fields")
	}
	if !strings.Contains(list, `data-admin-path="/admin/education"`) {
		t.Fatal("sidebar missing education link")
	}

	form := renderPage(t, r, "admin_education_form", AdminEducationFormData{
		BaseData:  base,
		Education: entry,
	})
	if !strings.Contains(form, `action="/admin/education/4/edit"`) {
		t.Fatal("edit form posts to the wrong path")
	}
	if !strings.Contains(form, `value="Anna University"`) || !strings.Contains(form, `value="2012"`) {
		t.Fatal("edit form missing stored values")
	}

	blankYears := renderPage(t, r, "admin_education_form", AdminEducationFormData{
		BaseData:  base,
		Education: models.Education{Institution: "Open University", Degree: "M.Sc."},
		IsNew:     true,
	})
	if !strings.Contains(blankYears, `action="/admin/education/new"`) {
		t.Fatal("new form posts to the wrong path")
	}
	if strings.Contains(blankYears, `name="start_year" value="0"`) || strings.Contains(blankYears, `name="end_year" value="0"`) {
		t.Fatal("blank years rendered as 0")
	}

	about := renderPage(t, r, "about", AboutData{
		BaseData: BaseData{
			SiteName: "Vishnu Balachandran", Title: "About", MetaDescription: "desc",
			Active: "about", CanonicalURL: "https://example.com/about", OGImageURL: "https://example.com/og.png",
			Nav: NavLinks{Home: "/", About: "/about", Courses: "/courses", Consultancy: "/consultancy", Articles: "/articles", Contact: "/contact", Search: "/search", Login: "/login", Privacy: "/privacy", Terms: "/terms", Refund: "/refund", Cookies: "/cookies"},
		},
		Education: []models.Education{
			entry,
			{Institution: "Self study", Degree: "Certificate", Details: "Evenings"},
		},
	})
	if !strings.Contains(about, "2012 – 2016 · First class") {
		t.Fatal("about page missing dated education line")
	}
	if strings.Contains(about, "0 – 0") {
		t.Fatal("about page rendered blank years as 0 – 0")
	}
	if !strings.Contains(about, "Evenings") {
		t.Fatal("about page dropped details when years are blank")
	}
}

func TestParseOptionalYear(t *testing.T) {
	if y, ok := parseOptionalYear("  "); !ok || y != 0 {
		t.Fatalf("blank: %d %v", y, ok)
	}
	if y, ok := parseOptionalYear("2014"); !ok || y != 2014 {
		t.Fatalf("year: %d %v", y, ok)
	}
	if _, ok := parseOptionalYear("1899"); ok {
		t.Fatal("accepted a year before 1900")
	}
	if _, ok := parseOptionalYear("nope"); ok {
		t.Fatal("accepted a non-year")
	}
	edu := models.Education{Institution: "A", Degree: "B", StartYear: 2016, EndYear: 2012}
	if educationFormError(edu, true, true) == "" {
		t.Fatal("end before start was accepted")
	}
}
