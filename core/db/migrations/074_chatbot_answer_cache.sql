-- Semantic cache for the chatbot's *finished answers*, not just its
-- retrieved chunks: a repeat or near-duplicate question can skip both
-- retrieval and the NVIDIA chat-completion call entirely and be served
-- straight from here -- see core/handlers/chatbot.go's Chat and
-- core/repo/chatbot_answer_cache.go.
--
-- course_id mirrors chatbot_chunks.is_gated's reasoning, but as the
-- actual course identity rather than a boolean: NULL means the cached
-- answer used no gated content and is safe to serve to any visitor; a
-- real course id means the answer was generated from that course's
-- gated lesson content, so it may only be served back to a visitor
-- whose own unlocked courses include that same id -- exactly the check
-- ChatbotChunkRepo.Search already does for a live chunk, applied here to
-- a cached answer instead. A boolean alone would leak one course's paid
-- content to a buyer of a completely different course; course_id is the
-- actual security boundary, not an incidental detail.
CREATE TABLE IF NOT EXISTS chatbot_answer_cache (
    id BIGSERIAL PRIMARY KEY,
    question_text TEXT NOT NULL,
    answer_text TEXT NOT NULL,
    source_titles TEXT[] NOT NULL DEFAULT '{}',
    source_urls TEXT[] NOT NULL DEFAULT '{}',
    course_id BIGINT,
    embedding vector(2048) NOT NULL,
    hit_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_served_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_chatbot_answer_cache_course ON chatbot_answer_cache(course_id);

ALTER TABLE chatbot_answer_cache ENABLE ROW LEVEL SECURITY;
