package repo

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ChatbotAnswerCacheRepo struct{ db *sql.DB }

func NewChatbotAnswerCacheRepo(db *sql.DB) *ChatbotAnswerCacheRepo {
	return &ChatbotAnswerCacheRepo{db: db}
}

// chatbotAnswerCacheMaxDistance is deliberately much tighter than
// chatbotRelevanceThreshold (0.75, "is this even on topic"): a cache hit
// has to mean "this is essentially the same question," not just "a
// related one," or two differently-worded questions with different
// correct answers could collide and serve the wrong cached reply.
const chatbotAnswerCacheMaxDistance = 0.08

// Find returns the closest cached answer to queryEmbedding, if any is
// within chatbotAnswerCacheMaxDistance and allowed for this asker: a
// course_id IS NULL row is open to everyone, a row tagged with a real
// course id only matches when that id is in unlockedCourseIDs -- the
// same rule ChatbotChunkRepo.Search applies to a raw chunk, applied here
// to an already-finished answer. nil, nil means no qualifying hit, not
// an error.
func (r *ChatbotAnswerCacheRepo) Find(ctx context.Context, queryEmbedding []float32, unlockedCourseIDs []int64) (*models.ChatbotAnswerCacheEntry, error) {
	if unlockedCourseIDs == nil {
		unlockedCourseIDs = []int64{}
	}
	var e models.ChatbotAnswerCacheEntry
	err := r.db.QueryRowContext(ctx, `
		SELECT id, question_text, answer_text, source_titles, source_urls, course_id, hit_count, created_at, last_served_at,
			embedding <=> $1::vector AS distance
		FROM chatbot_answer_cache
		WHERE (course_id IS NULL OR course_id = ANY($3))
			AND embedding <=> $1::vector < $2
		ORDER BY distance
		LIMIT 1`, vectorLiteral(queryEmbedding), chatbotAnswerCacheMaxDistance, pq.Array(unlockedCourseIDs)).
		Scan(&e.ID, &e.Question, &e.Answer, pq.Array(&e.SourceTitles), pq.Array(&e.SourceURLs), &e.CourseID, &e.HitCount, &e.CreatedAt, &e.LastServedAt, &e.Distance)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// Touch records a cache hit (bumped inline before responding, a single
// fast UPDATE -- never an NVIDIA call) so the admin chatbot page can show
// how many generation calls the cache has actually saved.
func (r *ChatbotAnswerCacheRepo) Touch(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE chatbot_answer_cache SET hit_count = hit_count + 1, last_served_at = now() WHERE id = $1`, id)
	return err
}

// Insert records a freshly generated answer so the next matching
// question can be served from here instead of calling NVIDIA again.
// courseID is nil for an answer that used no gated content.
func (r *ChatbotAnswerCacheRepo) Insert(ctx context.Context, question, answer string, sourceTitles, sourceURLs []string, courseID *int64, embedding []float32) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO chatbot_answer_cache (question_text, answer_text, source_titles, source_urls, course_id, embedding)
		VALUES ($1,$2,$3,$4,$5,$6::vector)`,
		question, answer, pq.Array(sourceTitles), pq.Array(sourceURLs), courseID, vectorLiteral(embedding))
	return err
}

// Count returns how many answers are currently cached, for the admin
// chatbot page's summary.
func (r *ChatbotAnswerCacheRepo) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM chatbot_answer_cache`).Scan(&count)
	return count, err
}

// TotalHits returns the sum of every cache entry's hit_count -- how many
// times a question has actually been answered from the cache instead of
// a fresh NVIDIA call, for the admin chatbot page's summary.
func (r *ChatbotAnswerCacheRepo) TotalHits(ctx context.Context) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, `SELECT coalesce(sum(hit_count), 0) FROM chatbot_answer_cache`).Scan(&total)
	return total, err
}

// TruncateAll wipes every cached answer -- called at the end of
// RebuildChatbotIndex, since a reindex means the underlying content
// changed and a cached answer could now be citing something stale,
// wrong, or deleted. Simplest and safest to drop the whole cache and let
// it refill from real traffic, same "full replace" philosophy
// ChatbotChunkRepo.ReplaceAll already uses for the chunks themselves.
func (r *ChatbotAnswerCacheRepo) TruncateAll(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `TRUNCATE chatbot_answer_cache`)
	return err
}
