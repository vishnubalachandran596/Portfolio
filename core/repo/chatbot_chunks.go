package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ChatbotChunkRepo struct{ db *sql.DB }

func NewChatbotChunkRepo(db *sql.DB) *ChatbotChunkRepo { return &ChatbotChunkRepo{db: db} }

// vectorLiteral renders an embedding as pgvector's text input format
// ("[0.1,0.2,...]"), bound as an ordinary string parameter and cast with
// ::vector in SQL -- the app has no pgvector-aware driver type registered
// (database/sql via pgx's stdlib wrapper, see core/db/db.go), and this
// avoids adding one for a single column.
func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// ReplaceAll swaps the entire chatbot index for a freshly built one in a
// single transaction -- the admin's "Rebuild Index" action re-embeds every
// piece of site content from scratch, so a full replace is simpler and
// safer than diffing against what's already indexed. chunks and
// embeddings must be the same length, paired by position.
func (r *ChatbotChunkRepo) ReplaceAll(ctx context.Context, chunks []models.ChatbotChunk, embeddings [][]float32) error {
	if len(chunks) != len(embeddings) {
		return fmt.Errorf("chatbot chunks/embeddings length mismatch: %d vs %d", len(chunks), len(embeddings))
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `TRUNCATE chatbot_chunks`); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO chatbot_chunks (source_type, source_id, title, url, chunk_text, is_gated, embedding)
		VALUES ($1,$2,$3,$4,$5,$6,$7::vector)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, c := range chunks {
		if _, err := stmt.ExecContext(ctx, c.SourceType, c.SourceID, c.Title, c.URL, c.ChunkText, c.IsGated, vectorLiteral(embeddings[i])); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Count returns how many chunks are currently indexed, for the admin
// reindex page's "last built" summary.
func (r *ChatbotChunkRepo) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM chatbot_chunks`).Scan(&count)
	return count, err
}

// Search returns the limit chunks whose embedding is closest (cosine
// distance, pgvector's <=> operator) to queryEmbedding -- the chatbot's
// retrieval step for one incoming question. A gated chunk (a paid
// course's lesson content) is only eligible when its course is in
// unlockedCourseIDs -- an anonymous or non-buyer asker passes an empty
// slice, never nil, so `source_id = ANY($3)` compares against a real
// empty array rather than SQL NULL (which would make every comparison
// NULL, not false, and silently change nothing). Each returned chunk
// carries its cosine distance from the query, which the caller uses as a
// relevance gate before paying for a full generation call.
func (r *ChatbotChunkRepo) Search(ctx context.Context, queryEmbedding []float32, limit int, unlockedCourseIDs []int64) ([]models.ChatbotChunk, error) {
	if unlockedCourseIDs == nil {
		unlockedCourseIDs = []int64{}
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, source_type, source_id, title, url, chunk_text, is_gated, created_at,
			embedding <=> $1::vector AS distance
		FROM chatbot_chunks
		WHERE NOT is_gated OR source_id = ANY($3)
		ORDER BY distance
		LIMIT $2`, vectorLiteral(queryEmbedding), limit, pq.Array(unlockedCourseIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ChatbotChunk
	for rows.Next() {
		var c models.ChatbotChunk
		if err := rows.Scan(&c.ID, &c.SourceType, &c.SourceID, &c.Title, &c.URL, &c.ChunkText, &c.IsGated, &c.CreatedAt, &c.Distance); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
