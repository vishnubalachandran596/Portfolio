-- Singleton flag the admin course/article save/delete handlers set
-- cheaply (one fast UPDATE, no embedding work) instead of running a
-- full chatbot reindex inline. A Vercel Cron Job (see vercel.json)
-- polls this on its own schedule and does the actual re-embed in its
-- own request, so an admin save no longer waits on it.
--
-- id is always literal TRUE -- the CHECK constraint makes a second row
-- impossible, so there is exactly one row to update/read, ever.
CREATE TABLE IF NOT EXISTS chatbot_reindex_pending (
    id boolean PRIMARY KEY DEFAULT true,
    requested_at timestamptz,
    CONSTRAINT chatbot_reindex_pending_singleton CHECK (id)
);

INSERT INTO chatbot_reindex_pending (id, requested_at)
VALUES (true, NULL)
ON CONFLICT (id) DO NOTHING;
