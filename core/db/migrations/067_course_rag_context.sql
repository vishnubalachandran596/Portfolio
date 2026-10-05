-- A dedicated field for the chatbot's RAG index, separate from summary/
-- description: those are written for a human skimming a course card,
-- while this is written to be a self-contained, retrievable passage
-- (what the course covers, who it's for, outcomes) that reads well out
-- of context when handed to the LLM as retrieved evidence. Admin-editable
-- on the course form, same as every other course field -- content lives
-- here and in the admin console, never in this migration.
ALTER TABLE courses ADD COLUMN IF NOT EXISTS rag_context TEXT NOT NULL DEFAULT '';
