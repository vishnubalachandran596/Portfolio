-- Supabase Security Advisor (warnings): pin quiz_attempt_cap_guard search_path;
-- move vector/http extensions out of public into extensions. Idempotent.

CREATE OR REPLACE FUNCTION public.quiz_attempt_cap_guard()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public, pg_temp
AS $function$
DECLARE
    granted INT;
BEGIN
    IF NEW.user_id IS NULL OR NEW.attempt_no IS NULL THEN
        RETURN NEW;
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended('quiz_attempt:' || NEW.user_id || ':' || NEW.course_id, 0));
    SELECT g.extra INTO granted FROM public.quiz_attempt_grants g
     WHERE g.user_id = NEW.user_id AND g.course_id = NEW.course_id;
    IF NEW.attempt_no > 5 + COALESCE(granted, 0) THEN
        RAISE EXCEPTION 'quiz attempt limit reached' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$function$;

CREATE SCHEMA IF NOT EXISTS extensions;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        ALTER EXTENSION vector SET SCHEMA extensions;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'http') THEN
        ALTER EXTENSION http SET SCHEMA extensions;
    END IF;
END $$;
