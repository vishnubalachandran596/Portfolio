---
name: mcp-purpose-routing
description: Single MCP purpose routing — call only ready namespaces, rotate free peers, then OpenRouter (~$10) and the host model last. Skip needsAuth and error servers.
---

# MCP purpose routing (single policy)

**Live Cursor session (2026-10-26):** 68 namespaces. Call one only when it is `ready` and exposes real tools. Skip `needsAuth` and `error` (often only `mcp_auth`). Do not call `mcp_auth` during ordinary work. Section 4 is the call list. The appendix is a name catalog; when they disagree, section 4 wins.

## 1. Implicit always (mandatory every turn)

This policy is **global (account-wide)** — not tied to one repo or project. It applies **automatically every turn** on **Cursor and Claude** via: Claude.ai **Instructions for Claude**, Cursor **User rules**, `~/.cursor/rules/mcp-purpose-routing.mdc` (`alwaysApply: true`), and this skill on Claude Code. The user does **not** need to say "use MCP", "compress", or "save tokens". Skipping free-first order, **round-robin / LRU** among free peers, the **Token reduction ladder**, or silent `sqz` **after MD** when connected is a **rule break**.

## 2. Global agent ladder

1. **Free-tier MCP AIs first** — purpose tables below; **rotate** among free peers with quota left (not always index 0).
2. **Next free MCP** in the same purpose on cap, auth, empty, or bad quality.
3. **All free MCPs for that purpose exhausted** → **OpenRouter MCP** (`openRouter` / `Openrouter` / `user-openrouter-mcp`): **free models first**, then **paid** against **~$10** escalation budget (**NOT** first choice).
4. **Host Claude / Cursor model last** — orchestration only when no MCP can run the step.

**Fallback:** one short retry → next in purpose list → OpenRouter → host (one-line reason). **Never OpenRouter first** for search, extract, or compression.

**Parallelism & memory:** batch independent MCP calls per turn; **Mem0** + Claude/Cursor memory / Agent Stores before and after multi-agent groups.

## 3. Round-robin credit minimize

Within each purpose: prefer **keyless → high free RPD → keyed quota remaining**; **LRU / round-robin** across eligible free MCPs across turns so one quota is not burned while siblings sit unused.

## 4. Purpose tables (ready peers only, 2026-10-26)

Call only namespaces that are `ready` and expose real tools. If status is `needsAuth` or `error`, or the only tool is `mcp_auth`, skip it and take the next ready peer. Rotate among ready peers with quota left. Then OpenRouter free, then paid (~$10), then the host model.

**Installed but not callable:** `user-aituber`, `plugin-browserless-browserless`, `user-cloudflare-dex-mcp`, `user-cloudinary`, `user-google-bigquery-mcp`, `user-orshot`, `user-scrapfly` (`needsAuth`); `user-browserbase`, `user-comfy-cloud`, `user-elevenlabs`, `user-upstox`, `user-z-ai-web-search-mcp` (`error`). Voice uses `plugin-elevenlabs-elevenlabs`, not `user-elevenlabs`.

**Not in this Cursor session:** Context7, Figma, Canva, Adobe, Booking.com, Indeed, Supabase, Google Sheets, Microsoft Learn, OctoPerf, openart, PDF.net, Kernel, Docusign, Vanguard, Z.AI reader/zread/vision, HTML/CSS to Image, Anthropic Economic Index, Arize. Do not invent calls to them.

### Web search / quick research

1. `user-you-com-search` — `you-search`, `you-discover`
2. `user-parallel-search` — `web_search`
3. `user-exa-mcp` — `web_search_exa`
4. `user-tavily` — `tavily_search`
5. `user-firecrawl-mcp` — search
6. `user-jina` — `search_web`
7. `user-bright-data` — only after the lighter search peers fail
8. OpenRouter free, then paid, for synthesis only

### Deep multi-step research

