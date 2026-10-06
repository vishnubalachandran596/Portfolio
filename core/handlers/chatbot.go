package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vishnubalachandran596/portfolio/core/ai"
	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

const (
	chatbotSessionCookie = "cb_sid"
	chatbotDailyLimit    = 30
	chatbotTopK          = 5
	chatbotChunkMaxChars = 1000
	// chatbotEmbedBatch is how many chunks go into one NVIDIA embeddings
	// call. RebuildChatbotIndex runs synchronously within an HTTP
	// request (see its own comment for why), so fewer, larger batches
	// directly shrink how much of vercel.json's maxDuration a reindex
	// spends on network round trips rather than actual work.
	chatbotEmbedBatch = 32
	// chatbotMaxBodyChars caps how much of one course lesson's or
	// article's plain-text body gets embedded, applied before chunking.
	// Discovered necessary empirically: one course's lesson HTML alone
	// is over 800KB (the rest average 30-135KB), and with 9 published
	// courses and 12 articles, embedding every chunk of every source in
	// full pushed a single RebuildChatbotIndex well past 60 seconds and
	// into a FUNCTION_INVOCATION_TIMEOUT -- which, unlike the "fire-and
	// -forget never completes" bug this replaced, would have made every
	// single course/article save fail outright, since a save always
	// triggers a full reindex. 15 chunks (~15,000 characters) is still
	// far more than enough grounding for a RAG answer about any one
	// course or article; the chatbot doesn't need the whole lesson,
	// just enough of it to answer "what does this course cover."
	chatbotMaxBodyChars = chatbotChunkMaxChars * 15

	// chatbotMaxHistoryTurns caps how many prior turns of a conversation
	// are kept and resent -- both to the LLM (prompt size/cost) and for
	// retrieval query reformation below. 6 turns is 3 exchanges, enough
	// for a follow-up like "as lead?" to still resolve against what was
	// being discussed without letting a long conversation's prompt grow
	// unbounded.
	chatbotMaxHistoryTurns = 6
	// chatbotMaxHistoryChars caps each history turn's content once it's
	// re-sent as context -- a long prior reply re-included verbatim on
	// every subsequent turn would otherwise make the prompt grow with
	// every exchange, not just linearly with turn count.
	chatbotMaxHistoryChars = 500

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

// chatHistoryTurn is one prior exchange's turn as the widget sends it
// back on every subsequent question -- the server holds no session-level
// chat history itself (stateless serverless, CLAUDE.md §3), so the
// client is the only place a conversation's turns live between requests.
type chatHistoryTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequestBody struct {
	Message string            `json:"message"`
	History []chatHistoryTurn `json:"history"`
}

// sanitizeHistory turns the client-supplied history into validated
// ai.Message turns: only role "user"/"assistant" survive (never "system"
// -- a crafted request could otherwise try to inject one), each turn is
// trimmed to chatbotMaxHistoryChars, and only the most recent
// chatbotMaxHistoryTurns are kept. Defense in depth -- the widget itself
// never sends anything else -- but this is the boundary where an
// untrusted request becomes what gets sent to the LLM.
func sanitizeHistory(raw []chatHistoryTurn) []ai.Message {
	var out []ai.Message
	for _, t := range raw {
		role := strings.TrimSpace(t.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := truncateChars(strings.TrimSpace(t.Content), chatbotMaxHistoryChars)
		if content == "" {
			continue
		}
		out = append(out, ai.Message{Role: role, Content: content})
	}
	if len(out) > chatbotMaxHistoryTurns {
		out = out[len(out)-chatbotMaxHistoryTurns:]
	}
	return out
}

// historySearchQuery folds the most recent user turn (if any) into the
// current message before embedding it for retrieval -- a short follow-up
// like "as lead?" embeds to a near-meaningless vector on its own, so
// without this the relevance gate and the chunk search would both lose
// the thread the moment a question doesn't stand alone. Only the last
// user turn is used, not assistant replies (already-generated text could
// drift from what the chunks actually say) and not the whole history
// (would dilute the embedding rather than sharpen it).
func historySearchQuery(history []ai.Message, message string) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return history[i].Content + " " + message
		}
	}
	return message
}

// AIAssistantData backs ai_assistant.html -- nothing beyond BaseData,
// since the chat itself (chatbot_widget.html) reads only ChatbotEnabled/
// ChatbotFullPage off that.
type AIAssistantData struct {
	BaseData
}

