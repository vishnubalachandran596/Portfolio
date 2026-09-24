package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

const articlesPageSize = 5

type ArticlesData struct {
	BaseData
	Articles   []models.Article
	Tags       []models.ArticleTag
	Query      string
	ActiveTag  string
	Sort       string
	Page       int
	TotalPages int
	Total      int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
}

// ArticlesPage lists Vishnu's own blog posts — every article is original
// writing hosted here, with its own body and its own permanent URL. (The
// site once also carried curated link-outs to pieces published elsewhere;
// that feature was removed in migration 040.) Supports search, a tag
// filter, sort by publish date or thumbs-up count, and pagination at 5
// per page.
func (a *App) ArticlesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	sort := q.Get("sort")
	if sort != "date_asc" && sort != "thumbs_desc" {
		sort = "date_desc"
	}

	opt := repo.ArticleListOptions{
		Query:    strings.TrimSpace(q.Get("q")),
		Tag:      strings.TrimSpace(q.Get("tag")),
		Sort:     sort,
		Page:     page,
		PageSize: articlesPageSize,
	}
	// The filtered page of articles and the full topic list are
	// independent queries and both have to succeed, so they go out
	// concurrently under an errgroup rather than as a serial round-trip
	// chain on a public page — see docs/ARCHITECTURE.md §9. The chrome's
	// own lookups join the same wave via the prefetch.
	var (
		result repo.ArticleListResult
		tags   []models.ArticleTag
	)
	pf := a.startBase(r)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { result, err = a.Articles.ListPublishedFiltered(gctx, opt); return })
	g.Go(func() (err error) { tags, err = a.Articles.ListAllTags(gctx); return })
	if err := g.Wait(); err != nil {
		a.serverError(w, "load articles page data", err)
		return
	}

	totalPages := (result.Total + articlesPageSize - 1) / articlesPageSize
	if totalPages < 1 {
		totalPages = 1
	}

	a.View.Render(w, http.StatusOK, "articles_list", ArticlesData{
		BaseData:   a.baseFrom(r, pf, "Blog — "+a.Cfg.SiteName, "Original writing on architecture, event-driven systems, and engineering careers, plus a short take on other pieces worth your time.", "articles"),
		Articles:   result.Articles,
		Tags:       tags,
		Query:      opt.Query,
		ActiveTag:  opt.Tag,
		Sort:       sort,
		Page:       page,
		TotalPages: totalPages,
		Total:      result.Total,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
		PrevPage:   page - 1,
		NextPage:   page + 1,
	})
}

type ArticleDetailData struct {
	BaseData
	Article models.Article
	Voted   bool
	JSONLD  template.JS
}

// ArticleDetailPage is the article's own internal page — summary, tags,
// publish date, and thumbs-up count, plus a link out to the real source.
// Having a real page per article (rather than only an outbound card) is
// what lets articles appear in the sitemap and actually get indexed.
func (a *App) ArticleDetailPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	pf := a.startBase(r)
	article, err := a.Articles.GetBySlug(r.Context(), slug)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load article", err)
		return
	}

	// Needs the article's id, so this is unavoidably a second round trip —
	// but only for a signed-in visitor.
	var voted bool
	if userID, ok := a.Auth.UserID(r); ok {
		voted, err = a.ArticleVotes.HasVoted(r.Context(), article.ID, userID)
		if err != nil {
			a.serverError(w, "check article vote", err)
			return
		}
	}
	base := a.baseFrom(r, pf, article.Title+" — "+a.Cfg.SiteName, article.Summary, "articles")
	a.View.Render(w, http.StatusOK, "article_detail", ArticleDetailData{
		BaseData: base,
		Article:  article,
		Voted:    voted,
		JSONLD:   articleJSONLD(article, base.CanonicalURL, a.Cfg.SiteName),
	})
}

