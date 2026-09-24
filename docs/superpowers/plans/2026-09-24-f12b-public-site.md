# F12b — Public Site Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans`. Inline, one session, no subagents.

**Goal:** 01 §7.19's public site with the new design language:
- **Routes (in both locales):** `/`, `/about`, `/references`, `/documents`, `/toolkit`, `/pricing` (flag), `/request-demo`, `/contact`, `/blog`, `/blog/[slug]`, `/bill-calculator`.
- **Blog:** the two legacy articles with their images.
- **Forms and calculator:** the public forms and calculator wired to F12a.
- **SEO:** metadata, Open Graph, sitemap, robots, JSON-LD.

**Architecture:**
- **Routes:** a `(site)` route group with its own layout (announcement bar, header with a mobile drawer, footer). The middleware lets these paths through without a session; `/` is the homepage (it redirected to `/ekorm` until now, R167).
- **Content:**
  - UI and page copy live in the `site` namespace, so both locales are parity-checked;
  - the blog is Markdown under `web/content/blog/<slug>.md`, rendered on the server with `marked` (GFM, raw HTML); images are under `public/blog/`.
- **Views:** as elsewhere, pure views with stories and tests, plus thin page files.

## Open questions (defaults shipped)

| # | Question | Default | Cost if wrong |
|---|---|---|---|
| Q-H7 | Markdown renderer. | **`marked`** (one dependency, GFM + raw HTML); the articles are repo content, so there is no user HTML | One dependency |
| Q-H8 | The "embedded map" on Contact. | **An OpenStreetMap link and the address; no third-party iframe** (CSP/privacy, air-gapped installs) | No inline map |
| Q-H9 | Analytics. | **The `EKOKOD_FEATURE_ANALYTICS` flag exists (default off); no vendor script ships.** Adding one is a product decision | No analytics until chosen |
| Q-H10 | Homepage copy. | The legacy information architecture, **rewritten concisely** (the legacy copy is hard-coded in components). The sections from §7.19 are kept, with no fabricated metrics, customers or testimonials. References and testimonials show only genuine, attributable content; none is available, so those sections say "yakında" | A thinner homepage until marketing supplies content |
| Q-H11 | A logged-in user at `/`. | **The site renders with "Platforma git"**; the `/ekorm` routes stay guarded | — |

## Tasks
1. Middleware (public paths), the `(site)` layout (header, drawer, footer, announcement), the `site` messages, SEO base (metadata, sitemap, robots) + tests.
2. Homepage, about, references, documents, toolkit, pricing (flag) views + pages + JSON-LD.
3. Contact and request-demo forms (F12a routes, honeypot, elapsed time) + the bill calculator page (form + breakdown + Q-H3/Q-H4 notes).
4. Blog: list with category filter, article page (`marked`), the two legacy articles and images.
5. e2e `tests/e2e/public/*.spec.ts`, build, the a11y run, self-review, handoff.
