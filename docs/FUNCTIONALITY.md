# Functionality Reference — vishnubalachandran.com

**Purpose of this document:** a complete, current inventory of every feature on this site — what a visitor, a logged-in user, and an admin can each do, and the business rules behind each flow. Written so a fresh AI agent (or a new human contributor) can read this one file and understand *what the product does* without having to read every handler first. For *how it's built* (packages, schema, request flow, deployment), see `docs/ARCHITECTURE.md`.

**Keep this updated.** Whenever a route, model field, or user-facing flow changes, update the relevant section here in the same commit — this document is only useful if it stays true.

---

## 1. Site identity

A personal site for Vishnu Balachandran — technical consultant, Java/event-driven-systems architect, 8+ years in banking/fintech (Zafin: pricing & billing platforms). Three revenue lines: **consultancy** (Architecture Review / retainer / hourly), **courses** (contribute-as-you-like), and (implicitly) **inbound credibility** from **articles/blog**. Built as a single Go binary, server-rendered, no JS framework, deployed as one Vercel serverless function, Postgres (Supabase) as the only datastore.

Domain: `vishnubalachandran.com`, canonical host `www.vishnubalachandran.com`. A page-group subdomain scheme also exists (`about.`, `courses.`, `consultancy.`, `articles.`, `contact.`, `admin.`, with `home.` as the landing/catch-all) — see `core/middleware/domain.go` — active whenever the `DOMAIN` env var is set; a bare path-based site otherwise.

---

## 2. Public (unauthenticated visitor) functionality

### Home (`GET /`)
Hero (headshot from `Profile.PhotoURL`, name, title, subtitle), a skills grid pulled from `skill_groups` (category/level/achievement/tags), a CTA row into Consultancy/Courses, and a "Giving back" teaser of the latest published blog posts linking to `/articles`.

