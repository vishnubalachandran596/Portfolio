-- Belt-and-suspenders: enable RLS on every heap table in public that still
-- has it off. Matches migration 051's intent; catches any table missed in
-- its CREATE migration (e.g. quiz_attempt_grants before 097). PostgREST
-- anon/authenticated are denied when RLS is on and no policy grants access;
-- the app's DATABASE_URL role bypasses RLS as table owner.
--
-- Idempotent: ENABLE on an already-enabled table is a no-op.
DO $$
DECLARE
    r record;
BEGIN
    FOR r IN
        SELECT c.relname
          FROM pg_class c
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE n.nspname = 'public'
           AND c.relkind = 'r'
           AND NOT c.relrowsecurity
    LOOP
        EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', r.relname);
    END LOOP;
END $$;
