-- Enables the RAG chatbot: pgvector as the vector store (kept in the same
-- Postgres instance per the "Postgres as the only datastore" architecture
-- rule, rather than a separate vector-DB service) and the table holding
-- embedded chunks of site content (profile, skills, experience, education,
-- courses, articles) that the chatbot retrieves against.
CREATE EXTENSION IF NOT EXISTS vector;

-- embedding is nvidia/nv-embed-v1's native dimension (4096). The corpus
-- here is small (a few hundred chunks at most for a personal site), so a
-- plain sequential scan with the <=> operator is fast enough without an
-- ANN index -- and pgvector's indexed types (HNSW/ivfflat) cap out well
-- below 4096 dimensions anyway.
CREATE TABLE IF NOT EXISTS chatbot_chunks (
    id BIGSERIAL PRIMARY KEY,
    source_type TEXT NOT NULL, -- 'profile' | 'skill' | 'experience' | 'education' | 'course' | 'article'
    source_id BIGINT,
    title TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    chunk_text TEXT NOT NULL,
    embedding vector(4096) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_chatbot_chunks_source ON chatbot_chunks(source_type, source_id);

ALTER TABLE chatbot_chunks ENABLE ROW LEVEL SECURITY;

-- Per-day message counter for the public chat endpoint's rate limit,
-- keyed by an anonymous per-browser id (cookie) rather than IP -- this is a
-- stateless serverless function per CLAUDE.md rule 3, so the limit has to
-- live in Postgres, not an in-process counter that wouldn't survive
-- between cold/warm instances anyway.
CREATE TABLE IF NOT EXISTS chatbot_rate_limits (
    session_id TEXT NOT NULL,
    day DATE NOT NULL,
    message_count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, day)
);

ALTER TABLE chatbot_rate_limits ENABLE ROW LEVEL SECURITY;
