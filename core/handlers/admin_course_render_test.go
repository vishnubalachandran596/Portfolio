package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestAdminCourseFormShowsStoragePath(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatalf("view.New: %v", err)
	}
	rec := httptest.NewRecorder()
	r.Render(rec, 200, "admin_course_form", AdminCourseFormData{
		BaseData: BaseData{SiteName: "Vishnu Balachandran", Title: "Edit Course — Admin"},
		Course: models.Course{
			ID:            3,
			Title:         "Java Fundamentals",
			Slug:          "java-fundamentals",
			ContentObject: "courses/java-fundamentals.html",
		},
	})
	body := rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatal(body)
	}
	if !strings.Contains(body, "courses/java-fundamentals.html") {
		t.Fatal("storage path not shown")
	}
	if !strings.Contains(body, `href="/admin/courses/3/preview"`) {
		t.Fatal("admin course form missing preview link")
	}
	if strings.Contains(body, "ContentHTML") {
		t.Fatal("template still mentions ContentHTML")
	}

	rec = httptest.NewRecorder()
	r.Render(rec, 200, "admin_courses_list", AdminCoursesListData{
		BaseData: BaseData{SiteName: "Vishnu Balachandran", Title: "Courses — Admin"},
		Courses: []models.Course{
			{ID: 3, Title: "Java Fundamentals", ContentObject: "courses/java-fundamentals.html"},
			{ID: 4, Title: "Draft with no file"},
		},
	})
	body = rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatal(body)
	}
	if !strings.Contains(body, `href="/admin/courses/3/preview"`) {
		t.Fatal("course list missing preview for a stored lesson")
	}
	if strings.Contains(body, "/admin/courses/4/preview") {
		t.Fatal("course without a file should not offer preview")
	}
}
