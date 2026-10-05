-- Paid course lesson content was being indexed into chatbot_chunks and
-- retrieved for any visitor's question, regardless of whether they'd
-- actually bought the course -- defeating the same paywall /courses/{slug}/learn
-- enforces. is_gated marks a chunk as answerable only to a visitor who
-- has unlocked the course it came from (checked per-request in
-- core/handlers/chatbot.go); everything else (profile, skills,
-- experience, education, course metadata, articles, free-course lessons)
-- stays ungated.
ALTER TABLE chatbot_chunks ADD COLUMN IF NOT EXISTS is_gated BOOLEAN NOT NULL DEFAULT false;
