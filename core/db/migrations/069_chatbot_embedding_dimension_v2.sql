-- Second embedding-model swap in the same incident (see migration 068):
-- nvidia/nv-embedqa-e5-v5 turned out to be the same retired generation as
-- nv-embed-v1 (identical 2026-08-25 end-of-life date), not a working
-- replacement -- confirmed from a second live 410 in production. Moved to
-- nemotron-3-embed-1b (2048-dim native output, released 2026-07-16).
-- chatbot_chunks has still never held a successful index, so this
-- remains a safe, data-free resize.
ALTER TABLE chatbot_chunks ALTER COLUMN embedding TYPE vector(2048);
