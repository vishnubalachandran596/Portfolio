package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

const (
	chatbotSessionCookie = "cb_sid"
	chatbotDailyLimit    = 30
	chatbotTopK          = 5
	chatbotChunkMaxChars = 1000
	chatbotEmbedBatch    = 16
)

// chatbotSessionID identifies an anonymous chat widget session so the
// daily rate limit (chatbot_rate_limits, see migration 065) can be
// counted per browser rather than per logged-in account -- most visitors
// asking the chatbot something are signed-out.
func (a *App) chatbotSessionID(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(chatbotSessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	sid, err := authutil.RandomHex(16)
	if err != nil {
		sid = "fallback-session"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     chatbotSessionCookie,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.Cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(365 * 24 * time.Hour),
	})
	return sid
}

type chatRequestBody struct {
	Message string `json:"message"`
}

type chatSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type chatResponseBody struct {
	Reply   string       `json:"reply,omitempty"`
	Sources []chatSource `json:"sources,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// Chat answers one question from the portfolio's RAG chatbot: embed the
// question, retrieve the closest indexed chunks of site content, and ask
// NVIDIA's free-tier LLM to answer grounded in only that context. The
// widget that calls this stays hidden until a.Nvidia.Enabled().
func (a *App) Chat(w http.ResponseWriter, r *http.Request) {
	if !a.Nvidia.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, chatResponseBody{Error: "The chatbot isn't configured yet."})
		return
	}

	var req chatRequestBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, chatResponseBody{Error: "Invalid request."})
		return
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		writeJSON(w, http.StatusBadRequest, chatResponseBody{Error: "Ask me something about Vishnu, his courses, or his articles."})
		return
	}
	if len(message) > 800 {
		message = message[:800]
	}

	sid := a.chatbotSessionID(w, r)
	ctx := r.Context()
	allowed, err := a.ChatbotRateLimits.IncrementAndCheck(ctx, sid, chatbotDailyLimit)
	if err != nil {
		a.serverError(w, "chatbot rate limit", err)
		return
	}
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, chatResponseBody{Error: "You've reached today's question limit on the chatbot -- please try again tomorrow, or use the Contact page."})
		return
	}

	embeddings, err := a.Nvidia.Embed(ctx, []string{message}, "query")
	if err != nil || len(embeddings) == 0 {
		a.serverError(w, "chatbot embed question", err)
		return
	}

	chunks, err := a.ChatbotChunks.Search(ctx, embeddings[0], chatbotTopK)
	if err != nil {
		a.serverError(w, "chatbot search", err)
		return
	}

	var retrieved strings.Builder
	sources := make([]chatSource, 0, len(chunks))
	seen := make(map[string]bool, len(chunks))
	for _, c := range chunks {
		retrieved.WriteString("### " + c.Title + "\n" + c.ChunkText + "\n\n")
		if c.URL != "" && !seen[c.URL] {
			seen[c.URL] = true
			sources = append(sources, chatSource{Title: c.Title, URL: c.URL})
		}
	}

	systemPrompt := "You are the AI assistant embedded on Vishnu Balachandran's personal portfolio site. " +
		"Answer the visitor's question using ONLY the context below, which covers Vishnu's background, skills, " +
		"experience, courses, and articles. If the context doesn't contain the answer, say you don't have that " +
		"information and suggest the Contact page. Keep answers short (2-5 sentences), speak about Vishnu in " +
		"third person as his assistant, and never invent prices, dates, or facts not present in the context.\n\n" +
		"Context:\n" + retrieved.String()

	reply, err := a.Nvidia.Chat(ctx, systemPrompt, message)
	if err != nil {
		a.serverError(w, "chatbot completion", err)
		return
	}

	writeJSON(w, http.StatusOK, chatResponseBody{Reply: reply, Sources: sources})
}

// AdminChatbotPageData backs the "AI Chatbot" admin page: a chunk count
// and a "Rebuild Index" button.
type AdminChatbotPageData struct {
	BaseData
	Enabled    bool
	ChunkCount int
	Rebuilt    bool
}

func (a *App) AdminChatbotPage(w http.ResponseWriter, r *http.Request) {
	count, _ := a.ChatbotChunks.Count(r.Context())
	a.View.Render(w, http.StatusOK, "admin_chatbot", AdminChatbotPageData{
		BaseData:   a.base(r, "AI Chatbot — Admin", "", ""),
		Enabled:    a.Nvidia.Enabled(),
		ChunkCount: count,
		Rebuilt:    r.URL.Query().Get("rebuilt") == "1",
	})
}

// AdminChatbotReindex rebuilds the entire chatbot index from the site's
// current content (profile, skills, experience, education, courses,
// articles). It's a full replace rather than an incremental update --
// simpler and safer for a corpus this small, and it's the only way stale
// chunks for since-edited or deleted content get cleaned up.
func (a *App) AdminChatbotReindex(w http.ResponseWriter, r *http.Request) {
	if !a.Nvidia.Enabled() {
		http.Redirect(w, r, "/admin/chatbot", http.StatusSeeOther)
		return
	}
	ctx := r.Context()

	chunks := a.gatherChatbotChunks(ctx)

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.ChunkText
	}

	embeddings := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += chatbotEmbedBatch {
		end := min(start+chatbotEmbedBatch, len(texts))
		batch, err := a.Nvidia.Embed(ctx, texts[start:end], "passage")
		if err != nil {
			a.serverError(w, "chatbot reindex embed", err)
			return
		}
		embeddings = append(embeddings, batch...)
	}

	if err := a.ChatbotChunks.ReplaceAll(ctx, chunks, embeddings); err != nil {
		a.serverError(w, "chatbot reindex replace", err)
		return
	}

	http.Redirect(w, r, "/admin/chatbot?rebuilt=1", http.StatusSeeOther)
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// stripHTML reduces admin-authored lesson/article HTML to plain text for
// embedding -- the chatbot only ever needs the words, never the markup.
func stripHTML(s string) string {
	s = htmlTagPattern.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// chunkText splits plain text into pieces no longer than maxChars,
// breaking on paragraph/sentence boundaries where possible so each chunk
// stays a coherent, self-contained passage for retrieval.
func chunkText(s string, maxChars int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if len(s) <= maxChars {
		return []string{s}
	}
	var out []string
	for len(s) > maxChars {
		cut := strings.LastIndexAny(s[:maxChars], ".\n")
		if cut < maxChars/2 {
			cut = maxChars
		} else {
			cut++
		}
		piece := strings.TrimSpace(s[:cut])
		if piece != "" {
			out = append(out, piece)
		}
		s = strings.TrimSpace(s[cut:])
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

// gatherChatbotChunks reads every piece of indexable site content (never
// anything from the repo itself -- all of this is live DB/Storage
// content, per CLAUDE.md rule 8) and splits it into embeddable chunks.
// Best-effort throughout: a lookup failure for one content type just
// means that type is skipped this rebuild, not a failed reindex.
func (a *App) gatherChatbotChunks(ctx context.Context) []models.ChatbotChunk {
	var chunks []models.ChatbotChunk

	if profile, err := a.Profile.Get(ctx); err == nil {
		text := strings.Join(nonEmpty(profile.Name, profile.Title, profile.Tagline, profile.HeroSubtitle, profile.Summary), ". ")
		for _, piece := range chunkText(text, chatbotChunkMaxChars) {
			chunks = append(chunks, models.ChatbotChunk{SourceType: "profile", Title: "About " + profile.Name, URL: "/about", ChunkText: piece})
		}
	}

	if skills, err := a.Skills.List(ctx); err == nil {
		for _, g := range skills {
			text := g.Category + " (" + g.Level + "): " + strings.Join(g.Items, ", ") + ". " + g.Achievement
			chunks = append(chunks, models.ChatbotChunk{SourceType: "skill", SourceID: g.ID, Title: "Skill: " + g.Category, URL: "/about#skills", ChunkText: text})
		}
	}

	if exp, err := a.Experience.List(ctx); err == nil {
		for _, e := range exp {
			text := e.Role + " at " + e.Company + " (" + e.StartDate + " - " + e.EndDate + "), " + e.Location + ". " + strings.Join(e.Bullets, " ")
			chunks = append(chunks, models.ChatbotChunk{SourceType: "experience", SourceID: e.ID, Title: e.Role + " at " + e.Company, URL: "/about#experience", ChunkText: text})
		}
	}

	if edu, err := a.Education.List(ctx); err == nil {
		for _, e := range edu {
			text := e.Degree + " in " + e.Field + ", " + e.Institution + " (" + strconv.Itoa(e.StartYear) + "-" + strconv.Itoa(e.EndYear) + "). " + e.Details
			chunks = append(chunks, models.ChatbotChunk{SourceType: "education", SourceID: e.ID, Title: e.Degree + ", " + e.Institution, URL: "/about#education", ChunkText: text})
		}
	}

	if courses, err := a.Courses.ListPublished(ctx); err == nil {
		for _, c := range courses {
			url := "/courses/" + c.Slug
			text := c.Title + ". " + c.Summary + ". " + c.Description + ". Level: " + c.Level + ". Duration: " + c.Duration + ". Price: " + priceSummary(c)
			for _, piece := range chunkText(text, chatbotChunkMaxChars) {
				chunks = append(chunks, models.ChatbotChunk{SourceType: "course", SourceID: c.ID, Title: "Course: " + c.Title, URL: url, ChunkText: piece})
			}
			if c.ContentObject != "" {
				if body, err := a.readStorageText(ctx, c.ContentObject); err == nil {
					for _, piece := range chunkText(body, chatbotChunkMaxChars) {
						chunks = append(chunks, models.ChatbotChunk{SourceType: "course", SourceID: c.ID, Title: "Course lesson: " + c.Title, URL: url, ChunkText: piece})
					}
				}
			}
		}
	}

	if articles, err := a.Articles.ListPublished(ctx, 1000); err == nil {
		for _, art := range articles {
			url := "/articles/" + art.Slug
			text := art.Title + ". " + art.Summary
			body := art.Body
			if art.ContentObject != "" {
				if fetched, err := a.readStorageText(ctx, art.ContentObject); err == nil {
					body = fetched
				}
			}
			text += ". " + stripHTML(body)
			for _, piece := range chunkText(text, chatbotChunkMaxChars) {
				chunks = append(chunks, models.ChatbotChunk{SourceType: "article", SourceID: art.ID, Title: "Article: " + art.Title, URL: url, ChunkText: piece})
			}
		}
	}

	return chunks
}

func (a *App) readStorageText(ctx context.Context, path string) (string, error) {
	rc, err := a.CourseStorage.Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return stripHTML(string(body)), nil
}

func priceSummary(c models.Course) string {
	if c.IsFree() {
		return "free"
	}
	return c.PriceDisplay()
}

func nonEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}