1. `user-parallel-task` — `createDeepResearch`, then `getStatus` / `getResultMarkdown`
2. `user-firecrawl-mcp` — `firecrawl_agent`, then `firecrawl_agent_status`
3. `user-exa-mcp` — `agent_run`
4. `user-tavily` — `tavily_research`
5. OpenRouter free, then paid

### Read URL / PDF → markdown (token ladder step 1)

1. `user-jina` — `read_url`, `extract_pdf`
2. `user-firecrawl-mcp` — scrape / parse
3. `user-parallel-search` — `web_fetch`
4. `user-tavily` — `tavily_extract`
5. `user-exa-mcp` — `web_fetch_exa`
6. OpenRouter only to rewrite Compact-MD, not to invent facts

### Token / context compression

`user-sqz` after markdown: `compress`, `sqz_read_file`, `sqz_grep`, `sqz_list_dir`. `expand` / `sqz_recall` for byte-exact recovery. Follow §5.

### AI / chat / multi-model reasoning

1. `user-hugging-face-mcp`
2. `user-openrouter-mcp` — free models, then paid (`send-message`, `send-batch`, image / speech / transcribe)
3. Host last

### Repo / library / code Q&A

1. `user-deepwiki`
2. `user-gitmcp`
3. `plugin-github-github`, `user-github`, `cursor-origin`
4. Docs, first match: `user-astro-docs`, `user-you-com-docs`, `user-gemini-docs-mcp`, `user-cohere-docs-mcp`, `user-nvidia-cuda-docs-mcp`, `user-cloudflare-docs-mcp`, `user-cloudflare-agents-sdk-docs`, `user-cloudflare-blog-mcp`
5. OpenRouter free, then paid

### Scrape / hard sites / structured extraction

1. `user-firecrawl-mcp`
2. `user-apify`
3. `user-bright-data`
4. `plugin-tinyfish-tinyfish`, `plugin-playwright-playwright` — when a real browser is required
5. `cursor-ide-browser` — a local interactive page
6. `cursor-computer-use` — native desktop, last

### Design / UI reference

1. `plugin-mobbin-mobbin` — `search_screens`, `search_flows`, `search_sections`
2. `plugin-wonder-wonder`

### Image generation

1. `user-fal`
2. `user-replicate`
3. `user-runway`
OpenRouter only for planning text.

### Video generation

1. `user-fal`
2. `user-higgsfield`
3. `user-kling`
4. `user-runway`
5. `user-mux` — host the rendered asset

### TTS / voice / audio

1. `plugin-elevenlabs-elevenlabs` — discover one tool; the namespace has 164
2. `user-fal` audio
3. `user-openrouter-mcp` `generate-speech`

### Template PDF / image / video

`user-templated` only.

### Media CDN / transform

`user-mux`. Cloudinary is installed and not callable until it is authenticated.

### Email / calendar / Google workspace

`user-gmail-mcp`, `user-google-calendar-mcp`, `plugin-google-docs-google-docs`, `plugin-google-drive-google-drive`, `user-google-drive-mcp`, `plugin-google-slides-google-slides`. BigQuery is installed and not callable. There is no Sheets namespace.

### Travel / jobs / finance / legal (domain-only)

No travel, jobs, e-sign, or advisor namespace is connected. Upstox is in error. Do not substitute OpenRouter.

### App backend / DB / deploy / errors / observability

`user-neon`, `user-firebase`, `user-netlify`, `plugin-vercel-vercel`, `user-sentry`. Ready Cloudflare ops: `user-cloudflare-api-mcp`, `user-cloudflare-containers-mcp`, `user-cloudflare-browser-run-mcp`, `user-cloudflare-casb-mcp`. DEX is not callable. No OpenRouter substitute. `plugin-vercel-vercel` has 239 tools: discover the one tool you need.

### Cloudflare documentation & edge

Docs first: `user-cloudflare-docs-mcp`, `user-cloudflare-agents-sdk-docs`, `user-cloudflare-blog-mcp`. Operations use the ready Cloudflare namespaces in the backend table.

