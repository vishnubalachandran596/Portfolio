-- Admin sign-in no longer uses a stored password. The admin password is the
-- ADMIN_BOOTSTRAP_PASSWORD environment variable (Vercel secret), compared in
-- constant time at login; the admins table is only the list of admin emails.
-- Idempotent: DROP COLUMN IF EXISTS is a no-op the second time.
ALTER TABLE admins DROP COLUMN IF EXISTS password_hash;
