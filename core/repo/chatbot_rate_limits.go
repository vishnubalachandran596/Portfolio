package repo

import (
	"context"
	"database/sql"
)

type ChatbotRateLimitRepo struct{ db *sql.DB }

func NewChatbotRateLimitRepo(db *sql.DB) *ChatbotRateLimitRepo { return &ChatbotRateLimitRepo{db: db} }

// IncrementAndCheck atomically bumps today's message count for a chatbot
// session and reports whether it's still within limit. The counter lives
// in Postgres rather than in-process (per CLAUDE.md rule 3 -- no state that
// must survive between requests outside the DB) since this serverless
// function can run as any number of concurrent cold/warm instances, each
// with its own memory.
func (r *ChatbotRateLimitRepo) IncrementAndCheck(ctx context.Context, sessionID string, limit int) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO chatbot_rate_limits (session_id, day, message_count)
		VALUES ($1, current_date, 1)
		ON CONFLICT (session_id, day) DO UPDATE SET message_count = chatbot_rate_limits.message_count + 1
		RETURNING message_count`, sessionID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count <= limit, nil
}