### LLM engineering / memory

`user-mem0-mcp` — `search_memories` before multi-step work, `add_memory` for a decision. On this Windows account the stored user ids are `vishnu`, `vishnubalachandran`, `vishnubalachandran596`, and `mem0-mcp`. The login name `Vishnu` matches none of them. `user-sqz` compresses tool traffic. Arize is not in this session.

### Performance testing

No load-test MCP is connected.

## 5. Token reduction ladder (mandatory every turn)

Parallel to MCP AI routing; **lossless** (no silent drop of numbers, names, URLs, code, decisions).

1. **Free extract MCPs** (LRU, ready only): **Jina** `read_url` / `extract_pdf` → **Firecrawl** scrape/parse → **Parallel** `web_fetch` → **Tavily** `tavily_extract` → **Exa** `web_fetch_exa`. Z.AI reader is not connected. Bulky PDF/HTML/URL becomes Markdown with `keep:` bullets.  
2. **Next free peer** on cap/auth/empty/quality fail.  
3. **`sqz` `compress`** after MD (Outline → Summary + `keep:` → Compact-MD / Query-Brief-MD) when connected — prefer `sqz_read_file`, `sqz_grep`, `sqz_list_dir`; `expand` / `sqz_recall` for byte-exact recovery. **Claude.ai without sqz:** Manual sqz-inspired pass per `context-md-compact` skill.  
4. Free extract exhausted → **OpenRouter free** models rewrite Compact-MD only (**not** invent facts).  
5. **OpenRouter paid** (~$10 budget) if still insufficient.  
6. **Host model last** for compaction rewrite only.

**Context compaction (always)** when ~500+ tokens, bulky MCP output, or attachments — full MD protocol in `C:\Users\Vishnu\.claude\skills\context-md-compact\SKILL.md`. No free public HTTPS token-reducer MCP (2026-10-19).

## 6. Multi-agent / mem0

Split work by purpose groups above; each group follows this ladder and stays on ready namespaces. Batch independent MCP calls in one turn. Before the group, `user-mem0-mcp` `search_memories`. After a decision, `add_memory`. Also keep Cursor Agent Stores.

## 7. Token hygiene (all purposes)

Short tool args; search/list before full fetch; limits 1–3 unless required; summarize MCP results before host context; no redundant repeat calls for the same fact.

## 8. Full catalog appendix (alphabetical)

Name catalog from Claude.ai Yours and the Cursor dashboard. Section 4 decides whether a name is callable in this session.

