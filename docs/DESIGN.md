# Design System — vishnubalachandran.com

The companion to `docs/FUNCTIONALITY.md` (what the product does) and `docs/ARCHITECTURE.md` (how it's built): this file is *how it looks, reads and moves*. Read it before adding or restyling any UI. Every value lives as a token at the top of `web/static/css/style.css`; components never use raw hex values.

## Philosophy — "engineered editorial"

1. **Evidence over adjectives.** Numbers, names and outcomes lead; decoration that carries no information is cut.
2. **One accent, used sparingly.** Gold marks the single most important action or fact on a screen. Everything else is ink and stone.
3. **Type does the hierarchy.** Cormorant for display, Inter for reading and UI, JetBrains Mono for code.
4. **Motion explains, never decorates.** It confirms a press, shows where a drawer came from, or reveals a section once.
5. **Mobile is the primary layout.** Designed at 390 px first; 44 px touch targets; then widened.
6. **Fast is a feature.** No splash screen, no layout shift, self-hosted fonts, no JS framework (CLAUDE.md rule 6).

References used when this system was built (Oct 2026): UI UX Pro Max (its generator independently proposed this site's stone + gold palette and Cormorant pairing — the reason the identity was evolved, not replaced), Emil Kowalski's design-engineering skills (the motion rules below), Watermelon UI (interaction patterns rebuilt in vanilla CSS/JS: pill pagination, filter chips, toasts), and the Brand-building personal-brand skill (proof before pitch).

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
- Drawers and sheets (mobile nav, reader contents, compiler) use `--ease-drawer`.
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

`course_learn.html` frames the sandboxed lesson iframe (still `sandbox="allow-scripts"`, no same-origin) with a contents rail (bottom sheet on phones), a progress bar, a section counter, previous/next, `j`/`k` keys and a JDoodle compiler drawer. The lesson side is `core/storage/lesson_runtime.go`, injected by `PrepareLesson`; it posts headings and scroll progress up and accepts "go to section" down, so the reader never reads the lesson DOM. See ARCHITECTURE.md §13.

## Lesson authoring conventions

New or reworked lesson files should use: `<h2>` per section (these become the reader contents and the public "What's inside" outline — don't number them, the outline numbers itself), `<h3>` for sub-sections, `<pre><code class="language-java|python|go">` for runnable snippets (gives them a Run button), and the shared classes `.objectives`, `.callout`, `.lab-block`, `.recap`, `.concept-card`. Keep lesson `<style>` to what a lab genuinely needs; typography comes from the site stylesheet.
