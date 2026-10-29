-- Email-verified sign-up and email-link password reset.
--
-- users.email_verified: true once the owner of the address has proved it,
-- by Google sign-in or by clicking an emailed link. Every Google-verified
-- account is backfilled. A password account is created only after the link
-- is clicked, so new password accounts are always verified.
--
-- auth_tokens: one-time links for sign-up activation ('signup') and password
-- reset ('reset'). Only the SHA-256 hash of the token is stored. A sign-up
-- token also carries the pending name and password hash, so no users row
-- exists until the link is used.
--
-- Safe to run twice: every statement is guarded.

ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT false;

UPDATE users SET email_verified = true WHERE google_verified AND NOT email_verified;

DO $$
DECLARE
    rel_kind "char";
BEGIN
    SELECT c.relkind INTO rel_kind
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relname = 'auth_tokens';

    IF rel_kind IN ('r', 'p') THEN
        RETURN;
    END IF;
    IF rel_kind = 'c' THEN
        DROP TYPE public.auth_tokens;
    ELSIF rel_kind IS NOT NULL THEN
        RAISE EXCEPTION 'public.auth_tokens is relkind %; refusing to replace it', rel_kind;
    END IF;

    BEGIN
        CREATE TABLE IF NOT EXISTS public.auth_tokens (
            id BIGSERIAL PRIMARY KEY,
            purpose TEXT NOT NULL CHECK (purpose IN ('signup', 'reset')),
            email TEXT NOT NULL,
            token_hash TEXT NOT NULL,
            name TEXT NOT NULL DEFAULT '',
            password_hash TEXT NOT NULL DEFAULT '',
            next_path TEXT NOT NULL DEFAULT '',
            expires_at TIMESTAMPTZ NOT NULL,
            used_at TIMESTAMPTZ,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now()
        );
    EXCEPTION
        WHEN unique_violation OR duplicate_table OR duplicate_object THEN
            IF to_regclass('public.auth_tokens') IS NULL THEN
                RAISE;
            END IF;
    END;
END $$;

DO $$
BEGIN
    CREATE UNIQUE INDEX IF NOT EXISTS auth_tokens_hash_uidx ON public.auth_tokens (token_hash);
EXCEPTION
    WHEN unique_violation OR duplicate_table OR duplicate_object THEN
        IF to_regclass('public.auth_tokens_hash_uidx') IS NULL THEN
            RAISE;
        END IF;
END $$;

DO $$
BEGIN
    CREATE INDEX IF NOT EXISTS auth_tokens_email_idx ON public.auth_tokens (purpose, email);
EXCEPTION
    WHEN unique_violation OR duplicate_table OR duplicate_object THEN
        IF to_regclass('public.auth_tokens_email_idx') IS NULL THEN
            RAISE;
        END IF;
END $$;

-- Same RLS lockdown as the other tables: no policies, so PostgREST is denied.
DO $$
BEGIN
    IF to_regclass('public.auth_tokens') IS NOT NULL THEN
        ALTER TABLE public.auth_tokens ENABLE ROW LEVEL SECURITY;
    END IF;
END $$;
