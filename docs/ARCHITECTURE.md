# Architecture Reference — vishnubalachandran.com

**Purpose of this document:** everything about *how* this codebase is built — stack, package layout, data model (with an ER diagram), request lifecycle, auth, payments, rendering, and deployment — detailed enough that a fresh AI agent session (or a new engineer) can start making correct changes without re-deriving any of this from scratch. Companion to `docs/FUNCTIONALITY.md`, which covers *what* the product does.

**Keep this updated.** A new table, a new package, a new route pattern, a new external integration — update this file in the same commit.

**Working conventions for any agent or developer touching this repo** live in `CLAUDE.md` (repo root), mirrored for Cursor in `.cursor/rules/architecture.mdc` and `.cursorrules`, and for Codex in `AGENTS.md` — read those before making a change, alongside this file and `docs/FUNCTIONALITY.md`.

---

## 1. Stack at a glance

| Layer | Choice | Notes |
|---|---|---|
| Language | Go 1.24 | Standard library `net/http` — no web framework |
| Routing | `http.ServeMux` (Go 1.22+ pattern syntax: `"GET /path/{id}"`) | Two muxes: public + admin, composed together |
| Templates | `html/template`, embedded via `go:embed` | Server-rendered, no client-side framework/build step |
| Database | PostgreSQL (Supabase-hosted) | Accessed via `database/sql`, driven by `jackc/pgx/v5`'s `stdlib` adapter (`stdlib.OpenDB` in `core/db/db.go`); `lib/pq` is a secondary dependency used only for its `pq.Array()` helper (e.g. `articles.tags text[]`), not as the connection driver. Raw SQL throughout, no ORM |
| Migrations | Custom runner, `core/db/db.go` | Numbered `.sql` files in `core/db/migrations/`, tracked in a `schema_migrations` table, applied on every cold start |
| Auth | JWT in `HttpOnly` cookies (`golang-jwt/jwt/v5`) | Two independent sessions: admin and user (visitor) |
| Payments | Razorpay (Orders API + Checkout.js) | Server creates orders, verifies HMAC signatures server-side; India + international pricing |
| PDF generation | `github.com/go-pdf/fpdf` | In-process — resume, receipts, certificates; no headless-browser dependency |
| Deployment | Vercel, one Go serverless function | `api/index.go` is the entrypoint; `vercel.json` rewrites everything to it |
| Static assets | `go:embed` (`web/embed.go`) | CSS/JS/images compiled into the binary, served at `/static/`, cache-busted via a content hash |
| CSS | Plain CSS, custom-property design tokens | No Tailwind/Bootstrap, no build step |
| JS | Vanilla JS, no framework | `main.js` (public), `admin.js` + `theme.js` (admin) |

---

## 2. Package layout

```
core/
  app/          Bootstrap() — the one place that wires config+DB+renderer into handlers.App
  authutil/     password hashing (bcrypt wrapper) + RandomHex, the one shared source of cryptographically random tokens (OAuth state, access tokens, cert codes, placeholder passwords)
  certificate/  PDF quiz-completion certificate generator
  config/       env-var loading (Config struct)
  content/      go:embed of the course content HTML files under core/content/courses/*.html
  db/           DB connection + migration runner
  geo/          visitor country detection from request headers
  handlers/     ALL HTTP handlers + App struct (the biggest package — see §4)
  middleware/   auth (admin+user sessions), domain/subdomain routing, security headers, logging
  models/       every domain struct + business-rule methods (pricing display, status enums, etc.)
  notify/       Telegram notification sender
  oauth/        Google OAuth2 client
  payments/     Razorpay client (order creation, signature verification)
  receipt/      PDF payment-receipt generator
  repo/         one file per table/aggregate — all SQL lives here, nowhere else
  resume/       PDF resume generator
  scheduler/    turns availability_rules + existing bookings into bookable time slots
  view/         html/template renderer setup + template FuncMap
web/
  embed.go      go:embed of static/ (CSS/JS/images) + the AssetVersion cache-busting hash
  static/       css/, js/, img/
  templates/
    layout/     base.html, admin_base.html, header.html, footer.html, adminnav.html
    pages/      one file per route (see docs/FUNCTIONALITY.md for what each renders)
api/
  index.go      Vercel serverless entrypoint (Handler func, warm-instance reuse via sync.Once)
cmd/server/
  main.go       local-dev entrypoint (plain http.ListenAndServe)
docs/
  FUNCTIONALITY.md   what the product does (companion to this file)
  ARCHITECTURE.md    this file
```

