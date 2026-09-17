package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

type ArticlesData struct {
	BaseData
	Articles []models.Article
}

// ArticlesPage lists curated articles — a link to the original plus a
// short in-house summary. Nothing here is re-hosted or scraped content;
// every entry credits and links its real source.
func (a *App) ArticlesPage(w http.ResponseWriter, r *http.Request) {
	articles, err := a.Articles.ListPublished(r.Context())
	if err != nil {
		a.serverError(w, "load articles", err)
		return
	}
	a.View.Render(w, http.StatusOK, "articles_list", ArticlesData{
		BaseData: a.base(r, "Articles — "+a.Cfg.SiteName, "Curated reading on architecture, AI, and engineering careers, with a short take and a link to the original.", "articles"),
		Articles: articles,
	})
}

// --- Admin -----------------------------------------------------------

type AdminArticlesListData struct {
	BaseData
	Articles []models.Article
}

func (a *App) AdminArticlesList(w http.ResponseWriter, r *http.Request) {
	articles, err := a.Articles.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load articles", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_articles_list", AdminArticlesListData{
		BaseData: a.base(r, "Articles — Admin", "", "admin"),
		Articles: articles,
	})
}

type AdminArticleFormData struct {
	BaseData
	Article models.Article
	IsNew   bool
	Error   string
}

func (a *App) AdminArticleNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_article_form", AdminArticleFormData{
		BaseData: a.base(r, "New Article — Admin", "", "admin"),
		IsNew:    true,
		Article:  models.Article{IsPublished: true},
	})
}

func (a *App) AdminArticleEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	article, err := a.Articles.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load article", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_article_form", AdminArticleFormData{
		BaseData: a.base(r, "Edit Article — Admin", "", "admin"),
		Article:  article,
	})
}

// AdminArticleSave handles both create and edit: paste a link, write a
// short summary, done — that's the whole "add an article" flow.
func (a *App) AdminArticleSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	var id int64
	if idStr := r.PathValue("id"); idStr != "" {
		var err error
		id, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			a.NotFound(w, r)
			return
		}
	}

	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	article := models.Article{
		ID:          id,
		Title:       strings.TrimSpace(r.FormValue("title")),
		Summary:     strings.TrimSpace(r.FormValue("summary")),
		SourceURL:   strings.TrimSpace(r.FormValue("source_url")),
		SourceName:  strings.TrimSpace(r.FormValue("source_name")),
		IsPublished: r.FormValue("is_published") == "on",
		SortOrder:   sortOrder,
	}
	if dateStr := strings.TrimSpace(r.FormValue("published_date")); dateStr != "" {
		if d, err := time.Parse("2006-01-02", dateStr); err == nil {
			article.PublishedDate = &d
		}
	}

	if article.Title == "" || article.SourceURL == "" || article.Summary == "" {
		formData := AdminArticleFormData{
			BaseData: a.base(r, "Article — Admin", "", "admin"),
			Article:  article,
			IsNew:    id == 0,
			Error:    "Title, source link, and summary are all required.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_article_form", formData)
		return
	}
	if !strings.HasPrefix(article.SourceURL, "http://") && !strings.HasPrefix(article.SourceURL, "https://") {
		article.SourceURL = "https://" + article.SourceURL
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.Articles.Create(ctx, article)
	} else {
		err = a.Articles.Update(ctx, article)
	}
	if err != nil {
		a.serverError(w, "save article", err)
		return
	}

	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}

func (a *App) AdminArticleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Articles.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete article", err)
		return
	}
	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}
