-- 003_lock_down_postgrest.sql enabled RLS on the ten tables that existed at
-- the time. Every table added since (022, 023, 025, 026, 027, 031, 033, 036,
-- 046) was created without it, and Supabase grants the anon/authenticated
-- PostgREST roles full SELECT/INSERT/UPDATE/DELETE on new tables in `public`
-- by default. RLS with zero policies is the only thing that denies them.
--
-- The app is unaffected: it connects as the table owner over DATABASE_URL,
-- and an owner always bypasses RLS. This only closes the PostgREST door —
-- exactly as it has done for the original ten tables since migration 003.
--
-- Idempotent: ENABLE ROW LEVEL SECURITY on an already-enabled table is a
-- no-op, and every table is guarded with to_regclass in case a fresh
-- database reaches this file in a different state.
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'promo_codes', 'promo_code_courses', 'quiz_questions', 'quiz_attempts',
        'admin_messages', 'course_updates', 'testimonials', 'audit_requests',
        'article_thumbs_up', 'contact_messages',
        'certifications', 'publications'
    ] LOOP
        IF to_regclass('public.' || t) IS NOT NULL THEN
            EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', t);
        END IF;
    END LOOP;
END $$;

-- notify_course_update() (migration 026) runs with a role-mutable
-- search_path. It is not SECURITY DEFINER, so this is hardening rather than
-- an open hole, but pinning the search_path removes the whole class of
-- object-shadowing question. CREATE OR REPLACE is idempotent by definition.
-- Body is unchanged from 026 apart from the SET clause.
CREATE OR REPLACE FUNCTION public.notify_course_update()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public, pg_temp
AS $function$
BEGIN
    INSERT INTO admin_messages (user_id, body)
    SELECT DISTINCT p.user_id,
        '"' || c.title || '" was updated: ' || NEW.note ||
        COALESCE((SELECT E'\n\nFeel free to reach out to me at ' || email FROM profile WHERE email <> '' LIMIT 1), '')
    FROM purchases p
    JOIN courses c ON c.id = p.course_id
    WHERE p.course_id = NEW.course_id
      AND p.status = 'paid'
      AND p.user_id IS NOT NULL;
    RETURN NEW;
END;
$function$;
