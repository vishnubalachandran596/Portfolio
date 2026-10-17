-- Local placeholder admin. This file is not a migration.
-- Do not move it into core/db/migrations. Migrate() would run it on
-- every production cold start, and a failure there takes the whole
-- site down.
--
-- Email and password are the placeholders in .env.example:
--   admin@example.com
--   change-this-password
-- The password is not stored in the database: /admin/login compares what you
-- type with ADMIN_BOOTSTRAP_PASSWORD. The row only puts the email on the
-- admin list.
--
-- Google visitor login is not a row in this database. .env.example sets
-- the local placeholder redirect URL. Add that exact URL as an authorized
-- redirect URI on the OAuth client, then paste the client id and secret:
--   GOOGLE_REDIRECT_URL=http://localhost:8080/login/google/callback
--   SITE_URL=http://localhost:8080
--   GOOGLE_CLIENT_ID=<your-client-id>.apps.googleusercontent.com
--   GOOGLE_CLIENT_SECRET=<your-client-secret>
--
-- Run it after the app has applied migrations (the admins table has to
-- exist). From the repo root, with Compose up. This command is the same
-- in PowerShell, Command Prompt, and Mac Terminal:
--
--   docker compose exec -T db psql -U portfolio -d portfolio -f /local-admin.sql
--
-- Git Bash on Windows rewrites /local-admin.sql. Prefix the command with
-- MSYS_NO_PATHCONV=1 there.
--
-- Every start also adds ADMIN_BOOTSTRAP_EMAIL to the admin list, so this
-- file is only needed if you want the row before the app has started.
-- Running it again does not change an existing row.

INSERT INTO admins (email, name)
VALUES ('admin@example.com', 'Admin')
ON CONFLICT (email) DO NOTHING;