**Convention:** a new feature almost always touches the same four layers in the same order: **`models`** (struct + any business-rule methods) → **`repo`** (a new file, or new methods on an existing repo, all SQL contained here) → **`handlers`** (wire the repo into `App`, add a handler, render or redirect) → **`web/view/templates/pages`** (the HTML) → **`routes.go`** (register the route). Follow this order for anything new; it's what every existing feature does.

---

## 3. Data model — ER diagram

```mermaid
erDiagram
    admins ||--o{ admin_messages : "sends (via UserRepo)"
    users ||--o{ purchases : makes
    users ||--o{ bookings : makes
    users ||--o{ testimonials : submits
    users ||--o{ admin_messages : receives
    users ||--o{ quiz_attempts : takes
    courses ||--o{ purchases : "is bought via"
    courses ||--o{ quiz_questions : has
    courses ||--o{ quiz_attempts : has
    courses ||--o{ course_updates : "notifies buyers via"
    promo_codes ||--o{ promo_code_courses : scopes
    courses ||--o{ promo_code_courses : "scoped by"
    promo_codes ||--o{ purchases : "discounts (nullable FK)"
    promo_codes ||--o{ bookings : "discounts (nullable FK)"
    availability_rules ||--o{ bookings : "generates slots for (app-level, not FK)"
    articles ||--o{ article_thumbs_up : "receives"
    users ||--o{ article_thumbs_up : "casts (nullable FK — real login, guest-by-email, or a name-only vote with no user row)"

    admins {
        serial id PK
        text email UK
        text password_hash
        text name
        timestamptz created_at
    }
    users {
        bigserial id PK
        text email UK
        text password_hash
        text name
        bool is_guest "true = captured via anonymous article thumbs-up form, never signed in"
        timestamptz created_at
    }
    profile {
        smallint id PK "always 1"
        text name
        text title
        text tagline
        text hero_subtitle
        text summary
        text location
        text email
        text linkedin_url
        text github_url
        text resume_url
        text photo_url
        smallint years_experience
        timestamptz updated_at
    }
    skill_groups {
        serial id PK
        text category
        text level
        text achievement
        text items "comma-joined tags"
        int sort_order
    }
    experiences {
        serial id PK
        text company
        text role
        text location
        text start_date
        text end_date
        text bullets "newline-joined"
        int sort_order
    }
    education {
        serial id PK
        text institution
        text degree
        text field
        int start_year
        int end_year
        text details
        int sort_order
    }
    testimonials {
        bigserial id PK
        bigint user_id FK "nullable, NULL = admin-authored"
        text author_name
        text author_title
        text body
        bool is_published
        int sort_order
        timestamptz created_at
    }
    home_features {
        serial id PK
        text title
        text description
        int sort_order
    }
    courses {
        serial id PK
        text slug UK
        text title
        text summary
        text description
        text level
        text duration
        int price_cents
        text currency
        text image_url
        bool is_published
        int sort_order
        bool is_paid
        int min_price_cents "USD, 0 = free"
        int min_price_cents_inr "INR, 0 = free"
        text content_key
        text content_html "uploaded single-file course content"
        timestamptz created_at
        timestamptz updated_at
    }
    course_updates {
        bigserial id PK
        bigint course_id FK
        text note
        timestamptz created_at
        note "Postgres trigger fans this out to admin_messages for every paid buyer"
    }
    quiz_questions {
        bigserial id PK
        bigint course_id FK
        text question
        text option_a
        text option_b
        text option_c
        text option_d
        char correct_option "A-D"
        int sort_order
    }
    quiz_attempts {
        bigserial id PK
        bigint course_id FK
        bigint user_id FK "nullable"
        bool is_admin
        text name
        text email
        int total_questions
        int correct_answers
        int score_percent
        bool passed
        text certificate_code UK "nullable, set only when passed"
        timestamptz created_at
    }
    purchases {
        bigserial id PK
        bigint course_id FK
        bigint user_id FK "nullable = guest checkout"
        text email
        int amount_cents
        text currency
        text status "created | paid | failed"
        text razorpay_order_id UK
        text razorpay_payment_id
        text access_token "bearer token unlocking /learn"
        bigint promo_code_id FK "nullable"
        text promo_code
        int discount_cents
        timestamptz created_at
        timestamptz paid_at
    }
    bookings {
        serial id PK
        bigint user_id FK
        text name
        text email
        text message
        timestamptz slot_start UK
        timestamptz slot_end
        text status "pending_payment | confirmed | cancelled"
        int amount_cents
        text currency
        text razorpay_order_id
        text razorpay_payment_id
        timestamptz previous_slot_start "nullable"
        timestamptz rescheduled_at "nullable"
        bool reschedule_acknowledged
        bigint promo_code_id FK "nullable"
        text promo_code
        int discount_cents
        timestamptz created_at
    }
    audit_requests {
        bigserial id PK
        text name
        text email
        text company
        text brief
        int amount_cents
        text currency
        text status "pending_payment | paid"
        text razorpay_order_id UK
        text razorpay_payment_id
        timestamptz created_at
        timestamptz paid_at "nullable"
        note "Architecture Review — fixed price, modeled like bookings not purchases"
    }
    availability_rules {
        serial id PK
        smallint weekday "0-6"
        time start_time
        time end_time
        int slot_minutes
        bool active
        timestamptz created_at
    }
    articles {
        bigserial id PK
        text slug UK
        text title
        text summary
        text body "empty = curated link, non-empty = original post"
        text cover_image_url
        text source_url
        text source_name
        text_array tags
        int thumbs_up_count
        date published_date "nullable"
        bool is_published
        int sort_order
        timestamptz created_at
    }
    article_thumbs_up {
        bigserial id PK
        bigint article_id FK
        bigint user_id FK "nullable — set for a real or guest-by-email vote; UNIQUE(article_id, user_id) is the de-dup"
        text guest_name "only used when user_id is null (name given, no email) — never deduped"
        timestamptz created_at
    }
    promo_codes {
        bigserial id PK
        text code UK
        text description
        text product "course | consultancy"
        int discount_percent
        bool applies_to_all_courses
        int max_redemptions "0 = unlimited"
        int redemption_count
        timestamptz expires_at
        bool is_active
        timestamptz created_at
    }
    promo_code_courses {
        bigint promo_code_id PK_FK
        bigint course_id PK_FK
    }
    admin_messages {
        bigserial id PK
        bigint user_id FK
        text body
        bool is_read
        timestamptz created_at
    }
    schema_migrations {
        text version PK
        timestamptz applied_at
    }
```

