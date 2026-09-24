package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestAboutRendersRoleResumeSelect(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatalf("view.New: %v", err)
	}
	base := BaseData{SiteName: "Vishnu Balachandran", Title: "About", Nav: NavLinks{Consultancy: "/consultancy"}}

	rec := httptest.NewRecorder()
	r.Render(rec, 200, "about", AboutData{
		BaseData: base,
		Profile:  models.Profile{Name: "Vishnu Balachandran", Title: "Technical Consultant"},
		Resumes: []models.RoleResume{{
			RoleKey:    "zafin-lead-product-engineer-i",
			Label:      "Lead Product Engineer I · Zafin",
			ObjectPath: "resumes/zafin-lead-product-engineer-i.pdf",
			SortOrder:  1,
		}},
	})
	body := rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatal(body)
	}
	if !strings.Contains(body, `href="/resume.pdf"`) {
		t.Fatal("default download link missing")
	}
	if !strings.Contains(body, `action="/resume.pdf"`) || !strings.Contains(body, `name="role"`) {
		t.Fatal("about page missing role resume select")
	}
	if !strings.Contains(body, `value="zafin-lead-product-engineer-i"`) || !strings.Contains(body, "Lead Product Engineer I · Zafin") {
		t.Fatal("role option missing")
	}

	rec = httptest.NewRecorder()
	r.Render(rec, 200, "about", AboutData{
		BaseData: base,
		Profile:  models.Profile{Name: "Vishnu Balachandran", Title: "Technical Consultant"},
	})
	body = rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatal(body)
	}
	if !strings.Contains(body, `href="/resume.pdf"`) {
		t.Fatal("empty resume list should keep the plain download link")
	}
	if strings.Contains(body, `name="role"`) {
		t.Fatal("empty resume list should not render a role select")
	}

	rec = httptest.NewRecorder()
	r.Render(rec, 200, "admin_experience_form", AdminExperienceFormData{
		BaseData: base,
		IsNew:    true,
		Experience: models.Experience{
			Company: "Zafin",
			Role:    "Lead Product Engineer I",
		},
	})
	body = rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatal(body)
	}
	if !strings.Contains(body, "multipart/form-data") || !strings.Contains(body, `name="resume_file"`) {
		t.Fatal("experience form missing resume upload")
	}
}

func TestResumeFileName(t *testing.T) {
	if got := resumeFileName(""); got != "Resume_Vishnu_balachandran.pdf" {
		t.Fatalf("default name %s", got)
	}
	if got := resumeFileName(models.DefaultResumeKey); got != "Resume_Vishnu_balachandran.pdf" {
		t.Fatalf("default key name %s", got)
	}
	got := resumeFileName("zafin-lead-product-engineer-i")
	if got != "Resume_Vishnu_balachandran_zafin-lead-product-engineer-i.pdf" {
		t.Fatalf("role name %s", got)
	}
}

func TestRoleSpecificResumesOmitsDefault(t *testing.T) {
	got := roleSpecificResumes([]models.RoleResume{
		{RoleKey: models.DefaultResumeKey, ObjectPath: "resumes/default.pdf"},
		{RoleKey: "zafin-lead-product-engineer-i", ObjectPath: "resumes/zafin-lead-product-engineer-i.pdf"},
		{RoleKey: "way-technical-lead"},
	})
	if len(got) != 1 || got[0].RoleKey != "zafin-lead-product-engineer-i" {
		t.Fatalf("got %+v", got)
	}
	if roleSpecificResumes(nil) != nil {
		t.Fatal("no role resumes should stay empty")
	}
}