// AIAssistantPage renders the chatbot's full-page, own-URL counterpart to
// the floating bubble every other page shows: same chatbot_widget.html
// partial, same /chat endpoint and main.js wiring, just laid out as the
// whole page instead of a corner panel (see chatbot_widget.html and
// BaseData.ChatbotFullPage). Its own URL is what lets it appear in
// sitemap.xml and get indexed -- the floating bubble can't, since it's
// JS-rendered chrome repeated on every page, not a distinct page of its
// own. "AI Assistant" (title, nav-adjacent copy, URL) is a deliberately
// generic, commonly-searched term rather than site-specific branding, so
// this page can also pick up generic "AI assistant"-type search traffic
// alongside the branded searches the rest of the site targets.
func (a *App) AIAssistantPage(w http.ResponseWriter, r *http.Request) {
	base := a.base(r, "AI Assistant — Ask About "+a.Cfg.SiteName,
		"A free AI chatbot and virtual assistant answering real questions about Vishnu Balachandran's software engineering experience, technical consultancy, and courses — including topics covered in any course you've purchased — with instant, retrieval-augmented answers grounded in this site's real content.",
		"ai-assistant")
	base.ChatbotFullPage = true
	a.View.Render(w, http.StatusOK, "ai_assistant", AIAssistantData{BaseData: base})
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

	// 16KB, not 4KB: a full chatbotMaxHistoryTurns-turn history at
	// chatbotMaxHistoryChars each is already ~3KB before JSON overhead,
	// plus the message itself -- sanitizeHistory trims/caps the content
	// that actually gets used, this just has to be large enough to
	// decode the raw request first.
	var req chatRequestBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&req); err != nil {
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
	history := sanitizeHistory(req.History)

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

	// The embedded search query folds in the last user turn so a
	// standalone-looking follow-up ("as lead?") still retrieves against
	// what was actually being discussed -- see historySearchQuery's own
	// comment. The generation call below still gets the literal message.
	embeddings, err := a.Nvidia.Embed(ctx, []string{historySearchQuery(history, message)}, "query")
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
		"This is a continuing conversation -- prior turns are included below the context; use them to resolve " +
		"short follow-up questions, pronouns, and references like \"that\" or \"it\" to whatever was actually " +
		"being discussed, rather than treating the latest message as standalone. " +
		"Answer the visitor's question using ONLY the context below, which covers Vishnu's background, skills, " +
		"experience, courses, and articles. The context already excludes any paid course's lesson content the " +
		"visitor hasn't unlocked -- if asked for specifics of a locked lesson, say that part is only available " +
		"after purchasing the course, rather than guessing at what it might contain. If the context doesn't contain the answer, say you don't have that " +
		"information and suggest the Contact page. Keep answers short (2-5 sentences), speak about Vishnu in " +
		"third person as his assistant, and never invent prices, dates, or facts not present in the context.\n\n" +
		"Context:\n" + retrieved.String()

	reply, err := a.Nvidia.Chat(ctx, systemPrompt, history, message)
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

// AdminChatbotReindex is the manual "Rebuild Index" button on
// /admin/chatbot. Runs synchronously (see RebuildChatbotIndex's comment
// for why): this exists for a first build, recovery after an
// embedding-model swap (a new model needs the whole corpus re-embedded,
// not just what changed), or whenever an admin wants to force one.
func (a *App) AdminChatbotReindex(w http.ResponseWriter, r *http.Request) {
	if !a.Nvidia.Enabled() {
		http.Redirect(w, r, "/admin/chatbot", http.StatusSeeOther)
		return
	}
	if err := a.RebuildChatbotIndex(r.Context()); err != nil {
		a.serverError(w, "chatbot reindex", err)
		return
	}
	http.Redirect(w, r, "/admin/chatbot?rebuilt=1", http.StatusSeeOther)
}

// APIChatbotReindex is a bearer-token-gated version of the same trigger,
// for a coding agent (not a logged-in admin) that has just edited course
// or article content directly -- a new/edited/deleted row in Postgres, or
// a new/replaced/removed HTML file in Supabase Storage -- bypassing the
// admin UI's own save/delete handlers, which already call
// RebuildChatbotIndex themselves. Disabled (404) until
// CHATBOT_REINDEX_TOKEN is configured, and the token must match exactly;
// POST-only. Runs synchronously and reports the resulting chunk count,
// rather than responding before the work is done -- see
// RebuildChatbotIndex's comment for why a "started, ask again later"
// response would be a lie on this platform.
func (a *App) APIChatbotReindex(w http.ResponseWriter, r *http.Request) {
	token := a.Cfg.ChatbotReindexToken
	if token == "" {
		http.NotFound(w, r)
		return
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		writeJSON(w, http.StatusUnauthorized, chatResponseBody{Error: "invalid or missing bearer token"})
		return
	}
	if !a.Nvidia.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, chatResponseBody{Error: "chatbot isn't configured (NVIDIA_API_KEY unset)"})
		return
	}
	if err := a.RebuildChatbotIndex(r.Context()); err != nil {
		chatServerError(w, "api chatbot reindex", err)
		return
	}
	count, _ := a.ChatbotChunks.Count(r.Context())
	writeJSON(w, http.StatusOK, chatResponseBody{Reply: fmt.Sprintf("reindex complete: %d chunks", count)})
}

