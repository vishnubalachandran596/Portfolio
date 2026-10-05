-- nvidia/nv-embed-v1 (4096-dim, what chatbot_chunks.embedding was sized
-- for in migration 065) was retired by NVIDIA on 2026-08-25 and returns
-- HTTP 410 Gone -- confirmed from live production logs, not a guess.
-- core/ai.Client now defaults to nvidia/nv-embedqa-e5-v5 (1024-dim); this
-- resizes the column to match. The table has never held a successful
-- index (every embed call since go-live has 410'd), so there is no data
-- to migrate -- a plain ALTER COLUMN TYPE is safe and reruns as a no-op
-- once the column is already vector(1024).
ALTER TABLE chatbot_chunks ALTER COLUMN embedding TYPE vector(1024);