### About (`GET /about`)
Full bio, headshot, skill groups, work experience, education, a downloadable PDF resume (`GET /resume.pdf`, generated in-process from live profile/skills/experience/education data — never drifts from what's on the page), a GitHub link, and published testimonials (only ones an admin has approved — see §4).

### Courses list (`GET /courses`)
Every published course as a card: title, level, duration, and a pricing line. **Every course is "contribute as you like"** — `Course.RegionalPriceDisplay()` renders "Contribute as you like to support" when no minimum is set, or the same plus `*min $X` when the course does have a floor (admin-configurable per course, defaults to 0 for all courses as of Sept 2026).

### Course detail (`GET /courses/{slug}`)
Full description, level/duration, and either:
- **No access yet:** a "Get access" box. Email (skipped if logged in — pulled from the session), an optional contribution amount (defaults to the course's minimum, which is usually 0), an optional promo code, and a button that reads "Get Free Access" (amount = 0) or "Continue" (amount > 0).
- **Has access:** "Open Course" + (if the course has a quiz) "Take the Final Quiz".

**Checkout flow (`POST /courses/{slug}/checkout/order` → `POST /courses/{slug}/checkout/verify`):**
1. Server resolves the visitor's region (`core/geo`) and the course's minimum for that region; a client-submitted amount below the minimum is silently floored — never trusted below that floor.
2. A promo code (if any) is validated and its discount subtracted.
3. **If the final amount is $0:** no Razorpay involved at all — a `purchases` row is written straight to `paid` status (with a synthetic `razorpay_order_id` of `"free-"+token`), the access cookie is set immediately, and the response tells the client to redirect straight to `/courses/{slug}/learn`.
4. **If the final amount is > $0:** a real Razorpay order is created, the client opens Razorpay's Checkout.js modal, and only after `/checkout/verify` confirms the HMAC signature server-side does access actually get granted.

Either way, access is proven going forward by an opaque bearer token (`access_token`) set as an `HttpOnly` cookie (`ca_{courseID}`) — the same token also works as a `?token=` query-string "magic link" (e.g. for emailing a buyer their access without them being logged in), and gets re-set as a cookie the first time it's used that way.

### Course content (`GET /courses/{slug}/learn`) — gated
Requires `courseAccess()` to pass: an authenticated admin (unless previewing-as-visitor), or a valid access token. Serves the course's `ContentHTML` (a single self-contained HTML file the admin uploaded — see §4) with a **watermark script injected** identifying the viewer (their email, or "Admin preview") as a copy deterrent. `noindex`ed — never in search results.

### Quiz & certificate (`GET/POST /courses/{slug}/quiz`, `GET /courses/{slug}/certificate`) — gated
If a course has quiz questions, a purchaser (or previewing admin) can take a multiple-choice final quiz. Passing (`QuizPassPercent = 70%`) unlocks a downloadable PDF completion certificate, looked up by a unique `certificate_code` generated on that passing attempt. Re-attempts are allowed; `BestPassingAttempt` is what the certificate download resolves against.

### Consultancy (`GET /consultancy`) — three tiers, no payment gateway anywhere here
1. **Architecture Review** (featured/primary CTA) — scoped on a call, not self-checkout. Shows an indicative reference price ($3,000 USD / ₹1,50,000 INR, region-resolved via `AuditRegionalPriceDisplay`) with a "Discuss an Architecture Review" CTA to `/contact` — that figure is display-only, never charged. A paid self-checkout version of this existed briefly and was fully removed (handlers, routes, JS, Razorpay calls) rather than just disabled, since payment integration isn't being pursued further for now. The `audit_requests` table and `AuditRequestRepo.ListAll` remain, read-only, only so the admin payments ledger can still show the real historical purchases from when it was live — nothing writes to that table anymore.
2. **Fractional Advisory (retainer)** — a *display-only* price range ($4,000–8,000/month), links to `/contact`. Never had self-checkout: retainers are scoped on a call.
3. **Hourly session / "Schedule a Call"** — always free, no price displayed at all, links to `/booking`.

### Booking (`GET /booking`, requires login) — free hourly consultancy call
A calendar of available slots computed from `availability_rules` minus already-booked slots (`core/scheduler`). Pick a slot → confirm (`POST /booking/confirm`) — reserves it and confirms it immediately, no payment step, no Razorpay involved at all. Internally still goes through `BookingRepo.CreatePending` → `MarkPaid` (the same pair a paid booking would have used) purely to reuse the slot-race protection (`status=pending_payment` the moment the slot is reserved, then flipped to `confirmed`); the unique constraint on `slot_start` is the backstop. A Telegram notification fires on confirmation.

### Blog (`GET /articles`, `GET /articles/{slug}` — nav/footer label "Blog")
Vishnu's original writing only — every published row has a `Body`. The schema and admin form still support a **curated link-out** mode (admin pastes a source URL + writes a short take, no `Body` — `IsOriginal()` in `core/models/models.go` returns false for these), but as of the "keep only original blogs" cleanup, all 42 previously-published curated rows were hard-deleted from `articles` (not soft-unpublished) and only original posts remain live. Curated link-outs remain a supported *feature* if the admin wants to use it again, just with zero current rows. The list page supports search (`?q=`), tag filter (`?tag=`), and sort (`newest` / `oldest` / `most thumbs-up`), paginated. Each article has its own permanent URL. A post's body can be **plain text** (blank-line-separated paragraphs, auto-escaped) or **full HTML** (headings, links, lists, images — detected by the body *starting* with a tag) — see `articleBody` in `core/view/view.go`. An optional cover image shows atop the post and as a list thumbnail. Thumbs-up (`POST /articles/{slug}/thumbs-up`): a logged-in visitor votes directly with their real account. A signed-out visitor gets an optional name/email form instead of a login wall — both fields optional, submittable blank. Giving an email reuses or creates a lightweight "guest" account for that email (`users.is_guest = true`, same table as a real account, just never signed in) — email is the unique identity, so a second vote from the same email de-dupes exactly like a logged-in vote (via the `article_thumbs_up(article_id, user_id)` unique constraint). A name with no email (or nothing at all) has no stable identity, so it's stored directly on the vote row (`article_thumbs_up.guest_name`) and never de-dupes — every such submission counts as a new vote, same as the site behaved before any identity capture existed. If a guest's email later signs in for real via Google OAuth, `is_guest` flips to false on that same row (`auth.go`'s `GoogleLoginCallback`) — their vote history carries over, they're just no longer labeled "not signed up." The vote widget appears both above the body and below it, with a "Found this useful? Give it a thumbs up for more of these contents" line near the bottom one — both widgets point at the same vote and stay in sync client-side. The admin article edit page lists every voter: name/email (with a "not signed up" tag for guest accounts), name-only, or "Anonymous visitor" when nothing was given.

### Contact (`GET /contact`)
Email (with copy-to-clipboard), LinkedIn link, and a CTA into Consultancy.

### Search (`GET /search`)
Simple substring search across published courses and a small hardcoded index of static pages.

### Legal pages
`/privacy`, `/terms`, `/refund`, `/cookies` — static content pages, plus a cookie-consent banner (no third-party tracking; only a consent-preference cookie and, once logged in, a session cookie).

### SEO/meta endpoints (all served dynamically from live data, never static files)
- `GET /sitemap.xml` — every static page + all published courses + all published articles, with `lastmod`.
- `GET /robots.txt` — allows everything except `/admin/`, references the sitemap.
- `GET /llms.txt` — a plain-text site summary (profile + courses) for AI answer engines.
- `GET /{indexNowKey}.txt` + IndexNow submission — pushes "this URL changed" directly to Bing/Yandex.
- `GET /favicon.ico`, `/static/img/apple-touch-icon.png`, `/static/img/favicon.svg`.

---

## 3. User (logged-in visitor) functionality

Login is separate from admin auth (see §5 of `ARCHITECTURE.md`) and is **Google-only** (`GET /login` shows a single "Sign in with Google" button → `GET /login/google` → `/login/google/callback`) — no password to set up or lose. A user account exists purely to tie purchases/bookings/testimonials to a person across visits.

### Account page (`GET /account`, login required)
Four sections in one page (`#courses`, `#bookings`, `#payments`, `#messages`), plus a testimonial section:
- **My Courses** — every paid course, linking straight into `/courses/{slug}/learn`.
- **My Bookings** — upcoming/past consultancy hours, status badges, and (for a rescheduled booking the visitor hasn't acknowledged yet) a reschedule notice with "that works for me" / "pick a different time" actions.
- **Payments** — every payment (course or consultancy), each with a **PDF receipt download** (`/account/bookings/{id}/receipt.pdf`, `/account/purchases/{id}/receipt.pdf`, generated in-process, sequential receipt numbers `CNS-…` / `CRS-…`).
- **Messages** — one-off notes an admin sent this specific account (see §4), shown here and in the header's notification bell until read.
- **Share your experience** — submit a testimonial (name, title, quote). **Always created unpublished** (`is_published=false`) regardless of what's submitted — it only appears on `/about` after an admin explicitly approves it. One submission per account; the form shows "pending review" / "live" instead of re-submitting once one exists.

### Booking self-service
- Acknowledge a reschedule (`POST /account/bookings/{id}/acknowledge`).
- Pick a new slot for a booking that's still `pending_payment`-eligible for reschedule (`GET/POST /account/bookings/{id}/reschedule`).

---

## 4. Admin functionality

Admin auth is a **separate** session/cookie from user auth (`portfolio_admin_session` vs `portfolio_session`) — an email can hold both an admin and a user account simultaneously, and logging in as one doesn't imply the other. Every `/admin/*` route is `noindex`ed and behind `RequireAdmin`.

### Dashboard (`GET /admin`)
At-a-glance counts/summary across the site.

### Courses (`/admin/courses`)
Full CRUD. Fields: title, summary, description, level, duration, image URL, published toggle, sort order, "contribute-as-you-like" toggle (`is_paid`) with an optional per-region minimum (USD cents / INR paise — 0 means fully free), and **course content**: upload a single self-contained `.html` file (≤8MB) that becomes `ContentHTML`, served verbatim (plus a watermark) at `/courses/{slug}/learn`. A "what changed" note on save writes a `course_updates` row, which a Postgres trigger automatically fans out to every paid buyer as an `admin_messages` entry — no app-level loop needed.

### Quiz questions (`/admin/courses/{courseID}/quiz`)
Per-course CRUD of multiple-choice questions (4 options, one correct, sort order) — this is what powers §2's quiz/certificate flow.

### Bookings (`/admin/bookings`)
List all bookings, change status, and **admin-initiated reschedule** (`/admin/bookings/{id}/reschedule`) — sets `previous_slot_start`/`rescheduled_at` and flips `reschedule_acknowledged` to false, which is what surfaces the "your booking was rescheduled" notice to the visitor.

### Purchases (`/admin/purchases`) & Payments (`/admin/payments`)
Purchases lists raw course-purchase rows. **Payments is the combined ledger** — every actually-paid course purchase, merged with the historical Architecture Review purchases from before self-checkout was removed there, into one chronological list with a running total per currency (`core/handlers/admin_payments.go`). Consultancy bookings never appear here anymore — they're always free, so there's nothing to list as a payment.

### Users (`/admin/users`)
List all user accounts; per-user detail page (their bookings/purchases/messages); promote/demote a user to/from admin; **send a one-off message** to a specific user (shows on their `/account` page and notification bell).

### Skills / Experience (`/admin/skills`, `/admin/experience`)
CRUD for the About page's skill groups and work-experience entries.

### Testimonials (`/admin/testimonials`)
Full CRUD, plus **Approve** (one click, for a visitor-submitted testimonial still pending) that flips `is_published=true` without needing to open the full edit form. The list distinguishes admin-authored vs. visitor-submitted entries.

### Promo codes (`/admin/promo-codes`)
CRUD for percentage-off codes, each scoped to exactly one product (`course` or `consultancy`), optionally to specific courses (`promo_code_courses` join table) or all of them, with an expiry, a redemption cap (0 = unlimited), and an active toggle. Redeemed at course/booking checkout via `resolvePromoCode()`.

### Blog / articles (`/admin/articles`)
Full CRUD — see §2's Blog section for the content model (original posts only, currently; curated link-out mode still exists in the form but has no published rows). Fields: title, summary, body (plain text or HTML), cover image URL, source URL/name (curated only), tags, slug (auto-generated from title if left blank), published date, published toggle, sort order. The edit page (existing articles only) also lists everyone who thumbs-up'd the post, newest first — see §2's Thumbs-up paragraph for the three display states.

### Availability (`/admin/availability`)
CRUD for the weekly recurring rules (weekday, start/end time, slot length in minutes, active toggle) that `core/scheduler` expands into bookable hourly slots for §2's Booking page.

### Profile (`/admin/profile`)
Edit the single `profile` row: name, title, tagline, hero subtitle, summary, location, email, LinkedIn/GitHub/resume URLs, photo URL, years of experience.

### Preview as visitor (`GET /admin/preview-visitor` / `GET /exit-preview`)
Lets a logged-in admin browse the site exactly as a normal visitor would (no Admin Console tab, no automatic gated-content access) without actually logging out — a cookie flag, not a session change.

---

## 5. Cross-cutting systems (touch both admin and visitor sides)

- **Notifications** — the header's bell aggregates, at request time, two live sources: any unacknowledged booking reschedule, and any unread `admin_messages` row for that user. No dedicated "notifications" table — see `userNotifications()` in `core/handlers/app.go`.
- **PDF generation** (resume, receipts, certificates) — all generated in-process with `github.com/go-pdf/fpdf`, from live DB data, no external service. A Vercel serverless function can't shell out to a headless-Chrome-style renderer, so this is a deliberate constraint, not an oversight.
- **Telegram notifications** — fires on a paid course purchase and a confirmed (free) consultancy booking (`core/notify`), straight to the admin's own chat. Optional — silently a no-op if unconfigured.
- **Regional pricing** — course contribution and the Architecture Review's display-only reference figure resolve currency/amount from the visitor's detected country (`core/geo`): India gets its own INR figure, everyone else gets the USD one. Consultancy hourly has no price to resolve — it's always free.
- **Security headers** — a baseline CSP + `X-Content-Type-Options`/`X-Frame-Options`/`Referrer-Policy` on every response (`core/middleware/security.go`), allowlisting only Razorpay's checkout origin and Google Fonts.
- **IndexNow** — a `submitIndexNow(host, paths)` helper can push changed-URL batches directly to Bing/Yandex; not yet wired to fire automatically on every admin save (a manual/future enhancement), but the key-file route and submission capability are live.

---

*Last updated: September 2026, alongside the course pricing / consultancy 3-tier / articles-blog / SEO work described in the project's git history. If you're an AI agent reading this to get oriented on the codebase, `docs/ARCHITECTURE.md` is the companion doc — read both before making changes.*
