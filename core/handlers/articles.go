package handlers

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/storage"
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
	Subscribe  SubscribeFormData
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
		Subscribe:  SubscribeFormData{Source: "articles", Subscribed: q.Get("subscribed") == "1"},
	})
}

type ArticleDetailData struct {
	BaseData
	Article            models.Article
	Watermark          template.HTML
	Voted              bool
	JSONLD             template.JS
	PaymentsEnabled    bool
	ContributeCurrency string
	ContributeMinCents int
	ContributeMinWhole int
	Contributed        bool
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
	if err := a.loadArticleHTML(r.Context(), &article); err != nil {
		a.serverError(w, "load article html", err)
		return
	}
	base := a.baseFrom(r, pf, article.Title+" — "+a.Cfg.SiteName, article.Summary, "articles")
	currency := models.CurrencyINR
	a.View.Render(w, http.StatusOK, "article_detail", ArticleDetailData{
		BaseData:           base,
		Article:            article,
		Watermark:          template.HTML(storage.WatermarkScript(storage.WatermarkLabel, "")),
		Voted:              voted,
		JSONLD:             articleJSONLD(article, base.CanonicalURL, a.Cfg.SiteName),
		PaymentsEnabled:    a.Razorpay.Enabled(),
		ContributeCurrency: currency,
		ContributeMinCents: models.ArticleContributeMinCents,
		ContributeMinWhole: models.ArticleContributeMinCents / 100,
		Contributed:        r.URL.Query().Get("contributed") == "1",
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

	userID, err := a.guestUserForEmail(r, email, name)
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

type AdminArticleListRow struct {
	Article           models.Article
	PaidContributions int
	ContributionTotal string
}

type AdminArticlesListData struct {
	BaseData
	Articles []AdminArticleListRow
}

func (a *App) AdminArticlesList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		articles  []models.Article
		summaries map[int64]repo.ArticlePaidSummary
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { articles, err = a.Articles.ListAll(gctx); return })
	g.Go(func() (err error) { summaries, err = a.ArticleContributions.SummarizePaidByArticle(gctx); return })
	if err := g.Wait(); err != nil {
		a.serverError(w, "load articles", err)
		return
	}

	rows := make([]AdminArticleListRow, 0, len(articles))
	for _, art := range articles {
		row := AdminArticleListRow{Article: art}
		if s, ok := summaries[art.ID]; ok {
			row.PaidContributions = s.PaidCount
			row.ContributionTotal = models.FormatMoneyTotals(s.Totals)
		}
		rows = append(rows, row)
	}
	a.View.Render(w, http.StatusOK, "admin_articles_list", AdminArticlesListData{
		BaseData: a.base(r, "Articles — Admin", "", "admin"),
		Articles: rows,
	})
}

type adminArticleContribution struct {
	When      string
	Email     string
	UserID    int64
	Amount    string
	Status    string
	PaymentID string
}

type AdminArticleFormData struct {
	BaseData
	Article           models.Article
	IsNew             bool
	Error             string
	Votes             []models.ArticleVote
	Contributions     []adminArticleContribution
	PaidCount         int
	ContributionTotal string
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
	var (
		votes   []models.ArticleVote
		contrib []models.ArticleContribution
	)
	g, gctx := errgroup.WithContext(r.Context())
	g.Go(func() (err error) { votes, err = a.ArticleVotes.ListByArticle(gctx, article.ID); return })
	g.Go(func() (err error) { contrib, err = a.ArticleContributions.ListByArticle(gctx, article.ID); return })
	if err := g.Wait(); err != nil {
		a.serverError(w, "load article engagement", err)
		return
	}

	paidCount := 0
	totals := map[string]int{}
	rows := make([]adminArticleContribution, 0, len(contrib))
	for _, c := range contrib {
		when := c.CreatedAt.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM")
		if c.PaidAt != nil {
			when = c.PaidAt.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM")
		}
		if c.Status == models.ContributionStatusPaid {
			paidCount++
			totals[c.Currency] += c.AmountCents
		}
		rows = append(rows, adminArticleContribution{
			When:      when,
			Email:     c.Email,
			UserID:    c.UserID,
			Amount:    models.FormatMoney(c.AmountCents, c.Currency),
			Status:    c.Status,
			PaymentID: c.RazorpayPaymentID,
		})
	}

	a.View.Render(w, http.StatusOK, "admin_article_form", AdminArticleFormData{
		BaseData:          a.base(r, "Edit Article — Admin", "", "admin"),
		Article:           article,
		Votes:             votes,
		Contributions:     rows,
		PaidCount:         paidCount,
		ContributionTotal: models.FormatMoneyTotals(totals),
	})
}

// AdminArticleSave handles both create and edit. A typed body stays in
// Postgres. An uploaded HTML file goes to the private Storage bucket and
// the article page reads it back from there.
func (a *App) AdminArticleSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxCourseUploadBytes); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	var id int64
	isEdit := false
	if idStr := r.PathValue("id"); idStr != "" {
		var err error
		id, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			a.NotFound(w, r)
			return
		}
		isEdit = true
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

	var previous models.Article
	if isEdit {
		existing, err := a.Articles.GetByID(r.Context(), id)
		if err != nil {
			if err == repo.ErrNotFound {
				a.NotFound(w, r)
				return
			}
			a.serverError(w, "load article", err)
			return
		}
		previous = existing
		article.ContentObject = existing.ContentObject
	}

	var uploaded []byte
	if file, header, ferr := r.FormFile("content_file"); ferr == nil {
		defer file.Close()
		name := strings.ToLower(header.Filename)
		if !strings.HasSuffix(name, ".html") && !strings.HasSuffix(name, ".htm") {
			a.renderArticleForm(w, r, article, id == 0, "Article content must be an .html file.")
			return
		}
		b, rerr := io.ReadAll(io.LimitReader(file, maxCourseUploadBytes+1))
		if rerr != nil {
			a.serverError(w, "read uploaded article", rerr)
			return
		}
		if len(b) == 0 {
			a.renderArticleForm(w, r, article, id == 0, "The HTML file is empty.")
			return
		}
		if len(b) > maxCourseUploadBytes {
			a.renderArticleForm(w, r, article, id == 0, "Article content must be 8MB or smaller.")
			return
		}
		uploaded = b
		if a.CourseStorage == nil {
			a.renderArticleForm(w, r, article, id == 0, "Article storage is not configured on this server.")
			return
		}
		article.Body = ""
	}

	// A post is original writing hosted here. That writing is either the
	// typed body, a file just uploaded, or an HTML file already in Storage.
	if article.Title == "" || article.Summary == "" || (article.Body == "" && len(uploaded) == 0 && article.ContentObject == "") {
		a.renderArticleForm(w, r, article, id == 0, "Title, summary, and a body or HTML file are required — an article is original writing hosted on this site.")
		return
	}

	ctx := r.Context()
	dropPath := ""
	if len(uploaded) > 0 {
		path, perr := storage.ArticleObject(article.Slug)
		if perr != nil {
			a.serverError(w, "article object path", perr)
			return
		}
		if err := a.CourseStorage.Put(ctx, path, "text/html", uploaded); err != nil {
			a.serverError(w, "store article html", err)
			return
		}
		article.ContentObject = path
		if previous.ContentObject != "" && previous.ContentObject != path {
			dropPath = previous.ContentObject
		}
	} else if article.Body != "" && previous.ContentObject != "" {
		article.ContentObject = ""
		dropPath = previous.ContentObject
	} else if previous.ContentObject != "" {
		path, perr := storage.ArticleObject(article.Slug)
		if perr != nil {
			a.serverError(w, "article object path", perr)
			return
		}
		article.Body = ""
		if path != previous.ContentObject {
			raw, rerr := a.readArticleObject(ctx, previous.ContentObject)
			if rerr != nil {
				a.serverError(w, "read article html", rerr)
				return
			}
			if err := a.CourseStorage.Put(ctx, path, "text/html", raw); err != nil {
				a.serverError(w, "move article html", err)
				return
			}
			article.ContentObject = path
			dropPath = previous.ContentObject
		}
	}

	var err error
	if id == 0 {
		id, err = a.Articles.Create(ctx, article)
	} else {
		err = a.Articles.Update(ctx, article)
	}
	if err != nil {
		log.Printf("save article: %v", err)
		a.renderArticleForm(w, r, article, id == 0, "Could not save article — the URL slug may already be in use.")
		return
	}
	if dropPath != "" && a.CourseStorage != nil {
		if err := a.CourseStorage.Delete(ctx, dropPath); err != nil {
			log.Printf("delete article object %s: %v", dropPath, err)
		}
	}

	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}

