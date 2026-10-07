-- chatbot_reindex_pending (migration 070) was created after 051's RLS
-- lockdown pass and was missed by it -- Supabase's security advisor
-- flagged it as the one table in `public` still exposed to the
-- PostgREST anon/authenticated roles with no RLS at all (every other
-- table already has RLS enabled with zero policies, which is what
-- actually denies PostgREST access -- see 051's comment; the app itself
-- connects as the table owner and always bypasses RLS regardless).
--
-- Idempotent the same way 051 is: ENABLE ROW LEVEL SECURITY on an
-- already-enabled table is a no-op, and to_regclass guards a fresh
-- database where this table might not exist yet at migration time.
DO $$
BEGIN
    IF to_regclass('public.chatbot_reindex_pending') IS NOT NULL THEN
        ALTER TABLE public.chatbot_reindex_pending ENABLE ROW LEVEL SECURITY;
    END IF;
END $$;
