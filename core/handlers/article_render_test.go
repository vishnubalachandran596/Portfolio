package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestArticleTemplatesRenderStorageAndGrid(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatalf("view.New: %v", err)
	}
	base := BaseData{SiteName: "Vishnu Balachandran", Title: "Blog", IsUser: true}
	when := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	articles := []models.Article{{
		Slug:          "kafka-notes",
		Title:         "Kafka notes",
		Summary:       "A short hook.",
		Tags:          []string{"Kafka"},
		ThumbsUpCount: 2,
		PublishedDate: &when,
	}}

	rec := httptest.NewRecorder()
	r.Render(rec, 200, "articles_list", ArticlesData{
		BaseData:   base,
		Articles:   articles,
		Total:      1,
		Page:       1,
		TotalPages: 1,
	})
	body := rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatal(body)
	}
	if !strings.Contains(body, "article-grid") || !strings.Contains(body, "Kafka notes") {
		t.Fatal("article grid did not render")
	}

	rec = httptest.NewRecorder()
	r.Render(rec, 200, "admin_article_form", AdminArticleFormData{
		BaseData: base,
		Article: models.Article{
			ID:            4,
			Title:         "Kafka notes",
			Slug:          "kafka-notes",
			ContentObject: "articles/kafka-notes.html",
		},
	})
	body = rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatal(body)
	}
	if !strings.Contains(body, "articles/kafka-notes.html") || !strings.Contains(body, `name="content_file"`) {
		t.Fatal("admin article form missing storage upload")
	}
	if !strings.Contains(body, "multipart/form-data") {
		t.Fatal("article form is not multipart")
	}
}
