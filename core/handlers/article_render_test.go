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
	if !strings.Contains(body, `href="/admin/articles/4/preview"`) {
		t.Fatal("admin article form missing preview link")
	}
	if !strings.Contains(body, "multipart/form-data") {
		t.Fatal("article form is not multipart")
	}

	rec = httptest.NewRecorder()
	r.Render(rec, 200, "admin_articles_list", AdminArticlesListData{
		BaseData: base,
		Articles: []models.Article{{ID: 4, Slug: "kafka-notes", Title: "Kafka notes", ContentObject: "articles/kafka-notes.html"}},
	})
	body = rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatal(body)
	}
	if !strings.Contains(body, `href="/admin/articles/4/preview"`) {
		t.Fatal("admin article list missing preview link")
	}
}

func TestCourseLearnTemplateUsesSiteChrome(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatalf("view.New: %v", err)
	}
	rec := httptest.NewRecorder()
	r.Render(rec, 200, "course_learn", CourseLearnData{
		BaseData: BaseData{
			SiteName: "Vishnu Balachandran", Title: "Java Fundamentals — Lesson",
			Active: "courses", AssetVersion: "test",
			Nav: NavLinks{Home: "/", Courses: "/courses", Articles: "/articles"},
		},
		Course:        models.Course{Slug: "java-fundamentals", Title: "Java Fundamentals"},
		HasQuiz:       true,
		LessonStyles:  ".lesson-embed{color:red}",
		LessonPayload: "ZmFrZQ==",
	})
	body := rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatal(body)
	}
	if !strings.Contains(body, `id="lesson-root"`) || !strings.Contains(body, "class=\"lesson-embed\"") {
		t.Fatal("lesson embed missing")
	}
	if !strings.Contains(body, "ZmFrZQ==") {
		t.Fatal("payload missing")
	}
	if !strings.Contains(body, `href="/courses/java-fundamentals/quiz"`) {
		t.Fatal("quiz link missing")
	}
	if !strings.Contains(body, `class="site-header"`) && !strings.Contains(body, "<header") {
		t.Fatal("site chrome missing")
	}
}