// ArticleThumbsUp records one thumbs-up. A logged-in visitor votes with
// their real account. A signed-out visitor gets an optional name/email
// form instead of a login wall: giving an email reuses (or creates) a
// guest account for that email — email is the unique identity, so a
// second vote from the same email de-dupes exactly like a real login
// does, via the same UNIQUE(article_id, user_id) constraint. A name with
// no email (or nothing at all) has no stable identity, so it's stored
// directly on the vote row and never de-dupes — every submission counts.
func (a *App) ArticleThumbsUp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	article, err := a.Articles.GetBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Article not found."})
		return
	}

	if userID, ok := a.Auth.UserID(r); ok {
		a.recordArticleVote(w, r, article, userID)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	name := strings.TrimSpace(r.FormValue("name"))

	if email == "" {
		if err := a.ArticleVotes.CreateGuest(r.Context(), article.ID, name); err != nil {
			a.serverError(w, "record guest article vote", err)
			return
		}
		count, err := a.Articles.IncrementThumbsUp(r.Context(), slug)
		if err != nil {
			a.serverError(w, "increment thumbs up", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"thumbs_up_count": count})
		return
	}

	randomPassword, err := authutil.RandomHex(32)
	if err != nil {
		a.serverError(w, "provision guest voter", err)
		return
	}
	hash, err := authutil.HashPassword(randomPassword)
	if err != nil {
		a.serverError(w, "provision guest voter", err)
		return
	}
	userID, err := a.Users.GetOrCreateGuest(r.Context(), email, hash, name)
	if err != nil {
		a.serverError(w, "provision guest voter", err)
		return
	}
	a.recordArticleVote(w, r, article, userID)
}

func (a *App) recordArticleVote(w http.ResponseWriter, r *http.Request, article models.Article, userID int64) {
	if err := a.ArticleVotes.Create(r.Context(), article.ID, userID); err != nil {
		if err == repo.ErrAlreadyVoted {
			writeJSON(w, http.StatusOK, map[string]any{"already_voted": true, "thumbs_up_count": article.ThumbsUpCount})
			return
		}
		a.serverError(w, "record article vote", err)
		return
	}

	count, err := a.Articles.IncrementThumbsUp(r.Context(), article.Slug)
	if err != nil {
		a.serverError(w, "increment thumbs up", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thumbs_up_count": count})
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
	Votes   []models.ArticleVote
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
	votes, err := a.ArticleVotes.ListByArticle(r.Context(), article.ID)
	if err != nil {
		a.serverError(w, "load article votes", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_article_form", AdminArticleFormData{
		BaseData: a.base(r, "Edit Article — Admin", "", "admin"),
		Article:  article,
		Votes:    votes,
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
		ID:            id,
		Slug:          slugify(r.FormValue("slug")),
		Title:         strings.TrimSpace(r.FormValue("title")),
		Summary:       strings.TrimSpace(r.FormValue("summary")),
		Body:          strings.TrimSpace(r.FormValue("body")),
		CoverImageURL: strings.TrimSpace(r.FormValue("cover_image_url")),
		Tags:          parseTags(r.FormValue("tags")),
		IsPublished:   r.FormValue("is_published") == "on",
		SortOrder:     sortOrder,
	}
	if article.Slug == "" {
		article.Slug = slugify(article.Title)
	}
	if dateStr := strings.TrimSpace(r.FormValue("published_date")); dateStr != "" {
		if d, err := time.Parse("2006-01-02", dateStr); err == nil {
			article.PublishedDate = &d
		}
	}

	// An article is original writing hosted here, so a body is required —
	// there is no longer a second, link-only kind of article that could
	// stand in for one (see migration 040).
	if article.Title == "" || article.Summary == "" || article.Body == "" {
		formData := AdminArticleFormData{
			BaseData: a.base(r, "Article — Admin", "", "admin"),
			Article:  article,
			IsNew:    id == 0,
			Error:    "Title, summary, and a body are all required — an article is original writing hosted on this site.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_article_form", formData)
		return
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.Articles.Create(ctx, article)
	} else {
		err = a.Articles.Update(ctx, article)
	}
	if err != nil {
		formData := AdminArticleFormData{
			BaseData: a.base(r, "Article — Admin", "", "admin"),
			Article:  article,
			IsNew:    id == 0,
			Error:    "Could not save article — the URL slug may already be in use.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_article_form", formData)
		return
	}

	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}

// parseTags splits a comma-separated tags input into a trimmed, non-empty
// slice, e.g. "AI, System Design" -> ["AI", "System Design"].
func parseTags(raw string) []string {
	var out []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
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
