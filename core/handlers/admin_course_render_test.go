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
	if strings.Contains(body, "ContentHTML") {
		t.Fatal("template still mentions ContentHTML")
	}
}
