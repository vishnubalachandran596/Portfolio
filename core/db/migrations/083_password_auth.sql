-- Password sign-up, security-question recovery and session invalidation.
-- Idempotent: every statement is IF NOT EXISTS / guarded, safe to re-run on
-- every cold start (see CLAUDE.md rule 4).

-- token_version is embedded in the signed session cookie and compared with
-- the row on each request, so a password reset or a forced logout kills old
-- cookies without a session table.
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version integer NOT NULL DEFAULT 0;
-- true only when the account has a real, user-chosen password. Google-created
-- accounts keep a random unusable hash and stay false.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_usable boolean NOT NULL DEFAULT false;
-- true once Google has proven control of the email. A password sign-up is
-- never verified (the site sends no email), so a Google sign-in by the real
-- owner takes over an unverified password account.
ALTER TABLE users ADD COLUMN IF NOT EXISTS google_verified boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS security_question text NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS security_answer_hash text NOT NULL DEFAULT '';

-- Accounts that existed before password sign-up were all created by Google
-- (or are anonymous thumbs-up guests, which stay unverified).
UPDATE users SET google_verified = true
WHERE is_guest = false AND password_usable = false AND google_verified = false;

-- Failed-attempt counters for login, password reset, admin login and
-- sign-up. Kept in Postgres because serverless instances share no memory.
-- key is a SHA-256 hex digest (of the lower-cased email or the client IP),
-- never the raw value.
CREATE TABLE IF NOT EXISTS auth_throttle (
    kind         text        NOT NULL,
    key          text        NOT NULL,
    fail_count   integer     NOT NULL DEFAULT 0,
    last_fail_at timestamptz NOT NULL DEFAULT now(),
    locked_until timestamptz,
    PRIMARY KEY (kind, key)
);
ALTER TABLE auth_throttle ENABLE ROW LEVEL SECURITY;