**Notes on the model that aren't obvious from the diagram:**
- `admins` and `users` are **completely separate tables** — an admin logging in does not create a `users` row, and vice versa. The two sessions (§5) are independent; the same email *can* hold both.
- `purchases.access_token` is the single mechanism gating `/courses/{slug}/learn` — see `courseAccess()` in `core/handlers/checkout.go`. A `$0` "free" purchase still gets a real row and a real token; it just skips Razorpay entirely (`amountCents == 0` branch in `CheckoutCreateOrder`).
- `audit_requests` deliberately does **not** reuse the `purchases` table, even though it's conceptually similar (a paid Razorpay flow) — it has no course, no pay-what-you-want amount, and a different intake form (name/company/brief), so it's modeled as its own table closer to `bookings`' pending→paid pattern.
- `course_updates` has no Go-side fan-out logic — a **Postgres trigger** (`trg_course_updates_notify`, from migration `026_course_updates.sql`) does the INSERT into `admin_messages` for every buyer automatically. Inserting a `course_updates` row (from the admin course-edit form's "what changed" field, or directly via SQL) is the *entire* notification mechanism.
- `min_price_cents` / `min_price_cents_inr` on `courses` default to `0` (migration `030`) — "contribute as you like, fully free" is the standard state; a positive value is an opt-in minimum, not the default.

---

## 4. `core/handlers` — the App struct and request flow

`handlers.App` (in `core/handlers/app.go`) is the one struct every handler is a method on. It holds:
- `Cfg *config.Config`, `DB *sql.DB`, `View *view.Renderer`, `Auth *middleware.Auth`
- One repo field per aggregate: `Profile, Skills, Experience, Education, Features, Courses, Bookings, Availability, Admins, Purchases, Users, Articles, PromoCodes, QuizQuestions, QuizAttempts, AdminMessages, CourseUpdates, Testimonials, AuditRequests, ArticleVotes`
- `Razorpay *payments.Razorpay`, `Telegram *notify.Telegram`, `Google *oauth.Google`, `Timezone *time.Location`

`App.base(r, title, metaDescription, active) BaseData` runs on **every single page render** — it resolves login state (both admin and user), notifications, the footer's LinkedIn link, nav URLs, and the asset-version string. Because it runs on every page, its three independent DB lookups (user name, notifications, profile) are fetched **concurrently** via `sync.WaitGroup` (not `errgroup` — this function's existing contract is "degrade quietly on error," which `errgroup` doesn't fit). Every page-specific `*Data` struct embeds `BaseData`.

`App.Routes()` (in `routes.go`) builds the full `http.Handler`: one public `ServeMux`, one admin-auth `ServeMux`, one `RequireAdmin`-wrapped protected `ServeMux` for the rest of `/admin/`, composed and wrapped in `middleware.Logging(middleware.SecurityHeaders(middleware.DomainRouter(domain)(mux)))`. See `docs/FUNCTIONALITY.md` for what each route does; the exact route table (method + path + handler) is the source of truth in `routes.go` itself — don't let this doc's prose drift from it, read `routes.go` directly for the literal list.

Handler files, by area:
| File | Covers |
|---|---|
| `public.go` | Home, About, Resume, CoursesList, CourseDetail, Sitemap-adjacent data |
| `checkout.go` | Course purchase order/verify, `courseAccess()` gate |
| `consultancy.go` | Consultancy page, Audit checkout order/verify, Booking page/checkout/verify, Contact |
| `articles.go` | Public article list/detail/thumbs-up + Admin article CRUD |
| `account.go` | `/account` page + PDF receipts + testimonial submission |
| `booking_reschedule.go` | Both admin- and self-service booking reschedule flows |
| `learn.go` | Gated course content serving + watermark injection |
| `quiz.go` | Quiz taking, scoring, certificate download |
| `auth.go` | Login page, Google OAuth start/callback, logout |
| `legal.go` | Generic static legal-page handler factory |
| `seo.go` | `baseURL`/`pageURL` helpers, Sitemap, Robots, LLMsTxt, IndexNow key + submit, Search |
| `schema.go` | JSON-LD builders (Person/Website/Service, Article, Course) |
| `admin.go` | Admin login, dashboard, course CRUD, bookings, purchases, availability, profile, preview-as-visitor |
| `admin_content.go` | Admin CRUD for skills, experience, testimonials |
| `admin_users.go` | Admin user list/detail/promote/demote/message |
| `admin_quiz.go` | Admin CRUD for quiz questions |
| `admin_promo.go` | Admin CRUD for promo codes |
| `admin_payments.go` | The combined payments ledger (purchases + bookings + audit requests merged) |
| `promo.go` | `resolvePromoCode()` — the one shared discount-resolution function all three checkout flows call |

---

## 5. Auth — two independent JWT sessions

Both live in `core/middleware/auth.go`, sharing one `Auth` struct (one JWT secret, one TTL — 7 days) but **separate cookies and separate context keys**:

| | Admin | User (visitor) |
|---|---|---|
| Cookie name | `portfolio_admin_session` | `portfolio_session` |
| Claims | `{admin_id}` | `{user_id}` |
| Gate middleware | `Auth.RequireAdmin` | `Auth.RequireUser` |
| Unauthenticated redirect | `/admin/login` | `/login?next=<path>` |
| Context accessor | `middleware.AdminIDFromContext` | `middleware.UserIDFromContext` |
| Non-redirecting check | `Auth.IsAuthenticated(r)` | `Auth.IsUserAuthenticated(r)` |

**Visitor login is Google-only** — deliberately, per the comment at the top of `auth.go`: one "Sign in with Google" button serves both a regular visitor and the site owner (a Google account whose email matches an `admins` row also gets an admin session issued alongside the normal user session), so there's no separate password to set up or lose. `core/oauth/google.go` is a standard authorization-code OAuth2 flow: `GoogleLoginStart` builds the consent-screen URL with a random CSRF `state` stashed in a short-lived cookie, `GoogleLoginCallback` exchanges the code, fetches the Google profile, and creates-or-finds a `users` row by email before issuing the session. Admin login (`/admin/login`) is the only place email/password auth exists in this codebase, via `core/authutil` (bcrypt).

**Admin preview-as-visitor** is a third, lightweight state: a plain cookie flag (`preview_visitor`), not a session — `isPreviewingAsVisitor(r)` short-circuits `base()`'s `isAdmin` computation to `false` while the admin's real session cookie stays intact, so `/exit-preview` just clears the flag.

---

## 6. Payments — three independent Razorpay flows, one shared pattern; currently only one is live

All three follow the same shape: **create-order handler** (server resolves the true price, creates a Razorpay order, writes a `pending`/`created` DB row) → client opens Razorpay's Checkout.js modal with that order ID → **verify handler** (HMAC-verifies the callback server-side via `Razorpay.VerifySignature`, flips the DB row to paid, fires a Telegram notification, returns a redirect). Access/confirmation is **only ever granted from the verify step**, never trusted from the client-side "success" callback alone.

**As of now, only the course flow can actually reach Razorpay.** A deliberate business policy — "no live payment gateway anywhere except courses, for now" — is enforced in code via `consultancy.go`'s `nonCoursePaymentsDisabled = true` constant, checked as the very first line of both `AuditCheckoutOrder` and `BookingCheckoutOrder`'s paid branch, *before* the usual `a.Razorpay.Enabled()` config check. This is deliberately stronger than just unlinking the UI: even a direct `POST` to either endpoint gets an immediate `503`, so there's no live paid endpoint reachable in the background for either flow. Flip that one constant back to `false` (and re-link the relevant UI) to restore either.

| Flow | Create endpoint | Verify endpoint | DB table | Currently live? |
|---|---|---|---|---|
| Course purchase | `POST /courses/{slug}/checkout/order` | `.../verify` | `purchases` | **Yes** — real Razorpay when the resolved amount is above $0; **skips Razorpay entirely when it resolves to $0** — see §3 |
| Consultancy hourly | `POST /booking/checkout/order` | `.../verify` | `bookings` | No — always free right now (`models.ConsultancyPriceCentsUSD/INR = 0`, same $0-skip branch as the course flow); the paid `else` branch is additionally gated by `nonCoursePaymentsDisabled` as a second, independent safeguard |
| Architecture Review | `POST /consultancy/audit/checkout/order` | `.../verify` | `audit_requests` | No — gated by `nonCoursePaymentsDisabled`; not linked from `consultancy.html` either, which routes to `/contact` to scope on a call first, same as the retainer |

`core/payments/razorpay.go`: `Razorpay.Enabled()` is `false` whenever `RAZORPAY_KEY_ID`/`RAZORPAY_KEY_SECRET` aren't set — every create-order handler still checks this too (belt-and-suspenders alongside `nonCoursePaymentsDisabled` for the two disabled flows) and returns `503` with a friendly message instead of erroring.

**To add a fourth fixed-price paid product**, copy the Architecture Review pattern (`consultancy.go`'s `AuditCheckoutOrder`/`AuditCheckoutVerify` + `audit_requests` table) rather than the course or booking pattern — it's the simplest of the three (no pay-what-you-want, no slot reservation). The pattern is still the right reference even though this particular instance is currently disabled.

---

## 7. Templates / rendering (`core/view`)

`view.New()` parses every page template **twice-composed**: once against the public layout (`base.html` + `header.html` + `footer.html`) for public pages, once against the admin layout (`admin_base.html` + `adminnav.html`) for admin pages — two explicit lists in `view.go`, so **a new page template must be added to one of those two lists** or `Renderer.Render` will 500 with "template not found". `Renderer.Render(w, status, "page_name", data)` executes the matched layout's entrypoint template, passing `data` as `.`.

Custom template functions (`view.go`'s `funcs` map): `currentYear`, `formatDate`/`formatDateOptional`/`formatTime`/`formatDateTime`, `money`, `add1`, `splitLines` (blank-line paragraph splitting), and `articleBody` (the HTML-or-plain-text blog body renderer — HTML only if the trimmed content *starts* with `<`, otherwise falls through to escaped `splitLines`-style paragraphs; see the doc comment in `view.go` for why prefix-only detection matters).

Every page's `*Data` struct embeds `handlers.BaseData` (site name, title, meta description, canonical URL, nav links, login/admin state, notifications, `AssetVersion`). A page that needs JSON-LD defines a `{{define "jsonld"}}` block that `base.html` calls via `{{block "jsonld" .}}{{end}}`.

---

## 8. Domain routing (`core/middleware/domain.go`)

Optional (`DOMAIN` env var) subdomain-per-page-group scheme: `about.`, `courses.`, `consultancy.`/`booking.`, `articles.`, `admin.` each own their path prefix; anything unmatched (home, search, legal, static) lives on `home.`. A request on the wrong host (apex, `www`, or another page-group's subdomain) 307-redirects to the correct one; an unrecognized host (e.g. a raw `*.vercel.app` preview URL) is served as-is rather than redirected. `SubdomainForPath(path)` is the same lookup, exposed for building absolute URLs (sitemap, canonical tags) for *any* path from *any* request. With `DOMAIN` unset, this middleware is a no-op passthrough — the whole site works fine as a single flat host too.

---

## 9. Deployment

- **Vercel**, one Go serverless function. `api/index.go`'s `Handler` calls `app.Bootstrap()` exactly once per cold start (`sync.Once`) and reuses the resulting `*handlers.App` (including its DB pool) across warm invocations — never re-bootstrapped per-request.
- **`app.Bootstrap()`** (`core/app/bootstrap.go`): loads `config.Load()` (fails fast if `DATABASE_URL`/`JWT_SECRET` are unset), opens the Postgres pool, **runs pending migrations** (`db.Migrate` — tracked in a `schema_migrations` table by filename, so it's safe to run on every cold start; only un-applied files execute), builds the `view.Renderer`, constructs `handlers.App`, and — if `ADMIN_BOOTSTRAP_EMAIL`/`ADMIN_BOOTSTRAP_PASSWORD` are set and no admin exists yet — creates the first admin account (a no-op once any admin exists, safe to leave the env vars set permanently).
- **Static assets and templates are compiled into the binary** via `go:embed` (`web/embed.go` for CSS/JS/images, `core/view/view.go`'s `templateFS` for HTML, `core/content/embed.go` for the course-content HTML files) — there is no filesystem read at runtime for any of this, which is what makes a single stateless serverless function workable.
- **Database**: Supabase-hosted Postgres. `DATABASE_URL` must point at Supabase's **pooled (Transaction pooler) connection string**, not the direct connection — a serverless function opening/closing many short-lived connections against the direct connection exhausts Postgres's connection limit fast.
- **Local dev**: `cmd/server/main.go` calls the same `app.Bootstrap()` and just runs `http.ListenAndServe` — no Vercel-specific code path exists outside `api/index.go` itself.

---

## 10. Security posture

- CSP + `X-Content-Type-Options: nosniff` + `X-Frame-Options: DENY` + `Referrer-Policy: strict-origin-when-cross-origin` on every response (`middleware.SecurityHeaders`). CSP allows `'unsafe-inline'` for script/style (templates and embedded course-content pages both rely on inline blocks) but restricts *external* origins to only Razorpay's checkout domain and Google Fonts.
- All payment confirmation is server-side HMAC verification (`Razorpay.VerifySignature`) — never trusts a client-reported "payment succeeded."
- Admin and user sessions are fully independent JWTs in `HttpOnly`, `SameSite=Lax` cookies, `Secure` in production.
- Course-content access is an opaque random 32-byte hex bearer token, not a guessable ID.
- `/admin/*` is `noindex`ed at the HTTP-header level (`X-Robots-Tag`), not just via `robots.txt`.

---

*Last updated: September 2026. Companion doc: `docs/FUNCTIONALITY.md`. If you're an AI agent about to modify this codebase, read both files first — they're written specifically so you don't have to re-derive this context from the source tree.*
