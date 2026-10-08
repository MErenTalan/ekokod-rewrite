# F23 — Dashboard map and form error feedback (2026-10-08)

Branch `fix/f23-map-form-errors` (`/home/personal/ekokod-f23-fixes`, cut from `main` @ 57db26d).

**Why.** The product owner reported that the map does not work, and that saving a tariff only said
"İstek gövdesi geçersiz" with nothing marked on the form. They asked for errors to be shown on the
field the response names, for clearer notifications, and for the same care on every form and endpoint.

## Rulings

| # | Ruling | Where |
|---|--------|-------|
| R515 | A body value its field cannot hold (Turkish `"12,5"`, a JSON number for a decimal, a bad date or uuid) is a 422 `validation_failed` naming every such field (`decimal`, `date`, `uuid`, `invalid`), on every endpoint. Unknown fields and malformed JSON stay 400 `invalid_body`, now with `reason`/`field` in details. | `internal/api/v1/kit/body_errors.go`, `bind.go` |
| R516 | Scalar `details` (reason, parameter) are not field errors; only code lists are. | `web/src/lib/api/problem.ts` |
| R517 | A 422 toast names the fields the form marks ("Düzeltilmesi gereken alanlar: T1 fiyatı (…)", max 4 + "ve N alan daha") and focuses the first; fields the form does not show are listed by API name. Network failures and proxy error pages say the server was not reached; `invalid_body`/`internal` give the request id as a support reference. | `web/src/lib/api/mutation.ts`, `Field` marks `data-field-*` |
| R518 | Free-text decimals accept a Turkish comma: `2,45` → `2.45`, `1.234,56` → `1234.56`; a lone dot stays a decimal point (unchanged API reading — no money value changes meaning). Non-numbers are named before sending. | `lib/decimal.ts` `toDecimalString`; tariff draft, defaults, solar, alarms |
| R519 | A form dialog closes only when the save succeeds (`done` callback), so a refusal stays readable; required fields are named instead of a silent no-op. | defaults/solar tariffs, company create, integration definition, credential create + history pull, SMTP test, ISO notes |
| R520 | Every field of every form shows the API error that names it (selects, rows, nested `tariff.` prefix for templates/bulk, `settings.*`/`channels[i]` for alarms, `periods[i].*` for vacations). | see commits 7442011, b66be8b |
| R521 | Map tiles come from `EKOKOD_MAP_TILE_URL`, read by the web server per request; unset = OpenFreeMap (free, keyless, commercial use allowed — not OSM's servers, so Q5 still holds), `none` = list only. Supersedes D23's build-time `NEXT_PUBLIC_MAP_TILE_URL`. | `lib/map-config.ts`, root layout, CSP |
| R522 | MapLibre's worker is served from `public/maplibre/<version>/` (copied before dev/build) and set with `setWorkerUrl`: the bundle rewrites `import.meta.url` to a build-machine path. | `scripts/copy-maplibre-worker.ts`, `map.tsx` |
| R523 | The map is created once per tile source; markers are redrawn only when their content changes; a late tile error does not swap a loaded map for the list; only a `{z}` template is raster. | `components/map/map.tsx` |

## Root causes of "the map does not work"

1. No tile URL ever reached the browser (build-time variable, Docker build had none) → list fallback.
2. With a URL, the MapLibre 6 worker URL was empty (bundled `import.meta.url` is `file:///…`).
3. The OpenFreeMap style URL has no `.json`, so it was used as a raster template.
4. The dashboard re-rendered the map with new marker arrays/callbacks, recreating it and aborting the style load.

## Verification

- Go: `internal/api/...` unit tests, `internal/arch`, golangci-lint on `internal/api/...` — green.
  The `-tags=integration` suites were **not** run: Docker was not reachable from WSL in this session.
  The two integration assertions on `invalid_body` (unknown fields) still hold by construction.
- Web: vitest 1290+ green, `pnpm lint`, typecheck, i18n parity, `next build`, `check:module-chunks`.
- Map: the built standalone app with a fake API, Playwright/Chromium under the app's real CSP: style,
  TileJSON, sprites, glyphs and vector tiles from tiles.openfreemap.org load, the worker comes from
  `/maplibre/6.10.0/`, markers drawn. CSP headers checked at runtime with the variable unset and `none`.
- e2e (Playwright `e2e` project) not run: it needs Docker.

## F24 follow-up (2026-10-09, branch `fix/f24-map-tiles-analyzers`)

The product owner found the OpenFreeMap positron map plain and asked for the Google layer an older
legacy build used (`{s}.google.com/vt/lyrs=m`), and pointed out that the dashboard's analyzer radio read
as a map filter that hid every other analyzer.

| # | Ruling | Where |
|---|--------|-------|
| R524 | Default tiles are Google's street tiles (`lyrs=m&hl=tr&scale=2`, mt0–mt3), at the owner's request. They are not a documented Google API (Maps Platform terms only cover its official APIs), so the first tile error before any tile drew swaps in OpenFreeMap liberty; `EKOKOD_MAP_TILE_URL=openfreemap` uses OpenFreeMap only. Supersedes R521's default. | `lib/map-config.ts`, `components/map/map.tsx`, CSP |
| R525 | A building's analyzers unfold under it as checkboxes deciding which analyzer markers the map shows (all by default, as legacy's AnalyzerList), each with its own "Seç" for the panels; "Tümünü seç/kaldır" covers buildings and analyzers. | `features/dashboard/building-list.tsx`, `dashboard-page.tsx` |

Verified: web vitest 1296, lint, typecheck, build; built app with a fake API in Chromium (Google tiles drawn,
an analyzer unticked leaves the map, Google answered 403 → OpenFreeMap drawn); e2e dashboard, consumption,
modules, responsive, security headers and the a11y pages.
