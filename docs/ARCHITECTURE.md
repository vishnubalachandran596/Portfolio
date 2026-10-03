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
| Migrations | Custom runner, `core/db/db.go` | Numbered `.sql` files in `core/db/migrations/`, tracked in a `schema_migrations` table, applied on every cold start. These carry **schema only** — no content `INSERT` or content `UPDATE`, including the `profile` identity row. Content lives in the live database; an idempotent snapshot is kept in the Google Drive folder named `Portfolio` (see §11) |
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
  storage/      private Supabase Storage client for lesson HTML, article HTML, and role-resume PDFs
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
    articles ||--o{ article_contributions : "receives"
    users ||--o{ article_contributions : "pays (nullable FK — login or guest-by-email; same user may have many rows)"

    admins {
        serial id PK
        text email UK
        text password_hash
        text name
        timestamptz created_at
    }
    users {
        serial id PK
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
        text category UK
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
        note "UNIQUE (company, role, start_date)"
    }
    role_resumes {
        text role_key PK
        text label
        text object_path "path in the resumes bucket"
        int sort_order
        timestamptz created_at
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
        note "UNIQUE (institution, degree, field)"
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
    certifications {
        serial id PK
        text name
        text issuer
        date issued_on "nullable, NULL = no date on record"
        date expires_on "nullable, NULL = does not expire"
        text credential_url
        bool is_published
        int sort_order
        timestamptz created_at
        note "UNIQUE NULLS NOT DISTINCT (name, issuer, issued_on)"
    }
    publications {
        serial id PK
        text title
        text kind "free text: Conference talk, Podcast, …"
        text venue
        text url
        date published_on "nullable"
        text summary
        bool is_published
        int sort_order
        timestamptz created_at
        note "UNIQUE NULLS NOT DISTINCT (title, venue, published_on)"
    }
    case_studies {
        bigserial id PK
        text title
        text client_context "e.g. employer name — Vishnu's own, not third-party PII"
        text problem
        text approach
        text result
        bool is_published
        int sort_order
        timestamptz created_at
        note "UNIQUE (title); ships empty like certifications/publications — migration 060"
    }
    subscribers {
        bigserial id PK
        text email UK
        text source_page "home | courses | articles | other"
        timestamptz created_at
        note "pure visitor PII, never in the bootstrap-data snapshot — migration 061"
    }
    home_features {
        serial id PK
        text title UK
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
        int price_cents "INR paise"
        text image_url
        bool is_published
        int sort_order
        bool is_paid
        int min_price_cents "INR paise, 0 = free -- courses sell in INR only since Oct 2026, migration 062"
        text content_key
        text content_html "legacy column, left empty"
        text content_object "private Storage path courses/{slug}.html"
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
        note "UNIQUE (course_id, question)"
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
        serial id PK
        bigint course_id FK
        bigint user_id FK "nullable column, but always populated since Oct 2026 — checkout requires a Google-authenticated account; ON DELETE SET NULL"
        text email
        int amount_cents
        text currency
        text status "created | paid | failed"
        text razorpay_order_id UK
        text razorpay_payment_id
        text access_token UK "bearer token unlocking /learn"
        bigint promo_code_id FK "nullable"
        text promo_code
        int discount_cents
        timestamptz created_at
        timestamptz paid_at
        note "CHECK (status <> paid OR paid_at IS NOT NULL)"
    }
    bookings {
        serial id PK
        bigint user_id FK
        text name
        text email
        text phone
        text message
        timestamptz slot_start UK
        timestamptz slot_end
        text status "pending_payment | confirmed | cancelled"
        int amount_cents
        text currency
        text razorpay_order_id "partial UNIQUE where <> ''"
        text razorpay_payment_id
        timestamptz previous_slot_start "nullable"
        timestamptz rescheduled_at "nullable"
        bool reschedule_acknowledged
        bigint promo_code_id FK "nullable"
        text promo_code
        int discount_cents
        timestamptz created_at
        note "amount_cents/currency/discount_cents/promo_code* are always 0/empty now — hourly consultancy has no payment step; razorpay_order_id is only a unique token for MarkPaid to scope its update to, never a real Razorpay reference"
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
        note "Architecture Review — read-only now, historical rows only, self-checkout removed"
    }
    contact_messages {
        bigserial id PK
        text name
        text email
        text phone "optional"
        text message
        bool is_read
        timestamptz created_at
        note "one /contact form submission — no login, no FK, standalone lead capture"
    }
    availability_rules {
        serial id PK
        smallint weekday "0-6"
        time start_time
        time end_time
        int slot_minutes
        bool active
        timestamptz created_at
        note "UNIQUE (weekday, start_time, end_time)"
    }
    articles {
        serial id PK
        text slug UK
        text title
        text summary
        text body "the post itself — plain text or HTML; empty when content_object is set"
        text content_object "private Storage path articles/{slug}.html; empty means body is the post"
        text cover_image_url
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
        int max_redemptions "0 = unlimited, total across every account"
        int max_redemptions_per_user "0 = unlimited; configurable per code, migration 062"
        int redemption_count
        timestamptz starts_at "migration 062"
        timestamptz expires_at
        bool is_active
        bool is_public "scrolls in the sitewide promo banner; migration 062"
        text allowed_emails "comma-separated; non-empty makes the code private/user-scoped and forces is_public false; migration 062"
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
- `audit_requests` is now read-only from the app's perspective — nothing writes new rows anymore since the Architecture Review's self-checkout was removed (see §6). The table and its historical rows are kept purely so the admin payments ledger still shows what was actually paid before the removal.
- `course_updates` has no Go-side fan-out logic — a **Postgres trigger** (`trg_course_updates_notify`, from migration `026_course_updates.sql`) does the INSERT into `admin_messages` for every buyer automatically. Inserting a `course_updates` row (from the admin course-edit form's "what changed" field, or directly via SQL) is the *entire* notification mechanism.
- `min_price_cents` on `courses` defaults to `0` at the column level (migration `030`), but every published course has carried a real, industry-level INR minimum (content, set via the admin course form) since Oct 2026 — the free-by-default "contribute as you like" state from Sept 2026 was retired in favor of real pricing plus whichever promo code is currently active as the free-access path. `CheckoutCreateOrder` (`core/handlers/checkout.go`) now requires a Google-authenticated session unconditionally — there is no guest-checkout branch left.
- **Multi-currency is retired (migration `062`, Oct 2026).** Every price on this site is INR only — `models.CurrencyINR`. There is no more visitor-region detection (`core/geo` and `X-Vercel-IP-Country` are gone entirely), no `courses.currency` or `courses.min_price_cents_inr` column, no `CHECKOUT_CURRENCY` env var, and no `Course.RegionalPrice`/`RegionalPriceDisplay` — `Course.PriceDisplay`/`MinPriceDisplay` render a plain INR figure unconditionally. Article contributions and the consultancy reference figures (`AuditPriceDisplay`, `RetainerRangeDisplay`) are INR-only the same way. Razorpay is still created with `models.CurrencyINR` explicitly passed through, rather than relying on the account's default.
- **Promo codes got three new scoping dimensions in migration `062`, all configurable per code from `/admin/promo-codes`:** a per-account redemption cap (`max_redemptions_per_user`, 0 = unlimited, 1 by default — independent of `max_redemptions`, the old total-across-everyone cap, both enforced in `resolvePromoCode` via `PurchaseRepo.CountUserRedemptions`); a start time (`starts_at`) alongside the existing `expires_at`; and a public/private split — `is_public` codes scroll in the sitewide promo banner (`PromoCodeRepo.ListActivePublic`, fetched concurrently in `startBaseChrome` on every single page load and rendered by the `promo_banner.html` partial in `base.html`, just below the header, on every page), while a code with any `allowed_emails` listed (comma-separated text, parsed in the repo layer, not a join table — it's a short, rarely-edited list) is always private regardless of the `is_public` checkbox: `AdminPromoSave` forces `IsPublic = false` the moment any email is present. `PromoCode.AppliesToAllCourses` is gone — an empty `promo_code_courses` join for a code now means "applies to any course" by itself, with no separate flag that could drift from it.
- Natural-key uniqueness (migrations `050` and `054`): `skill_groups.category`, `experiences (company, role, start_date)`, `education (institution, degree, field)`, `home_features.title`, `availability_rules (weekday, start_time, end_time)`, `quiz_questions (course_id, question)`, `articles.slug` (promoted from the existing unique index), and `UNIQUE NULLS NOT DISTINCT` on `certifications (name, issuer, issued_on)` and `publications (title, venue, published_on)`. Duplicate rows are cleaned in the same migration before the constraint is added, so a second cold start is a no-op.
- RLS with zero policies covers every public table, including `certifications` and `publications` (migration `051`) and `role_resumes` (migration `058`). The app connects as the table owner and bypasses RLS; this only closes the PostgREST `anon`/`authenticated` door.

---

## 4. `core/handlers` — the App struct and request flow

`handlers.App` (in `core/handlers/app.go`) is the one struct every handler is a method on. It holds:
- `Cfg *config.Config`, `DB *sql.DB`, `View *view.Renderer`, `Auth *middleware.Auth`
- One repo field per aggregate: `Profile, Skills, Experience, Resumes, Education, Features, Courses, Bookings, Availability, Admins, Purchases, Users, Articles, PromoCodes, QuizQuestions, QuizAttempts, AdminMessages, CourseUpdates, Testimonials, Certifications, Publications, CaseStudies, AuditRequests, ArticleVotes, ArticleContributions, ContactMessages, Subscribers`, plus `CourseStorage` for the private buckets
- `Razorpay *payments.Razorpay`, `Telegram *notify.Telegram`, `Google *oauth.Google`, `Timezone *time.Location`

Shared page chrome is assembled by `base` / `startBase` / `baseFrom` in `app.go`. `BaseData` carries site name, title, meta description, canonical URL, nav links, login/admin state, the footer's LinkedIn link, notifications, and `AssetVersion`. Every page-specific `*Data` struct embeds it.

Chrome lookups (the profile row, for the LinkedIn link; plus, for a signed-in visitor, user name and notifications) stay on their own `sync.WaitGroup` on purpose: that contract is "degrade quietly" — a missing LinkedIn link or an empty notification menu is a fine outcome for a page that isn't about either. Page-owned lookups use `errgroup` and fail the render. Folding chrome into the errgroup would silently promote a swallowed error into a 500; the inverse would swallow a missing About section.

Those chrome lookups used to run as a **second sequential wave** after each page's own `errgroup.Wait()`, and pages that already loaded `profile` fetched it again. On the public hot path the handler now calls `startBase` (or `startBaseWithProfile` when it already loads that row) *before* its own lookups and finishes with `baseFrom` after them, so chrome rides in the same round-trip wave. `base()` remains the one-shot form for admin pages, error pages, and form re-renders that have no wave of their own to join.

Anonymous `/about` is therefore one wave of eight errgroup lookups. Anonymous `/` is one wave of four. `/articles` is one wave of the filtered page (count + rows, themselves concurrent under an errgroup inside `ArticleRepo.ListPublishedFiltered`), the topic list, and chrome. List queries select `articleListColumns` — everything except `body`, which only the article's own page needs. `/courses` overlaps chrome with the course list. `/resume.pdf` has no chrome. A role that has its own file is one lookup plus the Storage read. Every other request loads the default row (`role_key` `default`) and reads that object. With no default file stored it is one wave of four and generates a PDF. The Person schema's `knowsAbout` is derived from the skill groups the home page has already loaded, not a separately maintained keyword list.

`App.Routes()` (in `routes.go`) builds the full `http.Handler`: one public `ServeMux`, one admin-auth `ServeMux`, one `RequireAdmin`-wrapped protected `ServeMux` for the rest of `/admin/`, composed and wrapped in `middleware.Logging(middleware.SecurityHeaders(middleware.CanonicalHost(siteURL)(middleware.DomainRouter(domain)(mux))))`. Public utility and gated routes that can render HTML but are not Google landing pages (`/search`, `/login`, `/booking`, `/account`, admin, and gated course learn/quiz/certificate pages) are wrapped with the shared `noindex` helper rather than added to the sitemap. See `docs/FUNCTIONALITY.md` for what each route does; the exact route table (method + path + handler) is the source of truth in `routes.go` itself — don't let this doc's prose drift from it, read `routes.go` directly for the literal list.

Handler files, by area:
| File | Covers |
|---|---|
| `public.go` | Home (incl. subscribe-form data), About (incl. case studies), Resume, CoursesList (incl. subscribe-form data), CourseDetail, Sitemap-adjacent data |
| `checkout.go` | Course purchase order/verify, `courseAccess()` gate |
| `consultancy.go` | Consultancy page (tiers, FAQ, case studies, JSON-LD), Booking page/confirm (free, no payment gateway), Contact page/submit |
| `admin_contact.go` | Admin contact-messages list + mark-read |
| `admin_credentials.go` | Admin CRUD for Certifications and Speaking & Publications |
| `admin_case_studies.go` | Admin CRUD for Case Studies |
| `newsletter.go` | Public `POST /subscribe` (capture only, no confirmation email) + admin subscribers list/delete |
| `articles.go` | Public article list (incl. subscribe-form data)/detail/thumbs-up + Admin article CRUD |
| `account.go` | `/account` page + PDF receipts + testimonial submission |
| `booking_reschedule.go` | Both admin- and self-service booking reschedule flows |
| `learn.go` | Gated course content serving inside the site chrome + watermark injection |
| `quiz.go` | Quiz taking, scoring, certificate download |
| `auth.go` | Login page, Google OAuth start/callback, logout |
| `legal.go` | Generic static legal-page handler factory |
| `seo.go` | `baseURL`/`pageURL` helpers, Sitemap, Robots, LLMsTxt, IndexNow key + submit, Search |
| `schema.go` | JSON-LD builders (Person/Website/Service on Home; ProfessionalService + FAQPage on Consultancy; Article; Course) |
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

## 6. Payments — Razorpay is wired up for courses only

**Course purchases are the only paid flow left on this site.** Consultancy (Architecture Review, retainer, hourly) never touches a payment gateway at all — no code path, no dormant endpoint, nothing to re-enable by flipping a flag. This was a deliberate removal, not a disable: the site owner isn't taking payment integration further any time soon, and having a live-but-unlinked paid endpoint sitting in the background was considered worse than just deleting it. If self-checkout for consultancy is ever wanted again, it needs to be rebuilt — `git log` on `consultancy.go`/`booking.html` around the removal commit is the reference for what the Razorpay version looked like.

**Course purchase** (`POST /courses/{slug}/checkout/order` → `.../verify`, `purchases` table) follows: **create-order handler** (server resolves the true price, creates a Razorpay order, writes a `pending` DB row) → client opens Razorpay's Checkout.js modal with that order ID → **verify handler** (HMAC-verifies the callback server-side via `Razorpay.VerifySignature`, flips the DB row to paid, fires a Telegram notification, returns a redirect). Access is **only ever granted from the verify step**, never trusted from the client-side "success" callback alone. It **skips Razorpay entirely when the resolved amount is $0** — see §3.

**Consultancy hourly** (`POST /booking/confirm`, `bookings` table) is a plain "reserve the slot, confirm it" flow with no price and no payment step of any kind — `BookingConfirm` in `consultancy.go` calls `BookingRepo.CreatePending` then `MarkPaid` back to back, purely to reuse the existing slot-race protection (the unique constraint on `slot_start`), not because anything is actually "paid." A synthetic `razorpay_order_id` (`"free-"+token`) only exists so `MarkPaid` has something unambiguous to scope its update to.

**Architecture Review and Fractional Advisory** have no create/verify endpoints at all — both route to `/contact` from `consultancy.html`, to be scoped and invoiced outside this site. The `audit_requests` table and `AuditRequestRepo.ListAll` still exist, read-only, purely so the admin payments ledger (`admin_payments.go`) can keep showing the real historical Architecture Review purchases from when self-checkout briefly existed; nothing writes to that table anymore.

`core/payments/razorpay.go`: `Razorpay.Enabled()` is `false` whenever `RAZORPAY_KEY_ID`/`RAZORPAY_KEY_SECRET` aren't set — the course create-order handler checks this (except its own $0 branch, which doesn't need Razorpay at all) and returns `503` with a friendly message instead of erroring.

**To add a new fixed-price paid product**, the course purchase flow (`checkout.go`'s `CheckoutCreateOrder`/`CheckoutVerify` + `purchases` table) is now the only live reference implementation for a Razorpay create/verify pair in this codebase.

---

## 7. Templates / rendering (`core/view`)

`view.New()` parses every page template **twice-composed**: once against the public layout (`base.html` + `header.html` + `footer.html` + `skills_grid.html`) for public pages, once against the admin layout (`admin_base.html` + `adminnav.html`) for admin pages — two explicit lists in `view.go`, so **a new page template must be added to one of those two lists** or `Renderer.Render` will 500 with "template not found". `Renderer.Render(w, status, "page_name", data)` executes the matched layout's entrypoint template, passing `data` as `.`.

`publicLayout` also carries **shared component partials** — templates more than one page renders, kept in one place rather than copy-pasted. `promo_banner.html` (`{{define "promo_banner"}}`, reads `.PromoCodes` off whatever `BaseData` it's handed) is called exactly once, from `base.html` itself right after the header — so it renders on *every* public page, not per-page — and lists every currently active, public promo code (`PromoCodeRepo.ListActivePublic`, fetched concurrently in `startBaseChrome`); empty renders nothing. A pure-CSS marquee (`.promo-banner-track`, `@keyframes promo-banner-scroll` in `style.css`), disabled under `prefers-reduced-motion`. `skills_grid.html` (`{{define "skills_grid"}}`, taking a `[]models.SkillGroup` as its dot) is the first: Home's "What I bring" and About's "Skills" both call `{{template "skills_grid" .Skills}}`, which is what stops the two grids from drifting apart the way two copies of the same markup did before. Add a new shared component to that list too. Its interaction is CSS-first — see the `.skills-grid` comment block in `web/static/css/style.css`. There are **two display modes with different layout mechanics**, which is the one non-obvious thing about this component: collapsed, the card face shows category, level, and four chips (plus "and N more") and a revealed panel is `position: absolute` over its own card (achievement summary plus the full tag list), and being out of normal flow is exactly what stops the grid reflowing and keeps the summary out of the resting layout; expanded ("Expand all"), the `.is-expanded-all` modifier returns every panel to `position: static` so the cards size to their content and CSS Grid equalises each row. The second mode exists because the first cannot scale to nine simultaneous panels — nine absolutely-positioned boxes overlap their neighbours and overflow the grid. `main.js` owns the mode toggle, the per-card click/tap toggle (which it suppresses while in expanded mode, so `aria-expanded` never disagrees with what is on screen), and the live-region announcement.

Custom template functions (`view.go`'s `funcs` map): `currentYear`, `formatDate`/`formatDateOptional`/`formatTime`/`formatDateTime`, `money`, `add1`, `splitLines` (blank-line paragraph splitting), and `articleBody` (the HTML-or-plain-text blog body renderer — HTML only if the trimmed content *starts* with `<`, otherwise falls through to escaped `splitLines`-style paragraphs; see the doc comment in `view.go` for why prefix-only detection matters).

Every page's `*Data` struct embeds `handlers.BaseData` (site name, title, meta description, canonical URL, nav links, login/admin state, notifications, `AssetVersion`). A page that needs JSON-LD defines a `{{define "jsonld"}}` block that `base.html` calls via `{{block "jsonld" .}}{{end}}`.

---

## 8. Domain routing (`core/middleware/domain.go`, `canonical.go`)

The public canonical host is `www` (`SITE_URL`, currently `https://www.vishnubalachandran.com`). `CanonicalHost` 301-redirects the bare apex and the old `home.` landing host to that www origin with the same path; the www host itself is **never** redirected (a canonical URL that 3xxs is Google's "Page with redirect" indexing failure). Preview hosts (`*.vercel.app`) and local dev are served as-is. `vercel.json` repeats the apex/`home.` → www 301 at the edge so HTTP apex is not a two-step chain (HTTP→HTTPS apex, then apex→www).

Optional (`DOMAIN` env var) subdomain-per-page-group scheme: `about.`, `courses.`, `consultancy.`/`booking.`, `articles.`, `admin.` each own their path prefix on their own subdomain. Unmatched paths (home, search, legal, static) live on **www**, not `home.`. A request on www is always served — including page-group paths — so the live path-based site stays indexable. Apex and `home.` 301 to www; other page-group subdomains still 301 to the subdomain that owns the path. An unrecognized host is served as-is. `SubdomainForPath(path)` is the same lookup, exposed for building absolute URLs; `"home"` resolves to the www host in `pageURL`. With `DOMAIN` unset, `DomainRouter` is a no-op and `CanonicalHost` still consolidates apex/`home.` onto www whenever `SITE_URL` prefers www.

---

## 9. Deployment

- **Vercel**, one Go serverless function. `api/index.go`'s `Handler` calls `app.Bootstrap()` exactly once per cold start (`sync.Once`) and reuses the resulting `*handlers.App` (including its DB pool) across warm invocations — never re-bootstrapped per-request.
- **`app.Bootstrap()`** (`core/app/bootstrap.go`): loads `config.Load()` (fails fast if `DATABASE_URL`/`JWT_SECRET` are unset), opens the Postgres pool, **runs pending migrations** (`db.Migrate` — tracked in a `schema_migrations` table by filename, so it's safe to run on every cold start; only un-applied files execute), builds the `view.Renderer`, constructs `handlers.App`, and — if `ADMIN_BOOTSTRAP_EMAIL`/`ADMIN_BOOTSTRAP_PASSWORD` are set and no admin exists yet — creates the first admin account (a no-op once any admin exists, safe to leave the env vars set permanently).
- **Static assets and templates are compiled into the binary** via `go:embed` (`web/embed.go` for CSS/JS/images, `core/view/view.go`'s `templateFS` for HTML) — there is no filesystem read at runtime for any of this, which is what makes a single stateless serverless function workable. Paid lesson HTML is not in the binary. `core/storage` reads it from the private `course-content` bucket after `courseAccess` passes (`SUPABASE_SECRET_KEY`, or the anon key plus the dedicated storage-reader login) and `PrepareLesson` returns a base64 iframe document: the lesson body, the lesson's own `<style>` (copied out of `<head>` so labs, compilers, and animations still run), the site stylesheet, a small reset so the site-wide `ul { list-style: none }` and any collapsed `word-spacing` cannot flatten lesson prose, and the watermark. `/courses/{slug}/learn` renders the site chrome and loads that document in a 75vw × 75dvh `.html-renderer` iframe (`sandbox="allow-scripts"`, no `allow-same-origin`) so the parent cannot read the lesson DOM. The watermark paints only inside that pane. The same login reads article HTML from the private `articles` bucket (`articles/{slug}.html`) and role-resume PDFs from the private `resumes` bucket (`resumes/{role_key}.pdf`). The article page reads that object and renders it in the same pane (in-page, for indexing), with the watermark mounted on `#html-renderer` so it does not cover the site chrome. Uploaded article `<style>` is still stripped so a post cannot restyle the public chrome. When `LOCAL_CONTENT_DIR` is set, `storage.New` uses that directory for the same object paths and does not call Supabase. Production leaves the variable unset. Admin preview uses that same reader after `RequireAdmin`: `GET /admin/resumes/{role_key}` streams the stored PDF, `GET /admin/articles/{id}/preview` renders the stored post (or `articles.body` when no file is set) in the article page, and `GET /admin/courses/{id}/preview` writes the stored lesson HTML. The course preview does not run `PrepareLesson`. The public learn route still does. None of these responses include a Storage URL. A warm instance may keep that login's access token in memory; dropping it only causes the next Storage request to sign in again. The token is not a content cache.
- **Database**: production is Supabase-hosted Postgres. On Vercel, `DATABASE_URL` must point at Supabase's **pooled (Transaction pooler) connection string**, not the direct connection — a serverless function opening/closing many short-lived connections against the direct connection exhausts Postgres's connection limit fast. `db.Open` logs a warning when the host looks like Supabase's direct connection (`*.supabase.co` without `pooler` in the hostname) rather than failing, so a plain local Postgres still boots. Local Compose uses a direct connection on purpose: one long-lived `cmd/server` process, not a fleet of serverless instances.
- **Connection pool**: a Vercel Go instance serves one request at a time, so the pool only has to cover the widest concurrent fan-out inside a single request — About's eight page lookups plus the two chrome lookups a signed-in visitor adds, currently `widestRequestFanOut+1` = 11 for both `MaxOpenConns` and `MaxIdleConns`. Matching idle to the ceiling means a warm instance does not re-establish connections on the widest page. If the fan-out grows, this constant has to grow with it, or one concurrent wave quietly becomes two.
- **Migrations**: `db.Migrate` reads `schema_migrations` once per cold start into a set, then skips already-applied files in memory. The previous per-file `SELECT count(*)` grew by one round trip with every migration added.
- **Local dev**: `docker compose up --build` is the local stack on Docker Desktop for Windows and for Mac (same Compose file; host paths use forward slashes). `docker-compose.yml` starts Postgres 16 (`db`, user/password/database `portfolio`) and the image built from `Dockerfile` (`cmd/server`). Copy `.env.example` to `.env` first. `DATABASE_URL` there is `postgres://portfolio:portfolio@db:5432/portfolio?sslmode=disable`: host `db` is the Compose service name, and the user, password, and database match `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`. Those four have to change together. Compose publishes Postgres on host port `POSTGRES_PORT` (default `5432`) and the app on `PORT` (default `8080`). `cmd/server/main.go` calls the same `app.Bootstrap()` as Vercel and runs `http.ListenAndServe`. Running `go run ./cmd/server` on the host against that database uses `localhost` in place of `db`. A missing `profile` row is an empty profile, not a 500; **Admin → Profile** inserts it on save. Migrations still insert no content. `docker/local-admin.sql` is the local placeholder admin (`admin@example.com` / `change-this-password`) and is not part of `Migrate()`. Apply it after the schema exists, or leave `ADMIN_BOOTSTRAP_EMAIL` and `ADMIN_BOOTSTRAP_PASSWORD` set so the first start creates the same account when `admins` is empty. Compose bind-mounts `LOCAL_CONTENT_DIR` (default `../portfolio-local-content`, outside the repo) at `/content` and sets `LOCAL_CONTENT_DIR=/content` in the app container. Files there are `articles/{slug}.html`, `courses/{slug}.html`, `resumes/default.pdf`, and `resumes/{role-key}.pdf`. `.env.example` sets `SITE_URL=http://localhost:8080` and `GOOGLE_REDIRECT_URL=http://localhost:8080/login/google/callback` (a placeholder; register it on the OAuth client and paste the client id and secret). The entrypoint script is LF, and the image strips a Windows CR before it runs.

---

## 10. Security posture

- CSP + `X-Content-Type-Options: nosniff` + `X-Frame-Options: DENY` + `Referrer-Policy: strict-origin-when-cross-origin` on every response (`middleware.SecurityHeaders`). CSP allows `'unsafe-inline'` for script/style (templates and embedded course-content pages both rely on inline blocks) but restricts *external* origins to only Razorpay's checkout domain — fonts are self-hosted (below), so `style-src`/`font-src` need no external allowance at all. `frame-src` allows `'self'` and Razorpay Checkout.
- All payment confirmation is server-side HMAC verification (`Razorpay.VerifySignature`) — never trusts a client-reported "payment succeeded."
- Admin and user sessions are fully independent JWTs in `HttpOnly`, `SameSite=Lax` cookies, `Secure` in production.
- Course-content access is an opaque random 32-byte hex bearer token, not a guessable ID. The learn page is the site chrome; the lesson loads from a base64 payload in a unique-origin iframe, so the outer page source does not contain the lesson tags and the parent cannot read the lesson DOM. `vishnubalachandran.com` is drawn three times along the pane diagonal, corner to corner, only inside the renderer; copied text is stamped with the viewer's identity.
- `/admin/*` is `noindex`ed at the HTTP-header level (`X-Robots-Tag`), not just via `robots.txt`.

---

## 11. Content vs schema — disaster recovery

The repo is **code only**. Migrations create tables, indexes, constraints and functions. They do not insert Vishnu's profile, skills, articles, courses, or anything else a visitor would read. UI chrome (headings, nav, button labels) stays in templates and handlers; that is not content.

**Live Postgres is the source of truth for content.** Edits happen in the admin console (`/admin/*`). After a content change, regenerate the snapshot from the live database:

```
psql "$DATABASE_URL" -AtX -v ON_ERROR_STOP=1 \
     -f core/db/gen_bootstrap_data.sql > portfolio_seed_data.sql
```

The output is stored in a Google Drive folder named **`Portfolio`**. Refer to that folder by name only — never record a Drive URL in this repo. The generator (`core/db/gen_bootstrap_data.sql`) stays in the repo because it is code; the snapshot does not.

**Direction of truth.** Edit in admin → regenerate the snapshot. The snapshot is a bootstrap copy, not an authoring surface. Editing the SQL file and expecting the site to change is backwards.

**Disaster-recovery trade-off, stated plainly.** Checking out this repo and running migrations produces a complete, empty schema and a site that renders with no name, no skills, no articles. The repo **cannot rebuild content**. Recovering a wiped database means: apply the migration chain, then load `portfolio_seed_data.sql` from the `Portfolio` Drive folder (or restore a database backup). The first admin account is created by `Bootstrap()` from `ADMIN_BOOTSTRAP_EMAIL` / `ADMIN_BOOTSTRAP_PASSWORD`, not from the snapshot — the snapshot never contains password hashes. Local Docker can also load `docker/local-admin.sql`, which inserts only the placeholder admin from `.env.example` (`admin@example.com` / `change-this-password`). That file is not the snapshot and is not a migration. The article section of the snapshot includes `content_object` so a restored row still points at `articles/{slug}.html`.

Paid lesson HTML is not in the snapshot either. `courses.content_object` is the path (`courses/{slug}.html`) inside the private Storage bucket `course-content`. Article HTML uploaded from the admin console is a separate private bucket, `articles` (`articles.content_object`, path `articles/{slug}.html`). Role-resume PDFs are a third private bucket, `resumes`. The default download is the row `role_key = default` at `resumes/default.pdf`. A role-specific file is any other row, at `resumes/{role_key}.pdf`. The bytes live only in those buckets. Restoring the SQL without the bucket objects leaves every learn page, any uploaded article, and any resume download with a path and no file. Posts that still have `articles.body` set and an empty `content_object` restore with the SQL alone. A missing default resume row falls back to the generated PDF. Migration 055 creates `course-content` and a policy for `course-content@vishnubalachandran.com` when the `storage` schema exists; it does not create that login or upload the files. Migration 056 adds `articles.content_object`. Migration 057 creates the `articles` bucket and the same login's policy on it. Migration 058 creates `role_resumes` (with row-level security and no content rows) and the `resumes` bucket with that same login's policy.

`Migrate()` only executes files that exist and are not yet recorded in `schema_migrations`. It never re-reads an already-applied file, and it never checks that a recorded filename still has a matching file on disk. Editing or deleting a historical migration is therefore a no-op against production once that version is recorded; it only changes what a fresh database gets.

---

*Last updated: September 2026. Companion doc: `docs/FUNCTIONALITY.md`. If you're an AI agent about to modify this codebase, read both files first — they're written specifically so you don't have to re-derive this context from the source tree.*
