-- quiz_attempt_grants (migration 088) was created after 051's lockdown pass
-- without ENABLE ROW LEVEL SECURITY — same class of issue as
-- chatbot_reindex_pending (076). With RLS off, Supabase's PostgREST
-- anon/authenticated roles get default grants on new public tables.
-- Zero policies + RLS enabled denies them; the app owner bypasses RLS.
--
-- Idempotent: ENABLE on an already-enabled table is a no-op; to_regclass
-- guards a database where this table does not exist yet.
DO $$
BEGIN
    IF to_regclass('public.quiz_attempt_grants') IS NOT NULL THEN
        ALTER TABLE public.quiz_attempt_grants ENABLE ROW LEVEL SECURITY;
    END IF;
END $$;