// RebuildChatbotIndex rebuilds the entire chatbot index from the site's
// current content (profile, skills, experience, education, courses,
// articles). It's a full replace rather than an incremental update --
// simpler and safer for a corpus this small, and it's the only way stale
// chunks for since-edited or deleted content get cleaned up (a deleted
// course just stops being gathered, so its chunks don't come back).
//
// Every caller runs this synchronously, bounded by its own request
// context (and vercel.json's maxDuration) -- an earlier version fired it
// in a detached goroutine instead ("fire-and-forget," so a save/delete
// wouldn't have to wait on a full re-embed). That never actually ran to
// completion in production: confirmed empirically -- several reindex
// triggers in a row, `chatbot_chunks`'s row count and newest timestamp
// never moved, and no error was ever logged either, meaning the
// goroutine simply never got scheduled again. Vercel's Go runtime
// freezes a function's whole process the instant its handler returns
// the HTTP response; a goroutine still running at that moment has no
// guaranteed chance to resume, since the platform gives no "waitUntil"
// equivalent to a Go handler and doesn't guarantee the same warm
// container is reused for the next request. The fix is the one every
// serverless platform's own docs recommend for this situation: do the
// work before responding, not after.
func (a *App) RebuildChatbotIndex(ctx context.Context) error {
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
			return fmt.Errorf("embed: %w", err)
		}
		embeddings = append(embeddings, batch...)
	}

	if err := a.ChatbotChunks.ReplaceAll(ctx, chunks, embeddings); err != nil {
		return fmt.Errorf("replace: %w", err)
	}
	return nil
}

// markChatbotReindexPending flags that course/article content changed,
// after a save or delete, so CronChatbotReindex picks it up on its next
// run -- a single fast UPDATE (ChatbotChunkRepo.MarkReindexPending), not
// a full re-embed, so a save is never slowed down by this. Logs rather
// than propagates any failure, same "degrade quietly" contract as the
// rest of this app's chrome lookups (see app.go). A no-op when
// NVIDIA_API_KEY isn't configured -- there would be nothing for the cron
// to reindex with anyway.
//
// RebuildChatbotIndex itself used to run here synchronously, immediately
// after every save. That correctly fixed an earlier bug (a detached
// goroutine that never actually completed on Vercel's runtime -- see
// RebuildChatbotIndex's own comment), but a second problem came with it:
// every course/article save started taking up to a minute, since a
// rebuild always re-embeds the *entire* corpus. This pending-flag +
// cron split keeps the fix (no fire-and-forget goroutine; the real
// re-embed still only ever runs synchronously inside a real HTTP
// request, in CronChatbotReindex) while giving saves their speed back.
func (a *App) markChatbotReindexPending(ctx context.Context) {
	if !a.Nvidia.Enabled() {
		return
	}
	if err := a.ChatbotChunks.MarkReindexPending(ctx); err != nil {
		log.Printf("mark chatbot reindex pending: %v", err)
	}
}

