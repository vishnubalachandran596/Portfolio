-- Supports anonymous thumbs-up with an optional name/email: email is the
-- unique identity (reuses the users table via a guest row, is_guest=true,
-- so it can de-dupe with the article_thumbs_up unique constraint exactly
-- like a real logged-in vote); a name with no email has nowhere stable to
-- live, so it's stored directly on the vote row instead and never dedupes.
--
-- ADD COLUMN IF NOT EXISTS is naturally idempotent (unlike ADD CONSTRAINT
-- in migration 034, which caused an outage from being re-run) — no guard
-- needed here.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_guest boolean NOT NULL DEFAULT false;
ALTER TABLE article_thumbs_up ADD COLUMN IF NOT EXISTS guest_name text NOT NULL DEFAULT '';
