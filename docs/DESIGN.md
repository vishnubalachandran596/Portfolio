# Design System — vishnubalachandran.com

The companion to `docs/FUNCTIONALITY.md` (what the product does) and `docs/ARCHITECTURE.md` (how it's built): this file is *how it looks, reads and moves*. Read it before adding or restyling any UI. Every value lives as a token at the top of `web/static/css/style.css`; components never use raw hex values.

## Philosophy — "engineered editorial"

1. **Evidence over adjectives.** Numbers, names and outcomes lead; decoration that carries no information is cut.
2. **One accent, used sparingly.** Gold marks the single most important action or fact on a screen. Everything else is ink and stone.
3. **Type does the hierarchy.** Cormorant for display, Inter for reading and UI, JetBrains Mono for code.
4. **Motion explains, never decorates.** It confirms a press, shows where a drawer came from, or reveals a section once.
5. **Mobile is the primary layout.** Designed at 390 px first; 44 px touch targets; then widened.
6. **Fast is a feature.** No splash screen, no layout shift, self-hosted fonts, no JS framework (CLAUDE.md rule 6).

### Design references (the owner's chosen sources — consult them for every redesign or content update)

| Reference | Link | What this site takes from it |
| --- | --- | --- |
| UI UX Pro Max | https://github.com/nextlevelbuilder/ui-ux-pro-max-skill | Its design-system generator independently proposed this site's stone + gold palette and the Cormorant pairing — the reason the identity was evolved, not replaced. Use its checklists (contrast, touch targets, hierarchy) on any new screen. |
| Emil Kowalski — design engineering skills | https://github.com/emilkowalski/skills | The motion rules below: custom ease-out curves, sub-300 ms UI timing, scale 0.97 on press, reveals once, reduced motion. |
| Watermelon UI | https://21st.dev/@quantvansh | Interaction patterns (pill pagination, filter chips, toasts, drawers), rebuilt in vanilla CSS/JS because CLAUDE.md rule 6 rules out React. |
| Brand-building skills | https://github.com/arnabbagxd/Brand-building-skills | Positioning and voice: one point of view up front, proof (numbers, names, outcomes) before pitch. |
| "Whisk" | — | Named by the owner; no matching skill repo has been identified yet. Ask before assuming one. |

## Tokens

| Group | Tokens |
| --- | --- |
| Colour | `--bg`, `--bg-subtle`, `--surface`, `--surface-sunken`, `--ink`, `--ink-2`, `--ink-3`, `--line`, `--line-strong`, `--accent`, `--accent-strong`, `--accent-soft`, `--on-accent`, `--band*`, `--success*`, `--danger*` |
| Type | `--font-display` (Cormorant), `--font-text` (Inter), `--font-mono` (JetBrains Mono); fluid scale `--step--1` … `--step-5`; `--measure-prose` 68ch |
| Space | `--space-1` … `--space-10` (4 px base), `--section-pad`, `--gutter` |
| Shape | `--radius-sm/md/lg/pill`, `--shadow-sm/md/lg` |
| Layout | `--container` 72rem, `--container-prose` 44rem, `--container-wide` 80rem, `--header-h` |
| Layers | `--z-sticky` 100, `--z-header` 200, `--z-drawer` 400, `--z-toast` 500, `--z-modal` 600 |
| Motion | `--ease-out`, `--ease-in-out`, `--ease-drawer`; `--dur-press` 120ms, `--dur-fast` 160ms, `--dur-base` 220ms, `--dur-slow` 320ms, `--dur-reveal` 480ms |

Dark mode redefines only the colour tokens — under `prefers-color-scheme: dark` unless the visitor picked light, and under `[data-theme="dark"]`. A tiny inline script in `base.html` applies a stored choice before first paint.

**Legacy aliases** (`--color-text`, `--color-accent`, `--font-sans`, …) stay defined because uploaded lesson and article HTML carries `<style>` written against them, and the lesson iframe copies that CSS verbatim. Don't use them in new code; don't delete them.

Contrast (verified): ink on surface 17.5:1, `--ink-2` 7.6:1, `--ink-3` 5.2:1, accent on white 4.9:1. Text on `--accent-soft` uses `--accent-strong`.

Breakpoints are mobile-first: **40rem** (640 px), **60rem** (960 px), **75rem** (1,200 px). Don't add new ones.

Prices: always through `models.FormatMoney` (Indian grouping for INR — ₹1,50,000), rendered in Inter with tabular figures. The rupee sign comes from a 1 KB Inter subset (`inter-rupee-var.woff2`).

## Motion rules

- Every pressable element scales to 0.97 on `:active`. Hover styles live inside `@media (hover: hover) and (pointer: fine)`.
- Entering things use `--ease-out` and start from `scale(0.96)` + opacity, never `scale(0)`; popovers grow from their trigger.
- Exits are faster than entries (`--dur-fast` vs `--dur-base`/`--dur-slow`).
- Drawers and sheets (mobile nav, reader contents, code editor) use `--ease-drawer`.
- Scroll reveals: add `data-reveal` to an element. `main.js` hides it only after JS runs (`.js-reveal`), reveals it once (12 px rise + fade), and staggers siblings 50 ms (max 6). No-JS readers and crawlers always see the content.
- Keyboard-triggered actions don't animate. `prefers-reduced-motion` removes movement and keeps fades.
- No splash screens and no infinite animation outside a real loading state (the route bar).

## Components (class → use)

| Component | Classes |
| --- | --- |
| Buttons | `.btn` + `.btn-primary` / `.btn-secondary` / `.btn-ghost` / `.btn-ink`, sizes `.btn-sm` / `.btn-lg`, `.btn-block`; `.icon-btn` (always with `aria-label`); `.link-arrow`, `.text-link` |
| Chips & badges | `.tag-chip` (+ `.tag-chip-active`), `.badge`, `.badge-accent`, `.course-level`, `.status-badge` |
| Page structure | `.page-hero`, `.home-hero`, `.product-hero`, `.section` (+ `.section-alt`, `.section-tight`), `.band` (dark), `.section-head`, `.section-header-row`, `.breadcrumb`, `.page-index` (sticky in-page nav) |
| Cards | `.card`, `.card-interactive`, `.course-card`, `.catalog-card`, `.article-card`, `.offer-card` (on `.band`), `.pricing-card`, `.feature-card`, `.track` |
| Data | `.stat-strip` / `.stat`, `.logo-strip`, `.timeline`, `.outline` (numbered list), `.admin-table` in `.table-scroll` |
| Commerce | `.product-layout` + sticky `.product-aside` > `.buy-card`; `.buy-bar` (phones, hidden while the buy card is on screen); `.purchase-box` |
| Navigation | `.topic-nav`, `.article-filters`, `.pagination`, `.toc-list`, `.mobile-nav` |
| Feedback | `window.siteToast(message, "error"?)` → `.toast`; `.alert-success` / `.alert-error` |
| Forms | `label` wraps its field; `.form-row`, `.form-card`, `.admin-form`, `.optional`, `.field-help` |
| Prose | `.html-content` / `.article-body` (articles and every lesson), `.callout`, `.objectives`, `.recap`, `.article-diagram` (inline SVG, `dg-*` classes) |

## Course reader

`course_learn.html` frames the sandboxed lesson iframe (still `sandbox="allow-scripts"`, no same-origin) with a contents rail (bottom sheet on phones), a progress bar, a section counter, previous/next, `j`/`k` keys and the site's own code editor drawer (server-side runs through JDoodle). The lesson side is `core/storage/lesson_runtime.go`, injected by `PrepareLesson`; it posts headings and scroll progress up and accepts "go to section" down, so the reader never reads the lesson DOM. See ARCHITECTURE.md §13.

## Content updates (courses and articles) — the strategy

Every course lesson and article follows the same path, so the content can never drift back to per-file themes:

1. **Source of truth.** Lessons live in Storage at `courses/{slug}.html` (bucket `course-content`), articles at `articles/{slug}.html` (bucket `articles`). The repo holds no content (CLAUDE.md rule 8).
2. **One normalizer.** `storage.NormalizeLesson` (`core/storage/normalize.go`) runs on every upload: the course form, Admin → Courses → Bulk upload, and the article form. Offline edits go through `go run ./cmd/lessonfmt -out DIR files...`. It is idempotent and stamps `<!-- vb:normalized v1 -->`. What it does:
   - Maps the lesson's own `:root` palette onto the site tokens (renamed `--l-*`), so components keep their rules but take the site's colours, type and dark mode. It also drops Google Fonts `@import`s, which the CSP blocks anyway.
   - Removes in-lesson "On this page" / "Contents" blocks, because the reader's contents rail replaces them.
   - Strips section numbers from `<h2>`, because the reader and the outline number sections.
   - Labels code blocks `language-java|go|python|javascript|sql|bash` when the signal is unambiguous. Runnable languages get a Run button.
3. **Dark "app" lessons** with hard-coded dark panels declare `<!-- vb:theme dark -->`. They then render in the site's dark palette in both schemes, and their teal/navy literals are recoloured to the site's dark surfaces and gold.
4. **Per-course fixes** that the normalizer can't make generically (a grid that blows out on phones, a fixed-width diagram) are small CSS rules appended to that lesson's `<style>`, each with a `/* vb: … */` comment saying why.
5. **Motion and navigation come from the reader**, not the lesson. `lesson_runtime.go` builds the contents from `h2`/`h3`, skips headings in hidden tab panels, and reveals concept cards, labs, figures, tables and code once as they scroll in. A Run button appears only for languages that have a JDoodle embed key (`JDOODLE_EMBED_KEYS`).
6. **Verify before upload.** Render every changed lesson through `PrepareLesson` and check it at 390 px and desktop, in light and dark: no horizontal overflow, no console errors, Inter as the body font. Keep the originals as a rollback copy: copy each object to `backup/{date}/` in its own bucket before replacing it (a server-side copy, so size doesn't matter), or put a zip in the Drive folder `Portfolio`. The 2026-10-17 rework's originals are in `backup/2026-10-17/`.
7. **After upload:** clear `courses.outline` for the changed courses so the detail page re-derives "What's inside", and flag the chatbot reindex.

## Lesson authoring conventions

New or reworked lesson files should use: `<h2>` per section (these become the reader contents and the public "What's inside" outline — don't number them, the outline numbers itself; a navigation heading such as "On this page" or "Contents" is skipped, see `models.IsNavigationHeading`), `<h3>` for sub-sections, `<pre><code class="language-java|python|go">` for runnable snippets (gives them a Run button), and the shared classes `.objectives`, `.callout`, `.lab-block`, `.recap`, `.concept-card`. Keep lesson `<style>` to what a lab genuinely needs; typography comes from the site stylesheet.