// CronChatbotReindex is the Vercel Cron target (see vercel.json's
// "crons") that actually runs the reindex a save only flagged as
// pending. Vercel's cron invoker sends `Authorization: Bearer
// $CRON_SECRET` automatically when that env var is set, which this
// checks against the same ChatbotReindexToken the agent-facing
// /api/chatbot/reindex endpoint uses -- CRON_SECRET and
// CHATBOT_REINDEX_TOKEN are set to the identical value in Vercel for
// exactly this reason, so one token authorizes both triggers. Exits
// immediately (no NVIDIA call, no DB write beyond the pending check) on
// every tick where nothing changed, which is the common case -- the
// free-tier NVIDIA budget is only spent when there's real work.
func (a *App) CronChatbotReindex(w http.ResponseWriter, r *http.Request) {
	token := a.Cfg.ChatbotReindexToken
	if token == "" {
		http.NotFound(w, r)
		return
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !a.Nvidia.Enabled() {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "nvidia not configured")
		return
	}
	pending, err := a.ChatbotChunks.ConsumePendingReindex(r.Context())
	if err != nil {
		log.Printf("cron chatbot reindex: check pending: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !pending {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "nothing pending")
		return
	}
	if err := a.RebuildChatbotIndex(r.Context()); err != nil {
		log.Printf("cron chatbot reindex: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	count, _ := a.ChatbotChunks.Count(r.Context())
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "reindexed: %d chunks", count)
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// stripHTML reduces admin-authored lesson/article HTML to plain text for
// embedding -- the chatbot only ever needs the words, never the markup.
func stripHTML(s string) string {
	s = htmlTagPattern.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// truncateChars caps s at maxChars, cutting on a word boundary rather
// than mid-word -- used to bound how much of one lesson/article body
// gets embedded (see chatbotMaxBodyChars) regardless of how long the
// source file is.
func truncateChars(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	cut := strings.LastIndexByte(s[:maxChars], ' ')
	if cut <= 0 {
		cut = maxChars
	}
	return s[:cut]
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

// gatherChatbotChunks reads every piece of indexable site content --
// profile, skills, experience, education, courses (+ gated lesson
// bodies), articles, consultancy tiers/FAQ, case studies,
// certifications, publications, and testimonials -- never anything
// from the repo itself (all of this is live DB/Storage content, or
// Go/template-literal copy for consultancy, per CLAUDE.md rule 8) and
// splits it into embeddable chunks. Best-effort throughout: a lookup
// failure for one content type just means that type is skipped this
// rebuild, not a failed reindex.
func (a *App) gatherChatbotChunks(ctx context.Context) []models.ChatbotChunk {
	var chunks []models.ChatbotChunk

	if profile, err := a.Profile.Get(ctx); err == nil {
		text := strings.Join(nonEmpty(profile.Name, profile.Title, profile.Tagline, profile.HeroSubtitle, profile.Summary), ". ")
		// RagContext (migration 073) is additive, same pattern as
		// courses.rag_context: Summary stays written for a visitor
		// reading the About page, RagContext is written specifically to
		// read well as retrieved evidence handed to the chatbot's LLM --
		// explicit years-of-experience figures, named skills, and named
		// AI tools read far better there than paraphrased out of prose.
		if profile.RagContext != "" {
			text += ". " + profile.RagContext
		}
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
		paths := make([]string, 0, len(courses))
		for _, c := range courses {
			if c.ContentObject != "" {
				paths = append(paths, c.ContentObject)
			}
		}
		// Every course's lesson body is an independent Storage read --
		// fetched concurrently (bounded) rather than one at a time, since
		// RebuildChatbotIndex now runs synchronously within an HTTP
		// request and each sequential round trip eats into
		// vercel.json's maxDuration for no reason (CLAUDE.md §3).
		bodies := a.fetchStorageBodies(ctx, paths)

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
			if body, ok := bodies[c.ContentObject]; ok {
				// A paid course's lesson body is the exact content
				// /courses/{slug}/learn paywalls -- gate it the same
				// way here, or the chatbot becomes a free side-channel
				// around that paywall. A free course has nothing to
				// gate: its lesson is already open to everyone.
				for _, piece := range chunkText(truncateChars(body, chatbotMaxBodyChars), chatbotChunkMaxChars) {
					chunks = append(chunks, models.ChatbotChunk{SourceType: "course", SourceID: c.ID, Title: "Course lesson: " + c.Title, URL: url, ChunkText: piece, IsGated: c.IsPaid})
				}
			}
		}
	}

	// Consultancy tiers and FAQ are template/Go-literal copy, not DB rows
	// (see docs/ARCHITECTURE.md) -- consultancyFAQ is the same function
	// consultancy.go uses for the page's own <details> accordion and its
	// FAQPage JSON-LD, so this can never drift from what a visitor
	// actually sees on /consultancy.
	auditDisplay, retainerDisplay := models.AuditPriceDisplay(), models.RetainerRangeDisplay()
	for _, item := range consultancyFAQ(auditDisplay, retainerDisplay) {
		chunks = append(chunks, models.ChatbotChunk{SourceType: "consultancy", Title: "Consultancy FAQ: " + item.Question, URL: "/consultancy", ChunkText: item.Question + " " + item.Answer})
	}
	tierText := "Consultancy tiers at /consultancy: " +
		"Architecture Review (" + auditDisplay + ", indicative, quoted after scoping) -- a focused written review of one system, migration, or architecture decision: a scoping call, the review, a written findings document with a specific recommendation, and a walkthrough call, typically a couple of weeks end to end. " +
		"Fractional Advisory (" + retainerDisplay + ") -- an ongoing month-to-month retainer: a standing review call plus async access for questions in between, with written notes and a recommendation from every review. " +
		"Schedule a Call (free) -- a free scoping call and the entry point into both paid tiers above; book it on the Booking page after signing in with Google."
	for _, piece := range chunkText(tierText, chatbotChunkMaxChars) {
		chunks = append(chunks, models.ChatbotChunk{SourceType: "consultancy", Title: "Consultancy tiers", URL: "/consultancy", ChunkText: piece})
	}

	if caseStudies, err := a.CaseStudies.ListPublished(ctx); err == nil {
		for _, cs := range caseStudies {
			text := cs.Title + ". Context: " + cs.ClientContext + ". Problem: " + cs.Problem + ". Approach: " + cs.Approach + ". Result: " + cs.Result
			for _, piece := range chunkText(text, chatbotChunkMaxChars) {
				chunks = append(chunks, models.ChatbotChunk{SourceType: "case_study", SourceID: cs.ID, Title: "Case study: " + cs.Title, URL: "/about#case-studies", ChunkText: piece})
			}
		}
	}

	if certs, err := a.Certifications.ListPublished(ctx); err == nil {
		for _, c := range certs {
			chunks = append(chunks, models.ChatbotChunk{SourceType: "certification", SourceID: c.ID, Title: "Certification: " + c.Name, URL: "/about#certifications", ChunkText: c.Name + " from " + c.Issuer})
		}
	}

	if pubs, err := a.Publications.ListPublished(ctx); err == nil {
		for _, p := range pubs {
			text := p.Title + " (" + p.Kind + ", " + p.Venue + "). " + p.Summary
			chunks = append(chunks, models.ChatbotChunk{SourceType: "publication", SourceID: p.ID, Title: p.Kind + ": " + p.Title, URL: "/about#publications", ChunkText: text})
		}
	}

	if testimonials, err := a.Testimonials.ListPublished(ctx); err == nil {
		for _, t := range testimonials {
			chunks = append(chunks, models.ChatbotChunk{SourceType: "testimonial", SourceID: t.ID, Title: "Testimonial from " + t.AuthorName, URL: "/about#testimonials", ChunkText: t.AuthorName + ", " + t.AuthorTitle + ": " + t.Body})
		}
	}

	if articles, err := a.Articles.ListPublished(ctx, 1000); err == nil {
		paths := make([]string, 0, len(articles))
		for _, art := range articles {
			if art.ContentObject != "" {
				paths = append(paths, art.ContentObject)
			}
		}
		bodies := a.fetchStorageBodies(ctx, paths)

		for _, art := range articles {
			url := "/articles/" + art.Slug
			text := art.Title + ". " + art.Summary
			body := art.Body
			if fetched, ok := bodies[art.ContentObject]; ok {
				body = fetched
			}
			text += ". " + truncateChars(stripHTML(body), chatbotMaxBodyChars)
			for _, piece := range chunkText(text, chatbotChunkMaxChars) {
				chunks = append(chunks, models.ChatbotChunk{SourceType: "article", SourceID: art.ID, Title: "Article: " + art.Title, URL: url, ChunkText: piece})
			}
		}
	}

	return chunks
}

// fetchStorageBodies reads several Storage objects concurrently (bounded
// to 5 at a time) and returns a path -> plain-text body map, skipping any
// path that fails to read -- these are independent lookups (CLAUDE.md
// §3), and since RebuildChatbotIndex now runs synchronously within an
// HTTP request (see its own comment), reading N lesson/article bodies
// one at a time would waste N round trips' worth of latency against
// vercel.json's maxDuration for no reason. CourseStorage's own access
// token is cached behind a mutex (core/storage/client.go), so concurrent
// calls are safe.
func (a *App) fetchStorageBodies(ctx context.Context, paths []string) map[string]string {
	out := make(map[string]string, len(paths))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)
	for _, p := range paths {
		g.Go(func() error {
			text, err := a.readStorageText(gctx, p)
			if err != nil {
				return nil
			}
			mu.Lock()
			out[p] = text
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return out
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
