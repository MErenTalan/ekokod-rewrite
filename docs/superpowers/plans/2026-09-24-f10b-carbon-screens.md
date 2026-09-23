# F10b — Carbon Footprint Screens Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this
> plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This phase runs **inline in
> one session**: no parallel agents, no subagents, no Workflow (the user's standing rule).

**Goal:** Build the §7.16 Eko-CM screens on F10a's API as one module, `/ekorm/carbon-footprint`:
- overview;
- activity selection;
- data entry with the cascading detail fields;
- activity status with approval;
- reporting with PDF download;
- the emission factor database;
- company details;
- the GHG Protocol, ISO 14064 and reporting-standards reference pages;
- documents.

It also adds the e2e spec, the a11y run and a self-review.

**Architecture:**
- **Route and navigation.** One route, `app/ekorm/carbon-footprint/page.tsx` → `features/carbon/carbon-page.tsx`. The module's own navigation is a tab strip whose value lives in `?tab=`, so every section is deep-linkable.
- **Building gate.** Every section except the four static ones needs a selected building (`useSelection().buildingId`, picked with `ScopePicker`). Otherwise it shows an empty state.
- **Component split.** Each section is a container, `*-panel.tsx` (queries and mutations), plus pure `*View` components (stories and tests), following R195–R199.
- **Server-computed figures.** The entry dialog asks the server for the factor list of the chosen sub-category (`/carbon/emission-factors?sub_category=`), then cascades `category_path` levels 1–3 to one factor and offers its units. The emission, scope and ISO category are displayed from the catalogue and the server's answer, never computed in the browser.

**Tech Stack:** Next.js 15 + React 19, TanStack Query via `openapi-react-query` (`$api`), next-intl, Tailwind v4 tokens, Recharts via `components/charts`, Vitest + Storybook + Playwright.

**Spec:**
- `docs/rewrite/01-project-context.md` §7.16.
- `docs/rewrite/05-api-contract.md` §12.
- `docs/rewrite/09-implementation-plan.md` §F10 (e2e `tests/e2e/carbon.spec.ts`).
- `docs/rewrite/07-design-system.md` §5, §6, §8, §9, §11; `design-system/bcem-energy/MASTER.md`, then `OVERRIDES.md`.
- F10a plan `2026-09-24-f10a-carbon-backend.md` (R300–R319, Q-F1…Q-F7).
- Legacy: `src/app/components/carbon-footprint/*` (the layout and sidebar, the tabs, `ActivityEntryModal.tsx`, `reporting/Reporting.tsx`, `database/database.tsx`, the reference pages).

## Global Constraints

- **Worktree:** `phase/f10-carbon` in `/home/personal/ekokod-f10-phase`. The environment and command prefixes are as in F10a.
- **Web gates:**
  - `pnpm lint`, `pnpm typecheck`, `pnpm check:i18n-parity`, `pnpm check:api`, `pnpm test --maxWorkers=2`.
  - Storybook build, then the a11y specs, in the foreground and one at a time.
- **Design:** read `MASTER.md`, then `OVERRIDES.md`:
  - flat surfaces, emerald tokens, no raw colours;
  - status is colour + icon + text;
  - every chart has units, a legend, tooltips and a data table (`ChartFrame`);
  - 44 px touch targets; links that act as buttons are `Button asChild`.
- **Strings:** message keys are camelCase in the `carbon` namespace, `tr` + `en`. An apostrophe before `{` is doubled.
- **Screen conventions (R195–R199, R229, R249):**
  - `useApiMutation` with `invalidate`; `downloadFile` for the PDF;
  - `mockApi` refuses what the API refuses;
  - data-free stories; an open-dialog story needs trigger + play.
- **Coverage:** every new component file has a `.stories.tsx` and a `.test.tsx`. Containers (`*-page`, `*-panel`) need a test only.
- **Responsive:** `SCREENS` in `tests/e2e/responsive.spec.ts` gains `['carbon-footprint', '/ekorm/carbon-footprint']`.
- **Permissions:** from `/auth/me` (`can('carbon.read')`, `can('carbon.edit')`). Read-only roles see no write controls.

## Open questions (defaults shipped)

| # | Question | Default shipped | Cost if wrong |
|---|---|---|---|
| Q-F8 | §7.16 says the module has "its own left sidebar". | **A tab strip under the page header**, `?tab=` deep links. A second vertical sidebar next to the app's would crowd 1280 px and break the 07 §7 shell | Layout only; swap the tab strip for a vertical list |
| Q-F9 | Documents: 05 §12 has no document routes and F10a stored none. | **An explicit "not yet available" state** that says where files will live (F11's authorised file store) | The tab shows no upload until F11 |
| Q-F10 | Company Details: `GET /companies/{id}` is A CA CR only. | **A CA CR see name/address/sector from `/companies/{id}`, plus a settings link for editors. BA/BR/D see the session's company name and a note** that details are managed by company admins | BA sees less than the report header prints |
| Q-F11 | Legacy's entry modal offered a file attachment. | **No attachment.** 05 §12 has no field for it; description only | One missing optional field |

## Rulings (R320–R327)

| Id | Rule |
|---|---|
| **R320** | **Navigation.** Nav entry `carbon` → `/ekorm/carbon-footprint` (was `/ekorm/carbon`) with `permission: 'carbon.read'`. Sections and their `?tab=` values: `overview`, `selection`, `entry`, `status`, `reporting`, `database`, `company`, `ghg`, `iso`, `standards`, `documents`. An unknown value falls back to `overview`. |
| **R321** | **Building gate.** `overview`, `selection`, `entry`, `status` and `reporting` need a building. Without one they show `EmptyState` ("Önce bir bina seçin") next to the `ScopePicker` in the filter bar. |
| **R322** | **Overview.**<br>• Stat tiles: total (t CO₂e with 2 dp, and kg in the caption), activity count, registered sub-categories, highest source (sub-category label + kg).<br>• Charts: a bar chart by main category (8, zeros included); a stacked bar or bar of the three scopes with percentages; a grouped bar of 12 months (this year, previous year). All in kg CO₂e with data tables.<br>• Recent table (5): activity, category, date range, emission, scope, status badge. "Tümünü gör" switches to `status`.<br>• `pending_count > 0` → a caption that pending records are included (Q-F4). |
| **R323** | **Selection.** 8 groups × their subs as checkboxes, with the scope + ISO chips beside each. Save = `PUT` with the checked keys. Read-only roles see disabled checkboxes and no save button. |
| **R324** | **Entry.**<br>• Cards: one per selected sub, with the count of that building's records (`GET /carbon/activities?building_id&type=<sub>&limit=1` is not a count, so the counts come from one `GET /carbon/activities?building_id&limit=500` grouped client-side, noting "500+" when it is full).<br>• Dialog fields: period start/end (`DateRangePicker`), level 1/2/3 selects built from the sub's factors' `category_path` (a level is shown only when the chosen prefix has more than one option below it), unit (the factor's `base_unit` plus its conversions, by label), quantity (`NumberInput`, decimal string), description.<br>• The dialog shows the factor (value, unit, source, year) and the sub's scope + ISO (from `/carbon/activity-catalogue`), and says the emission is computed on save.<br>• Server 422 field errors map to the fields.<br>• Nothing selected → the empty state links to `selection`. |
| **R325** | **Status.**<br>• Table: period, sub-category, quantity + unit, emission (kg), scope, ISO, status (badge: pending neutral/clock, approved success/check, rejected danger/x), automated marker.<br>• Filters: status, scope. Server-paged by cursor.<br>• A CA row actions: approve, reject, edit (the same dialog), delete (confirm). Automated rows show no edit/delete (R306). |
| **R326** | **Reporting and database.**<br>• Reporting: form (type GHG/ISO, period with `DateRangePicker` ≤ 366 days, optional name) → `POST`. History table (name, created, period, type) with a PDF download per row (`downloadFile`, locale query).<br>• Database: searchable table (key, label, main category, base factor + unit, source + year, overridden badge). A CA "Değiştir" opens a dialog (value, source, year, URL) → `PATCH`. "Varsayılana dön" confirms → `POST reset`. |
| **R327** | **Reference pages.**<br>• `ghg` and `iso`: static explanatory text plus the mapping table rendered from `/carbon/activity-catalogue`, so they cannot drift from R301.<br>• `standards`: a static comparison table (GHG Protocol vs ISO 14064-1: purpose, boundary, categories, verification).<br>• All text in both locales. |

## File map

```
web/src/app/ekorm/carbon-footprint/page.tsx
web/src/features/carbon/
  carbon-page.tsx (+test)                 tabs, building gate, permissions            Task 1
  labels.ts                               sub/main/scope/iso label keys                Task 1
  overview-panel.tsx, overview-view.tsx   R322                                          Task 2
  selection-panel.tsx, selection-view.tsx R323                                          Task 3
  entry-panel.tsx, entry-cards.tsx, activity-dialog.tsx, cascade.ts                    Task 4
  status-panel.tsx, status-table.tsx      R325                                          Task 5
  reporting-panel.tsx, report-form.tsx, report-history.tsx                              Task 6
  database-panel.tsx, factor-table.tsx, factor-dialog.tsx                               Task 7
  company-panel.tsx, company-view.tsx, reference-view.tsx, documents-view.tsx          Task 8
web/messages/{tr,en}/carbon.json, messages/index.ts
web/src/components/shell/nav-config.ts
web/tests/e2e/carbon.spec.ts, responsive.spec.ts                                       Task 9
```

## Review Focus

1. **A cascade where a sub has one factor with an empty `category_path`.** It must still select that factor, never show an empty select (Task 4 `cascade.test.ts`).
2. **A read-only role (CR, BA, BR, D).** It sees the data but no mutation control, including the dialog triggers. A BA outside the building gets the API's 404 as "bulunamadı", not a crash (Task 1, 5).
3. **Quantity typed with a Turkish comma ("1.234,5").** It is sent as the decimal string "1234.5" (`NumberInput`'s value), and 0 or a negative value is refused client-side too (Task 4).
4. **Switching building with the dialog open.** The dialog closes; no record is created against the new building by accident (Task 4).
5. **A report period over 366 days or ending in the future.** It is disabled client-side with the reason, and the server's 422 is shown if reached (Task 6).

---

### Task 1: Module shell, navigation, i18n namespace

- [ ] Failing tests: `carbon-page.test.tsx`:
  - `?tab=database` opens the database tab;
  - an unknown tab → overview;
  - no building → the building empty state on `overview`, but `ghg` renders;
  - `can('carbon.read')` false → the no-access state.

  Update `nav-config` test expectations.
- [ ] Implement the page, the tabs (URL-synced with `useRouter().replace`), `labels.ts` (keys for 8 mains / 18 subs / 3 scopes / 6 ISO categories), `carbon.json` in both locales and the `messages/index.ts` registration. Fix the nav entry and the `SCREENS` line. Gates, then commit.

### Task 2: Overview (R322)
- [ ] Failing tests (view):
  - the four tiles;
  - highest-source label;
  - 8 category bars with zeros;
  - the pending caption only when > 0;
  - recent rows + "Tümünü gör" calls `onSeeAll`;
  - the empty year (total 0, no highest) says "Bu yıl için kayıt yok".
- [ ] Implement the view + panel (year select, current and 4 previous years). Stories: `Year`, `Empty`, `Loading`. Gates, then commit.

### Task 3: Activity selection (R323)
- [ ] Failing tests:
  - groups and checkboxes from the catalogue;
  - toggling and saving sends the sorted keys;
  - read-only → disabled, no button;
  - the scope/ISO chips per sub.
- [ ] Implement view + panel, stories `Editable`, `ReadOnly`. Gates, then commit.

### Task 4: Data entry (R324)
- [ ] Failing tests:
  - `cascade.ts`: levels from paths (1-, 2- and 3-level factors, an empty path, duplicates collapsed, the factor resolved only at a leaf);
  - `entry-cards`: counts, the empty state linking to `selection`;
  - `activity-dialog`:
    - the cascade drives the unit options;
    - the factor block shows value/unit/source/year;
    - scope + ISO shown;
    - submit sends `{building_id, sub_category, factor_key, unit, quantity, period_start, period_end, description}`;
    - field errors render;
    - quantity 0 is disabled;
    - edit mode is prefilled from an activity.
- [ ] Implement it. Stories: `Cards`, `NothingSelected`, `DialogOpen` (trigger + play), `DialogEdit`. Gates, then commit.

### Task 5: Activity status (R325)
- [ ] Failing tests:
  - rows with badges (icon + text);
  - approve/reject call the status route;
  - automated rows have no edit/delete;
  - delete asks first;
  - filters go to the query;
  - read-only shows no actions.
- [ ] Implement with the shared `activity-dialog` for edit. Stories: `Mixed`, `ReadOnly`, `Empty`. Gates, then commit.

### Task 6: Reporting (R326)
- [ ] Failing tests:
  - form validation (span > 366 days, future end, missing type);
  - submit body;
  - the history rows' download calls `downloadFile('/api/v1/carbon/reports/{id}/pdf', …)`;
  - the empty history.
- [ ] Implement, stories `Form`, `History`, `EmptyHistory`. Gates, then commit.

### Task 7: Emission factor database (R326)
- [ ] Failing tests:
  - search filters by label/key (server `q`);
  - the overridden badge + the platform value in the row;
  - the override dialog sends `{base_factor, source, source_year, source_url}`;
  - reset asks first;
  - read-only shows no actions.
- [ ] Implement, stories `Catalogue`, `Overridden`, `ReadOnly`, `OverrideDialog`. Gates, then commit.

### Task 8: Company details, reference pages, documents (R327, Q-F9, Q-F10)
- [ ] Failing tests:
  - the company view for CA (fields + a settings link) and for BA (the note);
  - the GHG/ISO tables rendered from a catalogue fixture (waste → Kategori 6);
  - the standards table rows;
  - the documents unavailable state.
- [ ] Implement, stories for each view. Gates, then commit.

### Task 9: e2e, a11y, self-review

- [ ] Write `tests/e2e/carbon.spec.ts` (company admin on the seeded company A, building A1):
  - the overview shows the seeded records;
  - select a sub, then add an activity through the dialog;
  - approve it on status;
  - generate a GHG report and download its PDF (HTTP 200, `application/pdf`);
  - override a factor, then reset;
  - a building admin sees no write controls.
- [ ] Run `pnpm build`, the spec, the full e2e suite with the worker (**PENDING while Docker hangs**), the Storybook build, and the a11y run for `Features/Carbon/*`.
- [ ] Self-review: permissions per role first, then a11y/design (07 §11), then the Review Focus. Ledger `Final:` lines, update the handoff, commit.
