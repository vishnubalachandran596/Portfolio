# Portfolio

An end-to-end portfolio site for Vishnu Balachandran, built with a Go
backend: a public site (home, about, courses, book-a-session) plus a
password-protected admin portal to manage courses, bookings, weekly
availability, and profile/about content — all backed by Postgres
(Supabase-compatible), deployable to Vercel as a Go serverless function.

## Stack

- **Backend:** Go 1.24, standard library `net/http` (Go 1.22+ pattern-based
  `ServeMux`), `html/template` for server-rendered pages — no JS framework
  or build step.
- **Database:** PostgreSQL via `pgx` (works with Supabase, or any Postgres).
- **Auth:** bcrypt password hashing + JWT session in an HttpOnly cookie
  (`golang-jwt/jwt`), one admin-protected route tree.
- **Deployment target:** Vercel's Go runtime (`api/index.go` as a catch-all
  serverless function), or any host that can run a Go binary (`cmd/server`)
  — e.g. Fly.io, Render, a VM.

## Project layout

```
cmd/server/         Local dev entrypoint (long-lived HTTP server)
api/index.go         Vercel serverless entrypoint (wraps the same app)
internal/app/        Shared bootstrap: config, DB connect+migrate, wiring
internal/config/      Env var loading
internal/db/          Postgres connection + embedded SQL migrations
internal/models/      Domain types
internal/repo/        Database queries per entity
internal/middleware/  Admin session auth (JWT cookie) + logging
internal/authutil/    Password hashing
internal/scheduler/   Turns weekly availability rules into bookable slots
internal/handlers/    HTTP handlers (public site + admin portal) + routing
internal/view/        html/template renderer + all page templates
web/                  Embedded static assets (CSS/JS/SVG) served at /static
vercel.json           Vercel rewrite (all routes → api/index) + function config
```

## Local development

Requires Go 1.24+ and a Postgres database.

```bash
cp .env.example .env
# edit .env: DATABASE_URL, JWT_SECRET, ADMIN_BOOTSTRAP_EMAIL/PASSWORD

export $(grep -v '^#' .env | xargs)   # or use direnv / a dotenv tool
go run ./cmd/server
```

The app auto-applies its SQL migrations (schema + seed content) on
startup and, if `ADMIN_BOOTSTRAP_EMAIL`/`ADMIN_BOOTSTRAP_PASSWORD` are set
and no admin exists yet, creates the first admin account. Visit
`http://localhost:8080`, and log in at `/admin/login`.

**Change the bootstrap admin password after first login** — there's no
in-app password-change form yet; rotate it directly:

```sql
-- generate a bcrypt hash for the new password with any bcrypt tool, then:
UPDATE admins SET password_hash = '<new-bcrypt-hash>' WHERE email = 'admin@example.com';
```

### Running tests / checks

```bash
go build ./...
go vet ./...
gofmt -l .   # should print nothing
```

## Deploying: Supabase (database) + Vercel (app)

### 1. Create a Supabase Postgres database

1. Create a project at [supabase.com](https://supabase.com).
2. In **Project Settings → Database → Connection string**, copy the
   **Connection pooling** string (port `6543`, mode `transaction`) — not
   the direct connection. The app runs as a Vercel serverless function, so
   many short-lived instances may connect concurrently; the pooler avoids
   exhausting Postgres' connection limit. Append `?sslmode=require`.

### 2. Deploy to Vercel

1. Push this repo to GitHub (see below) and import it in Vercel, or run
   `vercel` from the repo root.
2. Vercel auto-detects `api/index.go` as a Go serverless function via
   `vercel.json`, which also rewrites every path to it so the Go router
   handles all site routes (`/`, `/about`, `/courses/...`, `/admin/...`).
3. Set these Environment Variables in the Vercel project (Production, and
   Preview if you want preview deploys to work):
   - `DATABASE_URL` — the Supabase pooled connection string from step 1
   - `JWT_SECRET` — a random secret (`openssl rand -hex 32`)
   - `APP_ENV=production`
   - `SITE_NAME` — display name
   - `SITE_URL` — your production URL
   - `ADMIN_BOOTSTRAP_EMAIL` / `ADMIN_BOOTSTRAP_PASSWORD` — creates the
     first admin on the next cold start; safe to leave set (it's a no-op
     once an admin exists), but consider removing after first login.
4. Deploy. On the function's first invocation it applies migrations and
   bootstraps the admin account automatically.

### Alternative: a single Go binary

`cmd/server` runs the exact same app as one process — no Vercel-specific
code required. `go build ./cmd/server` and run the resulting binary
anywhere with the same environment variables (Fly.io, Render, a VM,
Docker, etc.), pointed at any Postgres database (Supabase or otherwise).

## Content model & the admin portal

All site content lives in Postgres and ships pre-seeded with Vishnu's real
profile, skills, experience, and education (`internal/db/migrations/002_seed.sql`).
Everything below is editable from `/admin` after logging in:

- **Profile** (`/admin/profile`) — name, title, hero tagline/subtitle,
  about summary, links, photo.
- **Courses** (`/admin/courses`) — create/edit/delete/publish courses
  shown on `/courses`.
- **Bookings** (`/admin/bookings`) — view every booking request and set
  its status (pending/confirmed/cancelled).
- **Availability** (`/admin/availability`) — the weekly recurring rules
  (day, time window, slot length) that generate the bookable slots shown
  on `/booking`. The public booking form re-validates the chosen slot
  server-side against these rules and a unique DB constraint on the slot
  start time, so two visitors can never double-book the same slot.

Skills, experience, and education are currently seed-managed (edit
`internal/db/migrations/*.sql` or the rows directly); they weren't wired
into the admin UI to keep the CRUD surface focused on the three things
explicitly asked for (courses, bookings, profile).

## Notes on scope

- **CSRF:** admin forms rely on the session cookie's `SameSite=Lax`
  attribute (blocks cross-site POSTs) rather than a separate CSRF token —
  a reasonable simplification for a single-admin site; add token-based
  CSRF protection if that assumption changes.
- **OpenRouter:** used during the build of this site (not wired into the
  running app) to help draft the About page copy and homepage feature
  blurbs from Vishnu's real resume facts — no AI chat widget ships on the
  site itself, per explicit direction.
