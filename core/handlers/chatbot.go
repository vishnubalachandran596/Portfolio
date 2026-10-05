package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
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

	// chatbotRelevanceThreshold is the cosine-distance cutoff (0 =
	// identical, 2 = opposite) the top retrieved chunk must beat for a
	// question to count as "about Vishnu, his courses, or his articles."
	// This is the intent classifier: it costs nothing beyond the
	// embedding call already made for retrieval (no extra request, no
	// extra tokens), and it runs *before* the expensive, context-stuffed
	// generation call -- an off-topic question never reaches the LLM at
	// all, which is the whole point: cut token spend and keep the bot on
	// its one job instead of answering general-knowledge questions. A
	// starting value tuned by hand, not measured against a labeled set --
	// revisit if real traffic shows it too strict (relevant questions
	// getting declined) or too loose (off-topic ones getting answered).
	chatbotRelevanceThreshold = 0.75
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

// unlockedCourseIDs returns the paid course IDs the asker may have a
// gated lesson chunk answered from -- the chatbot applies the same rule
// /courses/{slug}/learn does. A signed-in admin not previewing as a
// visitor gets every course, mirroring courseAccess()'s own admin-preview
// bypass (so the site owner can ask the bot about any locked lesson
// while testing, exactly as they can already open it directly). Everyone
// else gets only the courses Purchases says they've actually paid for; a
// signed-out visitor (most chat traffic) gets an empty, non-nil slice.
func (a *App) unlockedCourseIDs(ctx context.Context, r *http.Request) []int64 {
	if a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r) {
		if courses, err := a.Courses.ListAll(ctx); err == nil {
			ids := make([]int64, len(courses))
			for i, c := range courses {
				ids[i] = c.ID
			}
			return ids
		}
	}

	uid, ok := a.Auth.UserID(r)
	if !ok {
		return []int64{}
	}
	purchases, err := a.Purchases.ListPaidByUserID(ctx, uid)
	if err != nil {
		return []int64{}
	}
	ids := make([]int64, len(purchases))
	for i, p := range purchases {
		ids[i] = p.CourseID
	}
	return ids
}

// chatServerError logs the real failure (same format a.serverError uses)
// but responds with the widget's own JSON shape rather than a.serverError's
// plain-text body -- the client always calls response.json() on every
// /chat response, success or failure, so a plain-text 500 fails that
// parse and surfaces only a generic "something went wrong reaching the
// chatbot" with the real cause visible nowhere but the server log. This
// was the actual bug behind a NVIDIA model retirement (410 Gone) showing
// up to visitors as an unexplained failure instead of a clear message.
func chatServerError(w http.ResponseWriter, action string, err error) {
	log.Printf("%s: %v", action, err)
	writeJSON(w, http.StatusInternalServerError, chatResponseBody{Error: "The chatbot hit a problem answering that. Please try again in a moment."})
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
		chatServerError(w, "chatbot rate limit", err)
		return
	}
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, chatResponseBody{Error: "You've reached today's question limit on the chatbot -- please try again tomorrow, or use the Contact page."})
		return
	}

	unlockedCourseIDs := a.unlockedCourseIDs(ctx, r)

	embeddings, err := a.Nvidia.Embed(ctx, []string{message}, "query")
	if err != nil || len(embeddings) == 0 {
		chatServerError(w, "chatbot embed question", err)
		return
	}

	chunks, err := a.ChatbotChunks.Search(ctx, embeddings[0], chatbotTopK, unlockedCourseIDs)
	if err != nil {
		chatServerError(w, "chatbot search", err)
		return
	}

	// Intent gate: if even the closest indexed chunk isn't a good match
	// for this question, nothing relevant to retrieve means nothing
	// grounded to answer with -- decline without spending a generation
	// call's tokens, rather than letting the LLM improvise from its own
	// training data (which is exactly the hallucination risk RAG exists
	// to avoid).
	if len(chunks) == 0 || chunks[0].Distance > chatbotRelevanceThreshold {
		writeJSON(w, http.StatusOK, chatResponseBody{
			Reply: "I can only answer questions about Vishnu's background, skills, experience, courses, and articles. Try asking something more specific, or use the Contact page for anything else.",
		})
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
		"experience, courses, and articles. The context already excludes any paid course's lesson content the " +
		"visitor hasn't unlocked -- if asked for specifics of a locked lesson, say that part is only available " +
		"after purchasing the course, rather than guessing at what it might contain. If the context doesn't contain the answer, say you don't have that " +
		"information and suggest the Contact page. Keep answers short (2-5 sentences), speak about Vishnu in " +
		"third person as his assistant, and never invent prices, dates, or facts not present in the context.\n\n" +
		"Context:\n" + retrieved.String()

	reply, err := a.Nvidia.Chat(ctx, systemPrompt, message)
	if err != nil {
		chatServerError(w, "chatbot completion", err)
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
			// RagContext, when an admin has written one, is a passage
			// authored specifically to read well out of context for the
			// LLM -- appended rather than replacing the derived facts
			// above (level/duration/price), which stay precise and can't
			// drift the way a paraphrase could.
			if c.RagContext != "" {
				text += ". " + c.RagContext
			}
			for _, piece := range chunkText(text, chatbotChunkMaxChars) {
				chunks = append(chunks, models.ChatbotChunk{SourceType: "course", SourceID: c.ID, Title: "Course: " + c.Title, URL: url, ChunkText: piece})
			}
			if c.ContentObject != "" {
				if body, err := a.readStorageText(ctx, c.ContentObject); err == nil {
					// A paid course's lesson body is the exact content
					// /courses/{slug}/learn paywalls -- gate it the same
					// way here, or the chatbot becomes a free side-channel
					// around that paywall. A free course has nothing to
					// gate: its lesson is already open to everyone.
					for _, piece := range chunkText(body, chatbotChunkMaxChars) {
						chunks = append(chunks, models.ChatbotChunk{SourceType: "course", SourceID: c.ID, Title: "Course lesson: " + c.Title, URL: url, ChunkText: piece, IsGated: c.IsPaid})
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
