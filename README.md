# Portfolio

An end-to-end portfolio site for Vishnu Balachandran, built with a Go
backend: a public site (home, about, courses, book-a-session) plus a
password-protected admin portal to manage courses, bookings, weekly
availability, and profile/about content — all backed by Postgres
(Supabase-compatible), deployable to Vercel as a Go serverless function.

## Licence

Proprietary — **all rights reserved**. The source is published so it can be
read and evaluated; it is not open source. Viewing for evaluation is permitted,
and nothing else is: no use, copying, modification, redistribution or
deployment without written permission. See **[`LICENSE`](./LICENSE)**.

## Working on this repo? Read this first

Before making a change — human or AI agent — read:
- **[`CLAUDE.md`](./CLAUDE.md)** — architecture rules, extensibility pattern, and the "never commit secrets" policy. Cursor users: the same rules also live in `.cursor/rules/architecture.mdc` (auto-loaded) and `.cursorrules`. Codex users: see **[`AGENTS.md`](./AGENTS.md)**.
- **[`docs/FUNCTIONALITY.md`](./docs/FUNCTIONALITY.md)** — every feature, what a visitor/user/admin can do, and the business rules behind each flow.
- **[`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md)** — package layout, full ER diagram, auth model, payment flows, deployment.

These files are kept up to date in the same commit as whatever they describe — treat them as the current source of truth, not just onboarding reading.

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
cmd/server/       Local dev entrypoint (long-lived HTTP server)
Dockerfile        Image for that entrypoint
docker-compose.yml  Local Postgres (`db`) plus the app
api/index.go      Vercel serverless entrypoint (wraps the same app)
core/app/         Shared bootstrap: config, DB connect+migrate, wiring
core/config/      Env var loading
core/db/          Postgres connection + embedded SQL migrations
core/models/      Domain types
core/repo/        Database queries per entity
core/middleware/  Admin session auth (JWT cookie), domain routing, logging
core/authutil/    Password hashing
core/scheduler/   Turns weekly availability rules into bookable slots
core/handlers/    HTTP handlers (public site + admin portal) + routing + SEO
core/view/        html/template renderer + all page templates
web/              Embedded static assets (CSS/JS/SVG/OG image) served at /static
vercel.json       Vercel rewrite (all routes → api/index) + function config
```

(Package names avoid Go's `internal/` convention on purpose — Vercel's Go
builder compiles `api/index.go` as a standalone file outside the normal
module package graph, which trips Go's internal-package visibility rule
even for code inside the same module.)

## Local development

Docker Desktop on Windows and on Mac uses the same `docker-compose.yml`.
Copy `.env.example` to `.env`, set `JWT_SECRET` (and change the bootstrap
admin password), then start the stack. Save `.env` as UTF-8. If you
override `LOCAL_CONTENT_DIR`, use forward slashes
(`C:/Users/you/portfolio-local-content` on Windows).
`DATABASE_URL` in `.env.example` already points at the database Compose
creates (`postgres://portfolio:portfolio@db:5432/portfolio?sslmode=disable`).
Leave that line as-is unless you also change `POSTGRES_USER`,
`POSTGRES_PASSWORD`, and `POSTGRES_DB` in `docker-compose.yml`.

```bash
cp .env.example .env
# set JWT_SECRET, and change ADMIN_BOOTSTRAP_PASSWORD
docker compose up --build
```

The app applies its SQL migrations on startup. Those migrations create the
schema only. If `ADMIN_BOOTSTRAP_EMAIL` is set, the app adds it to the admin
list on every start. `/admin/login` accepts an admin email plus the value of
`ADMIN_BOOTSTRAP_PASSWORD`. The same placeholder account is in
`docker/local-admin.sql` (`admin@example.com` / `change-this-password`). That file is not a migration. After the stack has
started once, apply it with the same command in PowerShell, Command
Prompt, and Mac Terminal:

```bash
docker compose exec -T db psql -U portfolio -d portfolio -f /local-admin.sql
```

Git Bash on Windows rewrites `/local-admin.sql`. Prefix that command with
`MSYS_NO_PATHCONV=1` there.

Running it again leaves an existing admin row alone. Visit
`http://localhost:8080`, and log in at `/admin/login` with that email and
password. A fresh database has no profile, articles, or courses. Pages
still render. Saving **Admin → Profile** inserts the profile row. Load
`portfolio_seed_data.sql` from the Google Drive folder named `Portfolio`
when you want the live rows locally.

Article HTML, course HTML, resume PDFs, the profile photo, and the site
logo are not in that SQL and not in the repo. Put them in the directory
`LOCAL_CONTENT_DIR` names (default `../portfolio-local-content`, next to
the repo). Compose mounts it at `/content`. Use the same paths Storage
uses:

```text
../portfolio-local-content/articles/{slug}.html
../portfolio-local-content/courses/{slug}.html
../portfolio-local-content/resumes/default.pdf
../portfolio-local-content/resumes/{role-key}.pdf
../portfolio-local-content/profile/photo.{ext}
../portfolio-local-content/brand/logo.{ext}
```

A database row's `content_object` or `object_path` has to match the file
name. Do not set `LOCAL_CONTENT_DIR` in production. On Mac, Docker Desktop
needs that folder shared if it sits outside the default file-sharing list.
On Windows, keep the folder on a drive Docker Desktop can mount.

Visitor Google sign-in reads `.env`, not the database.
`GOOGLE_REDIRECT_URL` in `.env.example` is the local placeholder
`http://localhost:8080/login/google/callback`. Add that exact URL as an
authorized redirect URI on the OAuth client, and paste
`GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`. Until those two are set,
the login page says Google is not configured. `SITE_URL` in the example
is `http://localhost:8080`.

To run the Go process on the host instead of in Compose, start only the
database (`docker compose up db`) and point `DATABASE_URL` at `localhost`
instead of `db`, then `go run ./cmd/server`. Leave `LOCAL_CONTENT_DIR` as
the host path. That path needs Go 1.24+.

**Changing the admin password** — change `ADMIN_BOOTSTRAP_PASSWORD` (in
`.env` locally, or the Vercel variable) and restart or redeploy. The password
is never stored in the database, so there is no hash to update.

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
   - `ADMIN_BOOTSTRAP_EMAIL` / `ADMIN_BOOTSTRAP_PASSWORD` — the owner's
     admin email (added to the admin list on every start) and the admin
     password for `/admin/login`. Keep them set; mark the password
     Sensitive. Changing it and redeploying changes the password.
4. Deploy. On the function's first invocation it applies migrations and
   bootstraps the admin account automatically.

### Alternative: a single Go binary

`cmd/server` runs the exact same app as one process — no Vercel-specific
code required. `go build ./cmd/server` and run the resulting binary
anywhere with the same environment variables (Fly.io, Render, a VM,
Docker, etc.), pointed at any Postgres database (Supabase or otherwise).

## Content model & the admin portal

All visitor-facing content lives in Postgres and is edited from `/admin`
after logging in. The git repo carries schema and code only — a fresh
database from the migration chain has no profile, skills, articles or
courses until you load `portfolio_seed_data.sql` from the Google Drive
folder named `Portfolio` (see `docs/ARCHITECTURE.md` §11) or enter the
rows yourself. Everything below is editable from `/admin`:

- **Profile** (`/admin/profile`) — name, title, hero tagline/subtitle,
  about summary, links, photo.
- **Courses** (`/admin/courses`) — create/edit/delete/publish courses
  shown on `/courses`.
- **Bookings** (`/admin/bookings`) — view every consultancy booking and
  its payment status (pending payment/confirmed/cancelled).
- **Availability** (`/admin/availability`) — the weekly recurring rules
  (day, time window, slot length) that generate the bookable slots shown
  on `/booking`. The chosen slot is re-validated server-side against
  these rules on every checkout, and a unique DB constraint on the slot
  start time means two visitors can never both pay for the same slot.

Booking a consultancy session or buying a course requires an account
(`/login`, email + password — `core/repo/users.go`). Logging in with the
email/password that matches the `admins` table opens the admin dashboard
*and* a matching visitor session in one step, so the site owner can browse
the rest of the site (buy/preview courses, book sessions) exactly like any
other logged-in visitor.

Skills, experience, education, certifications, publications, articles,
and quiz questions are all admin-editable at `/admin/skills`,
`/admin/experience`, `/admin/certifications`, `/admin/publications`,
`/admin/articles`, and `/admin/courses/{id}/quiz`. There is no seed data
in the repo to edit instead.

## Paid courses (Razorpay)

Every published course is paid, priced by depth/advancement rather than a
flat rate — a `min_price_cents` floor per course in INR paise (the
visitor can pay more, never less), ranging from ₹1,900 for the shortest
course up to ₹5,900 for the flagship Java SSE Ultimate Guide. The
visitor picks any amount at or above that minimum ("pay what you want",
or free with an active 100%-off promo code), pays via Razorpay Checkout,
and is redirected to `/courses/<slug>/learn`. Checkout is INR-only —
there's no multi-currency handling anywhere in this codebase — and
requires a Google-authenticated account; there is no guest-checkout path.

- **Payment flow:** `POST /courses/<slug>/checkout/order` creates a
  Razorpay order and a `purchases` row (`core/payments`, `core/repo/purchases.go`).
  Razorpay's Checkout.js widget collects card details directly — this app
  never sees or stores them. `POST /courses/<slug>/checkout/verify` is the
  step that actually matters: it recomputes the HMAC-SHA256 signature
  server-side and only marks the purchase "paid" if it matches
  (`core/payments/razorpay.go`). Access is never granted from the
  client-side "success" callback alone.
- **Access:** a per-course HttpOnly cookie (`ca_<courseID>`) holding an
  opaque access token, set once payment is verified. Logged-in admins
  always have access, for previewing paid content.
- **Content:** paid course content is a full HTML document stored in a
  private Supabase Storage bucket and read by `/courses/<slug>/learn`
  after the purchase check, never from `/static` or the sitemap, and
  never via a Storage URL the browser could request on its own. The
  response is the site chrome plus a 75vw × 75dvh pane. The lesson loads
  in a unique-origin iframe (`sandbox="allow-scripts"`, no
  `allow-same-origin`) so the parent cannot read the lesson DOM. Uploaded
  `<style>` is stripped so any tags follow the site typography. The
  document is served by its own gated route to the iframe only, so
  view-source of the outer page does not show the lesson markup. A new lesson is uploaded from the admin course
  form. Articles use the same pane and a renderer-scoped watermark; their
  HTML stays in the page so they can be indexed.
- **On "no one can copy the content":** be realistic about this — nothing
  rendered in a browser can be made impossible to copy (screenshots and
  devtools still exist, and the shell payload can be decoded). What's
  actually implemented: the content is never at a public URL, it's sent
  with `Cache-Control: no-store`, the lesson tags stay out of view-source,
  right-click is discouraged, copied text is stamped with the purchaser's
  email throughout, the parent cannot read the lesson iframe DOM, and
  **vishnubalachandran.com is drawn once, horizontally, centered in the
  renderer pane**,
  which is the actually-effective deterrent — a leaked copy is traceable
  back to whoever shared it.
- **Setup:** set `RAZORPAY_KEY_ID` / `RAZORPAY_KEY_SECRET` (start with
  Razorpay's test-mode keys). The purchase button is hidden until both are
  set. Checkout is INR-only. See `/admin/purchases` for a log of every
  order and its status.

### Notifying buyers when a course is updated

Course content updates (rewriting/extending a lesson) are done by uploading
a new HTML file on the course edit form, which replaces the private
Storage object. To let every paid buyer of that course know what
changed, insert one row into `course_updates` (`course_id`, `note`); a
Postgres trigger (`core/db/migrations/026_course_updates.sql`) fans it out
automatically to an `admin_messages` row per paid buyer — the same table
and mechanism that already powers the notification bell and
`/account#messages`, contact-email signature included. Nothing else to
run or remember:

```sql
INSERT INTO course_updates (course_id, note)
VALUES ((SELECT id FROM courses WHERE slug = 'payment-integration-razorpay'),
        'Added a real production bug walkthrough and a promo-code section.');
```

The same thing is also exposed in the admin portal: the course edit form
(`/admin/courses/<id>/edit`) has an optional "What changed" field that,
when filled in, calls the identical `CourseUpdateRepo.Create` under the
hood. Leaving it blank saves the course silently.

## Consultancy (paid 1:1 booking)

`/consultancy` is the only page with a "Book Now" call to action; it links
to `/booking`, which requires login and shows the price up front (₹3,000/hr
India, $40/hr elsewhere — `models.ConsultancyRegionalPrice`, the same
geo-header-driven regional pricing pattern as courses) before any slot is
picked. Picking a slot reserves it (`status='pending_payment'`) the moment
a Razorpay order is created — the same unique-slot-start constraint used
for the old free flow now doubles as the anti-double-booking guarantee
during checkout — and confirms it once the payment signature verifies
(`core/handlers/consultancy.go`). An abandoned checkout frees the slot
automatically 20 minutes later; no cleanup job needed.

## Domain architecture (www canonical, optional page-group subdomains)

The public canonical host is `www.yourdomain.com`. The bare apex and the
old `home.` host 301 to www with the same path; www itself is never
redirected (a canonical homepage that 3xxs is not indexed). Set
`DOMAIN=yourdomain.com` (no scheme, no `www`) to also pin optional
page-group subdomains:

| Host | Serves |
|---|---|
| `www.yourdomain.com` | The full path-based site (canonical; never redirected) |
| `about.yourdomain.com` | About Me |
| `courses.yourdomain.com` | Courses list/detail, checkout, and gated paid-course content |
| `consultancy.yourdomain.com` | The consultancy page and the (login-required) booking/checkout flow |
| `contact.yourdomain.com` | Contact |
| `admin.yourdomain.com` | The admin portal |

`/static/*`, `/sitemap.xml`, and `/robots.txt` are exempt from host
redirects and served identically on every subdomain (since pages reference
`/static/...` with relative paths). Canonical tags, OG URLs, and the
sitemap prefer `SITE_URL` (www). See `core/middleware/canonical.go`,
`core/middleware/domain.go`, and `core/handlers/seo.go` (`pageURL`).

Leaving `DOMAIN` unset (e.g. on a raw `*.vercel.app` preview URL) disables
the page-group redirects — no-op passthrough — while `CanonicalHost` still
301s apex/`home.` to www whenever `SITE_URL` is the www origin.

## SEO, legal, and UX checklist status

Implemented:
- **SEO:** `/sitemap.xml`, `/robots.txt`, per-page canonical tags,
  Open Graph + Twitter Card meta (including a generated `og-image.png`),
  JSON-LD (`Person`/`WebSite` on the home page, `Course` on course pages),
  `X-Robots-Tag: noindex` on the entire admin portal, single `<h1>` per
  page, clean URL slugs, mobile-responsive layout, no third-party scripts
  to slow down Core Web Vitals.
- **Legal/compliance:** `/privacy`, `/terms`, `/refund`, `/cookies` with
  real content (data collected, retention, deletion requests, children's
  data, no dark patterns/hidden fees), footer links, a minimal cookie
  consent banner.
- **UX:** dark/light theme toggle (persisted), full-site search (`/search`),
  expandable FAQ, password-visibility toggle on admin login, loading state
  on form submit, copy-to-clipboard (email), print stylesheet, scroll
  progress bar, back-to-top button, floating "Book time" CTA, sticky
  header, mobile menu, skip-to-content link, confirmation dialogs on
  destructive admin actions.

Explicitly **not** implemented (each needs an infrastructure/account
decision this repo can't make on its own):
- **Newsletter signup automation** — email *capture* exists (`POST
  /subscribe` on Home/Courses/Articles, `subscribers` table, admin list at
  `/admin/subscribers`), but nothing automated is sent on signup or ever
  after: no welcome email, no actual newsletter, no unsubscribe link,
  because there is still no transactional email provider (Resend, etc.)
  wired up anywhere in this codebase. Vishnu reaches out to the list
  manually. A real double opt-in/unsubscribe compliance flow is required
  before any automated sending is added on top of this.
- **UTM tracking** — added when you share links externally, not something
  to bake into internal navigation.
- **Google Search Console verification / sitemap submission** — a manual
  step in Google's dashboard once the site is live on a real domain.
- **Backlink strategy** — an ongoing external activity, not a code change.
- **SPF/DKIM/DMARC, `mail.`/`news.` email subdomains** — no transactional
  or marketing email is sent by the app yet, so there's nothing to
  authenticate; revisit if/when booking confirmation emails are added.

## Notes on scope

- **CSRF:** admin forms rely on the session cookie's `SameSite=Lax`
  attribute (blocks cross-site POSTs) rather than a separate CSRF token —
  a reasonable simplification for a single-admin site; add token-based
  CSRF protection if that assumption changes.
- **OpenRouter:** used during the build of this site (not wired into the
  running app) to help draft the About page copy and homepage feature
  blurbs from Vishnu's real resume facts — no AI chat widget ships on the
  site itself, per explicit direction.
