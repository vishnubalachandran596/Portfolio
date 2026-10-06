-- Same pattern as courses.rag_context (migration 067): a passage written
-- specifically to read well as retrieved evidence handed to the RAG
-- chatbot's LLM, additive to profile.summary (which stays written for a
-- visitor reading the About page, not for retrieval quality). Admin-
-- editable at /admin/profile; saving already marks a reindex pending via
-- the existing AdminProfileSave -> markChatbotReindexPending call.
ALTER TABLE profile ADD COLUMN IF NOT EXISTS rag_context TEXT NOT NULL DEFAULT '';