func (a *App) renderArticleForm(w http.ResponseWriter, r *http.Request, article models.Article, isNew bool, msg string) {
	a.View.Render(w, http.StatusUnprocessableEntity, "admin_article_form", AdminArticleFormData{
		BaseData: a.base(r, "Article — Admin", "", "admin"),
		Article:  article,
		IsNew:    isNew,
		Error:    msg,
	})
}

// loadArticleHTML replaces Body with the Storage object when one is set.
// Posts that still live in the body column are left alone. Uploaded
// <style> is stripped so any HTML still follows the site typography.
func (a *App) loadArticleHTML(ctx context.Context, article *models.Article) error {
	if article.ContentObject == "" {
		return nil
	}
	raw, err := a.readArticleObject(ctx, article.ContentObject)
	if err != nil {
		return err
	}
	article.Body = storage.StripStyleTags(storage.ArticleHTML(raw))
	return nil
}

func (a *App) readArticleObject(ctx context.Context, path string) ([]byte, error) {
	if a.CourseStorage == nil {
		return nil, fmt.Errorf("article storage is not configured")
	}
	rc, err := a.CourseStorage.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxCourseUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxCourseUploadBytes {
		return nil, fmt.Errorf("article %s exceeds %d bytes", path, maxCourseUploadBytes)
	}
	return raw, nil
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
	if existing, gerr := a.Articles.GetByID(r.Context(), id); gerr == nil && existing.ContentObject != "" && a.CourseStorage != nil {
		if err := a.CourseStorage.Delete(r.Context(), existing.ContentObject); err != nil {
			log.Printf("delete article object %s: %v", existing.ContentObject, err)
		}
	}
	if err := a.Articles.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete article", err)
		return
	}
	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}