- **Adobe for creativity** — *Creative / PDF* — **Use case:** Adobe Creative Cloud assets, PDF, and design workflows from chat.
- **AITuber** — *Video gen* — **Use case:** Short-form / avatar video with free starter credits.
- **aituber** — *Video gen* — **Use case:** Cursor MCP id for AITuber.
- **Anthropic Economic Index** — *Research / data* — **Use case:** Anthropic labor/economic index datasets and charts.
- **Apify** — *Scrape / automation* — **Use case:** Run Apify actors for structured web extraction.
- **apify** — *Scrape* — **Use case:** Cursor MCP id for Apify.
- **Arize** — *LLM ops* — **Use case:** Traces, datasets, evals, prompt ops (Cursor plugin).
- **Astro Docs** — *Docs* — **Use case:** Semantic search over Astro documentation.
- **astro-docs** — *Docs* — **Use case:** Cursor MCP id for Astro Docs.
- **Booking.com** — *Travel* — **Use case:** Search stays and travel inventory.
- **Bright Data** — *Search / scrape* — **Use case:** Proxy-backed search/scrape (~5k/mo free when connected).
- **bright-data** — *Search / scrape* — **Use case:** Cursor MCP id for Bright Data.
- **Browserbase** — *Browser automation* — **Use case:** Hosted browser sessions for dynamic/login pages.
- **browserbase** — *Browser automation* — **Use case:** Cursor MCP id for Browserbase.
- **Browserless** — *Browser automation* — **Use case:** Headless Chrome API (Cursor plugin).
- **Canva** — *Design* — **Use case:** Create and edit Canva designs from chat.
- **Cloudflare API MCP** — *Cloudflare ops* — **Use case:** Zones, DNS, Workers, account APIs.
- **Cloudflare Developer Platform** — *Cloudflare ops* — **Use case:** Developer platform tools and resources.
- **Cloudflare Docs MCP** — *Docs* — **Use case:** Official Cloudflare product documentation search.
- **cloudflare-agents-sdk-docs** — *Docs* — **Use case:** Cloudflare Agents SDK documentation.
- **cloudflare-api-mcp** — *Cloudflare ops* — **Use case:** Cursor MCP id for Cloudflare API.
- **cloudflare-blog-mcp** — *Docs / news* — **Use case:** Cloudflare blog and announcements.
- **cloudflare-browser-run-mcp** — *Browser / Workers* — **Use case:** Browser rendering in Workers sandbox.
- **cloudflare-casb-mcp** — *Security* — **Use case:** CASB / SaaS security posture.
- **cloudflare-containers-mcp** — *Cloudflare ops* — **Use case:** Cloudflare Containers management.
- **cloudflare-dex-mcp** — *Observability* — **Use case:** Digital Experience / device metrics.
- **cloudflare-docs-mcp** — *Docs* — **Use case:** Cursor MCP id for Cloudflare Docs.
- **cloudinary** — *Media CDN* — **Use case:** Cursor MCP id for Cloudinary.
- **Cloudinary Assets** — *Media CDN* — **Use case:** Upload, transform, deliver media.
- **Cohere Docs MCP** — *Docs* — **Use case:** Cohere API and SDK documentation.
- **cohere-docs-mcp** — *Docs* — **Use case:** Cursor MCP id for Cohere docs.
- **Comfy Cloud** — *Video / image gen* — **Use case:** ComfyUI cloud generative pipelines.
- **comfy-cloud** — *Video / image gen* — **Use case:** Cursor MCP id for Comfy Cloud.
- **Context7** — *Docs* — **Use case:** Up-to-date library docs for coding tasks.
- **cursor-computer-use** — *Desktop automation* — **Use case:** Local Windows desktop control when needed.
- **cursor-ide-browser** — *Browser automation* — **Use case:** Cursor-owned browser tab and CDP.
- **cursor-origin** — *Git host* — **Use case:** Origin.cursor.com repos, PRs, checks.
- **DeepWiki** — *Repo Q&A* — **Use case:** AI wiki Q&A on public GitHub repos (keyless).
- **deepwiki** — *Repo Q&A* — **Use case:** Cursor MCP id for DeepWiki.
- **Docusign** — *Legal / e-sign* — **Use case:** Send, sign, track Docusign envelopes.
- **ElevenLabs** — *TTS / voice* — **Use case:** TTS, voice design, agents (plugin).
- **elevenlabs** — *TTS / voice* — **Use case:** ElevenLabs MCP server on Cursor dashboard.
- **Exa MCP** — *Web search* — **Use case:** Neural web search and fetch.
- **exa-mcp** — *Web search* — **Use case:** Cursor MCP id for Exa.
- **fal** — *Image / video / audio* — **Use case:** fal.ai generative media hub.
- **Figma** — *Design* — **Use case:** Read and interact with Figma files.
- **firebase** — *Backend* — **Use case:** Firebase deploy, rules, SDK (local stdio).
- **Firecrawl MCP** — *Scrape / search* — **Use case:** Scrape, map, crawl, search, agent research.
- **firecrawl-mcp** — *Scrape / search* — **Use case:** Cursor MCP id for Firecrawl.
- **Gemini Docs MCP** — *Docs* — **Use case:** Gemini API official docs and SDK.
- **gemini-docs-mcp** — *Docs* — **Use case:** Cursor MCP id for Gemini docs.
- **GitHub** — *Repos / PRs* — **Use case:** GitHub plugin on Cursor dashboard.
- **github** — *Repos / PRs* — **Use case:** Local GitHub MCP launcher (mcp.json).
- **GitHub Integration** — *Repos / PRs* — **Use case:** Claude.ai GitHub OAuth integration.
- **gitmcp** — *Docs / repo* — **Use case:** Cursor MCP id for GitMCP.
- **GitMCP (Next.js)** — *Docs / repo* — **Use case:** GitMCP doc fetch for Next.js/repos.
- **Gmail** — *Email* — **Use case:** Read, draft, send Gmail (plugin).
- **gmail-mcp** — *Email* — **Use case:** Hosted Gmail MCP (mcp.json).
- **Google Calendar** — *Calendar* — **Use case:** List and create events (plugin).
- **Google Cloud BigQuery** — *Analytics / SQL* — **Use case:** BigQuery jobs and datasets.
- **Google Docs** — *Documents* — **Use case:** Create and edit Google Docs.
- **Google Drive** — *Files* — **Use case:** Search and manage Drive (plugin).
- **Google Sheets** — *Spreadsheets* — **Use case:** Read and update Sheets.
- **Google Slides** — *Presentations* — **Use case:** Build and edit slide decks.
- **google-bigquery-mcp** — *Analytics / SQL* — **Use case:** BigQuery MCP (mcp.json).
- **google-calendar-mcp** — *Calendar* — **Use case:** Hosted Calendar MCP (mcp.json).
- **google-drive-mcp** — *Files* — **Use case:** Hosted Drive MCP (mcp.json).
- **Higgsfield** — *Video gen* — **Use case:** Higgsfield generative video.
- **higgsfield** — *Video gen* — **Use case:** Cursor MCP id for Higgsfield.
- **HTML/CSS to Image MCP** — *Image gen* — **Use case:** Render HTML/CSS to PNG/JPEG.
- **Hugging Face** — *Models / inference* — **Use case:** Hub models, datasets, Spaces, inference.
- **hugging-face-mcp** — *Models / inference* — **Use case:** Cursor MCP id for Hugging Face.
- **Indeed** — *Jobs* — **Use case:** Job search via Indeed connector.
- **Jina** — *Extract / search* — **Use case:** read_url, search_web, extract_pdf — ladder step 1.
- **jina** — *Extract / search* — **Use case:** Cursor MCP id for Jina.
- **Kernel** — *Browser / automation* — **Use case:** Kernel community browser MCP (Claude).
- **Kling** — *Video gen* — **Use case:** Kling AI video generation.
- **kling** — *Video gen* — **Use case:** Cursor MCP id for Kling.
- **Mem0** — *Memory* — **Use case:** Long-term memory across sessions (plugin).
- **Microsoft Learn** — *Docs* — **Use case:** Microsoft official learning docs.
- **Mobbin** — *Design reference* — **Use case:** Real app UI patterns and screens.
- **Mux** — *Video hosting* — **Use case:** Mux assets, playback, analytics.
- **mux** — *Video hosting* — **Use case:** Cursor MCP id for Mux.
- **Neon** — *Database* — **Use case:** Neon Postgres branches and SQL.
- **neon** — *Database* — **Use case:** Cursor MCP id for Neon.
- **Netlify** — *Deploy* — **Use case:** Sites, deploys, env vars.
- **netlify** — *Deploy* — **Use case:** Cursor MCP id for Netlify.
- **NVIDIA CUDA Docs MCP** — *Docs* — **Use case:** CUDA/cuDNN/CUTLASS semantic search.
- **nvidia-cuda-docs-mcp** — *Docs* — **Use case:** Cursor MCP id for NVIDIA CUDA docs.
- **OctoPerf MCP** — *Performance test* — **Use case:** Load testing (Claude: Connect pending).
- **openart** — *Image gen* — **Use case:** OpenArt image generation.
- **Openrouter** — *AI / chat* — **Use case:** Multi-model chat/media — escalation (~$10), not first.
- **openRouter** — *AI / chat* — **Use case:** Cursor dashboard id for OpenRouter MCP.
- **Orshot** — *Template media* — **Use case:** Template PDF/image/video (~100 credits/mo).
- **orshot** — *Template media* — **Use case:** Cursor MCP id for Orshot.
- **Parallel Search** — *Web search / fetch* — **Use case:** Keyless web_search and web_fetch.
- **Parallel Task** — *Deep research* — **Use case:** Multi-step Parallel.ai research.
- **parallel-search** — *Web search / fetch* — **Use case:** Cursor MCP id for Parallel Search.
- **parallel-task** — *Deep research* — **Use case:** Cursor MCP id for Parallel Task.
- **PDF.net** — *PDF* — **Use case:** PDF manipulation and conversion.
- **Playwright** — *Browser automation* — **Use case:** Playwright browser testing plugin.
- **plugin-github-github** — *Repos / PRs* — **Use case:** GitHub MCP plugin namespace.
- **plugin-vercel-vercel** — *Deploy* — **Use case:** Vercel MCP plugin namespace.
- **Replicate** — *Image / ML* — **Use case:** Replicate model runs.
- **replicate** — *Image / ML* — **Use case:** Cursor MCP id for Replicate.
- **Runway** — *Video / image* — **Use case:** Runway generative video/image.
- **runway** — *Video / image* — **Use case:** Cursor MCP id for Runway.
- **Scrapfly** — *Scrape* — **Use case:** Anti-bot scrape and extraction.
- **scrapfly** — *Scrape* — **Use case:** Cursor MCP id for Scrapfly.
- **Sentry** — *Errors* — **Use case:** Issues, releases, error triage.
- **sentry** — *Errors* — **Use case:** Cursor MCP id for Sentry.
- **sqz** — *Token compress* — **Use case:** compress/expand/sqz_read_file after MD (ladder step 3).
- **Supabase** — *Database / backend* — **Use case:** Supabase projects, SQL, auth.
- **Tavily** — *Search / extract* — **Use case:** tavily_search and tavily_extract.
- **tavily** — *Search / extract* — **Use case:** Cursor MCP id for Tavily.
- **Templated** — *Template media* — **Use case:** Template image/PDF (~50 free).
- **templated** — *Template media* — **Use case:** Cursor MCP id for Templated.
- **TinyFish** — *Browser automation* — **Use case:** TinyFish web automation plugin.
- **Upstox** — *Trading (IN)* — **Use case:** Indian broker orders, holdings, IPOs.
- **upstox** — *Trading (IN)* — **Use case:** Cursor MCP id for Upstox.
- **Vanguard Advisor Tools** — *Finance* — **Use case:** Vanguard advisor tools and data.
- **Vercel** — *Deploy* — **Use case:** Vercel deployments and domains (plugin).
- **Wonder** — *Design* — **Use case:** Wonder design assistant plugin.
- **you-com-docs** — *Docs* — **Use case:** Cursor MCP id for You.com Docs.
- **you-com-search** — *Web search* — **Use case:** Cursor MCP id for You.com Search.
- **You.com Docs** — *Docs* — **Use case:** You.com developer documentation.
- **You.com Search** — *Web search* — **Use case:** you-search / you-discover (~100/day free).
- **z-ai-web-search-mcp** — *Web search* — **Use case:** Z.AI web search prime (mcp.json).
- **zai-vision** — *Vision* — **Use case:** Z.AI vision / image understanding.
- **zai-web-reader** — *Extract* — **Use case:** Z.AI URL reader / extract.
- **zai-web-search** — *Web search* — **Use case:** Z.AI web search (Cursor dashboard).
- **zai-zread** — *Extract* — **Use case:** Z.AI deep read / long documents.
