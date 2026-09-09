# 07 — Design System

## 0. The rule that governs this document

**All UI work goes through the `ui-ux-pro-max` skill.** Do not invent a visual language, do not
accept a component library's defaults, and do not carry over the legacy admin-template look.

Before writing UI code in any phase that touches the interface, run:

```bash
# once per project, at the start of Phase F5 — persist the master
python "<skill>/scripts/search.py" "energy monitoring sustainability analytics dashboard" \
  --design-system --density 8 --motion 4 --variance 5 \
  -p "BCEM Energy" --persist --output-dir "<project-root>"

# per page, when a screen needs its own treatment
python "<skill>/scripts/search.py" "<page intent>" --design-system --page "<page-name>" \
  -p "BCEM Energy" --output-dir "<project-root>"

# focused concerns
python "<skill>/scripts/search.py" "<query>" --domain ux|color|typography|chart|icons
python "<skill>/scripts/search.py" "<query>" --stack nextjs
```

The persisted `design-system/bcem-energy/MASTER.md` becomes the source of truth. Page-level
overrides live in `design-system/bcem-energy/pages/`. Re-read the master before building each page.
Never overwrite it with `--force` without asking.

The direction below is the **starting point** produced by that skill for this product. It is a
brief, not a substitute for running the skill.

---

## 1. Brand position

BCEM Energy measures, prices and reduces energy. Everything the product does ends in a number a
customer will act on — an invoice they will challenge, a penalty they will fix, a target they will
chase, emissions they will cut.

The visual language must read as:

| Yes | No |
|-----|-----|
| Precise, measured, trustworthy | Playful, decorative |
| Environmental, alive, forward-looking | Corporate grey, generic admin panel |
| Dense but calm — a lot of data, no anxiety | Sparse marketing-site whitespace in the dashboard |
| Turkish-market professional | Silicon-Valley-startup pastel |

**Green is the identity, not the decoration.** The product's subject is renewable energy, carbon
reduction and efficiency. Green carries the brand — but it must never collide with the *semantic*
green that means "good / within limits" in status indicators. That separation is solved in §2.

---

## 2. Colour

### 2.1 The green problem, solved

Two greens with different jobs:

- **Brand green** (`emerald`) — identity: primary actions, active navigation, logo, brand surfaces.
- **Status green** (`lime-shifted success`) — meaning: "within limits", "target met", "healthy".

They are deliberately different hues so a green button never reads as a status, and a healthy
badge never reads as a call to action. Status colours are used **only** for status.

### 2.2 Light theme (default)

```css
:root {
  /* brand */
  --color-primary:            #059669;  /* emerald 600 */
  --color-primary-hover:      #047857;
  --color-primary-subtle:     #D1FAE5;
  --color-on-primary:         #FFFFFF;

  /* surfaces */
  --color-background:         #F6FAF8;  /* barely-green off-white */
  --color-surface:            #FFFFFF;
  --color-surface-raised:     #FFFFFF;
  --color-surface-sunken:     #ECF3EF;

  /* text */
  --color-foreground:         #0B2A22;  /* deep green-black, not pure black */
  --color-foreground-muted:   #4B5D57;
  --color-foreground-subtle:  #6B7C76;

  /* lines */
  --color-border:             #DCE7E2;
  --color-border-strong:      #B9CCC4;
  --color-ring:               #059669;

  /* status — meaning only, never branding */
  --color-success:            #15803D;
  --color-success-subtle:     #DCFCE7;
  --color-warning:            #B45309;
  --color-warning-subtle:     #FEF3C7;
  --color-danger:             #B91C1C;
  --color-danger-subtle:      #FEE2E2;
  --color-info:               #0369A1;
  --color-info-subtle:        #E0F2FE;

  /* energy-domain semantics */
  --color-consumption:        #0E7490;  /* grid import — cyan */
  --color-generation:         #16A34A;  /* solar production — green */
  --color-reactive-inductive: #A16207;  /* amber */
  --color-reactive-capacitive:#7E22CE;  /* violet */
  --color-cost:               #B91C1C;  /* money out */
  --color-revenue:            #15803D;  /* money in */
  --color-forecast:           #64748B;  /* prediction — always neutral, always dashed */
}
```

### 2.3 Dark theme

Not a filter over the light theme — its own set of values, tuned so charts and status colours stay
legible on a dark ground.

```css
:root[data-theme="dark"] {
  --color-primary:            #34D399;
  --color-primary-hover:      #6EE7B7;
  --color-primary-subtle:     #064E3B;
  --color-on-primary:         #04211A;

  --color-background:         #0B1512;
  --color-surface:            #12201C;
  --color-surface-raised:     #172823;
  --color-surface-sunken:     #0A100E;

  --color-foreground:         #E8F2EE;
  --color-foreground-muted:   #9CB3AB;
  --color-foreground-subtle:  #7A8F88;

  --color-border:             #23372F;
  --color-border-strong:      #354E44;
  --color-ring:               #34D399;

  --color-success:            #4ADE80;
  --color-success-subtle:     #052E16;
  --color-warning:            #FBBF24;
  --color-warning-subtle:     #3B2A06;
  --color-danger:             #F87171;
  --color-danger-subtle:      #3B0D0D;
  --color-info:               #38BDF8;
  --color-info-subtle:        #06283B;

  --color-consumption:        #22D3EE;
  --color-generation:         #4ADE80;
  --color-reactive-inductive: #FBBF24;
  --color-reactive-capacitive:#C084FC;
  --color-cost:               #F87171;
  --color-revenue:            #4ADE80;
  --color-forecast:           #94A3B8;
}
```

### 2.4 Rules

1. **Semantic tokens only.** No raw hex in a component, ever. A component that needs a colour the
   token set does not have means the token set is incomplete — extend it deliberately.
2. **Contrast.** Body text ≥ 4.5:1, large text and UI chrome ≥ 3:1, in **both** themes. Verified in
   CI with an automated contrast check over the token pairs.
3. **Never colour alone.** Every status is colour **plus** an icon **plus** text. A red badge with
   no label is not acceptable — roughly 8 % of male users cannot rely on the hue.
4. **Energy semantics are constant.** Consumption is always cyan, generation always green, inductive
   always amber, capacitive always violet — on every chart, every table cell, every legend, every
   page. A user learns the mapping once.
5. **Forecast is always neutral and always dashed**, so a prediction is never mistaken for a
   measurement.

---

## 3. Typography

**Pairing: Corporate Trust** — chosen over the skill's `Fira Code + Fira Sans` default because this
product is read in Turkish, with long labels and dense tables, where a monospace heading is a
liability rather than a signal.

| Role | Family | Notes |
|------|--------|-------|
| Headings and UI | **Lexend** | Designed for reading efficiency; full Latin Extended, so Turkish `ğ ş ı İ ç ö ü` render correctly. |
| Body and labels | **Source Sans 3** | Highly legible at small sizes, wide weight range, excellent Turkish coverage. |
| Numerals in tables and metrics | **JetBrains Mono** | **Tabular figures.** Numbers in a data table must align on the decimal point. |

Self-host all three (offline installs have no access to Google Fonts) and subset to Latin +
Latin Extended-A.

### Scale

```
display   32 / 40   Lexend 600
h1        24 / 32   Lexend 600
h2        20 / 28   Lexend 600
h3        16 / 24   Lexend 600
body      14 / 20   Source Sans 3 400     ← dashboard default (density 8)
body-lg   16 / 24   Source Sans 3 400     ← public site default
small     13 / 18   Source Sans 3 400
caption   12 / 16   Source Sans 3 500
metric    28 / 32   JetBrains Mono 600, tabular
data      13 / 18   JetBrains Mono 400, tabular
```

Body text never goes below 13 px in the dashboard or 16 px on the public site. Turkish is roughly
10–15 % longer than English — every layout is tested against the Turkish catalogue, not the English
one.

---

## 4. Spacing, radius, elevation

Dense-dashboard scale (`--density 8`):

```
--space-1: 4px    --space-2: 8px    --space-3: 12px   --space-4: 16px
--space-5: 20px   --space-6: 24px   --space-8: 32px   --space-12: 48px

--radius-sm: 4px  --radius-md: 8px  --radius-lg: 12px --radius-full: 9999px

--shadow-sm: 0 1px 2px rgb(11 42 34 / .06);
--shadow-md: 0 4px 12px rgb(11 42 34 / .08);
--shadow-lg: 0 12px 32px rgb(11 42 34 / .10);
```

Dark theme replaces shadows with a lighter border and a raised surface tone — shadows are close to
invisible on a dark ground.

The public marketing site uses a **spacious** scale (24–96 px) — the two contexts are deliberately
different: marketing breathes, the dashboard works.

---

## 5. Charts

Charts are the product. They get the most attention.

### One library, one theme

A **single** charting library across the entire application, wrapped in project components that
read the design tokens. The legacy product used four libraries simultaneously and looked like four
products; that is not repeated.

Wrappers: `<LineChart>`, `<AreaChart>`, `<BarChart>`, `<StackedBarChart>`, `<ComboChart>`,
`<GaugeChart>`, `<HeatmapChart>`, `<Sparkline>`. A page never touches the library directly.

### Chart selection

| Question | Chart |
|----------|-------|
| Consumption over time | Line (area only when a single series) |
| Consumption vs. generation over time | Combo: bars for consumption, line for generation |
| This year vs. last year | Grouped bar, current year in brand green, prior year in muted grey |
| Load profile (24-hour shape) | Line, 0–23 on the x-axis, one line per profile with distinct dash patterns |
| Emissions by category | Horizontal bar sorted descending — **never** a pie or donut beyond 3 slices |
| Scope 1/2/3 split | Stacked bar or a 3-segment donut with direct labels |
| Target vs. actual production | Grouped bar with a target reference line |
| Forecast | Line for the median plus a shaded p10–p90 band, both neutral and dashed |
| Reactive ratio against limits | Gauge or bullet with the threshold marked |
| Invoice composition | Stacked horizontal bar, one segment per charge line |
| Grid import/export balance | Diverging bar around a zero baseline |

### Chart rules

- **Every chart is accompanied by its data table**, either beside it or behind a toggle. This is
  both an accessibility requirement and what an energy manager actually wants.
- **Axis units are always labelled** — `kWh`, `kVArh`, `kW`, `₺`, `tCO₂e`. A bare number is a bug.
- **Turkish number formatting**: `1.234,56`. Currency `₺1.234,56`.
- Series are distinguished by **colour and dash pattern and direct labels**, never colour alone.
- Tooltips show every series at the hovered point, with units, not just the nearest one.
- More than 6 series means a different visualisation is needed.
- Above ~1,000 points, downsample and say so in the chart footnote.
- Loading is a skeleton at the chart's final size — the layout must not shift when data arrives.
- Empty state says what is missing and what to do about it, never an empty axis frame.

---

## 6. Components

Built on **Radix UI primitives + Tailwind**, styled entirely from the tokens. No component library's
visual defaults survive contact with the design system.

### Core inventory

`Button` (primary, secondary, ghost, danger; sm/md/lg; loading; icon) · `IconButton` (always with an
accessible label) · `Input` `Select` `Combobox` `MultiSelect` `DatePicker` `DateRangePicker`
`MonthPicker` `NumberInput` `Switch` `Checkbox` `RadioGroup` `Textarea` · `Card` `MetricCard`
`StatTile` · `Table` (sortable, sticky header, column visibility, pagination, row actions, export)
· `Tabs` `Accordion` `Dialog` `Drawer` `Popover` `Tooltip` `DropdownMenu` · `Badge` `StatusBadge`
`Toast` `Alert` `EmptyState` `Skeleton` `ProgressBar` `Stepper` · `Breadcrumb` `Pagination`
`SearchInput` · `FileUpload` `FileList` · `Map` `MapMarker` · the chart wrappers.

### Domain components

These recur across the product and are built once:

- **`BuildingAnalyzerPicker`** — the building → analyzer cascade used on almost every analysis page,
  with search, "active only" and remembered selection.
- **`PeriodFilterBar`** — granularity + date range + apply, with sensible presets (last 7 days,
  last month, last 6 months, this year, last year).
- **`MetricCard`** — label, big tabular number, unit, delta versus the comparison period with an
  arrow and colour, and an optional sparkline.
- **`ReactiveStatusCard`** — ratios against thresholds, applied/not-applied, advisory text.
- **`InvoiceLineTable`** — the charge breakdown, used on screen and in the PDF.
- **`ExportMenu`** — CSV / Excel / PDF / e-mail, uniform everywhere.
- **`JobStatusBanner`** — surfaces a running background job and its result.
- **`DataQualityBadge`** — flags a period as estimated, incomplete or suspect. Given the product's
  history of silently wrong numbers, **any figure derived from incomplete data must be visibly
  marked**.

---

## 7. Layout

### Dashboard shell

```
┌──────────────────────────────────────────────────────────────────┐
│ Top bar: logo · search · notifications · language · theme · user │
├──────────┬───────────────────────────────────────────────────────┤
│ Sidebar  │ Breadcrumb                                            │
│ grouped  │ ┌───────────────────────────────────────────────────┐ │
│ nav,     │ │ Page header: title, description, primary actions  │ │
│ collaps- │ ├───────────────────────────────────────────────────┤ │
│ ible     │ │ Filter bar (sticky)                               │ │
│          │ ├───────────────────────────────────────────────────┤ │
│          │ │ Metric row                                        │ │
│          │ ├───────────────────────────────────────────────────┤ │
│          │ │ Content: charts, tables, tabs                     │ │
│          │ └───────────────────────────────────────────────────┘ │
└──────────┴───────────────────────────────────────────────────────┘
```

Sidebar groups, matching the legacy information architecture:
**Dashboard** · **Data Analysis** (Consumption · Load Profile · Forecast · Solar Plants · Financial
Analysis · Renewable Energy · *Water, Gas, EV Drivers — disabled*) · **Bills and Tariffs** ·
**Alarms** (Manual · Messages · *AI — disabled*) · **Reports** · **Settings** · **Carbon Footprint**
· **ISO 50001** · *Saving Actions — disabled* · **Contact**

Disabled items stay visible, visibly disabled, with a tooltip explaining they are not yet available.

### Grid and breakpoints

12-column grid, `--space-6` gutters.

```
sm  640px   single column, filters collapse into a sheet
md  768px   two columns, sidebar becomes an overlay
lg  1024px  sidebar docked, 2–3 column dashboard
xl  1280px  full dashboard layout
2xl 1536px  max content width 1600px, centred
```

Tables scroll horizontally inside their own container. The page body never scrolls sideways.

### Theme customiser

The legacy customiser (light/dark, LTR/RTL, theme colour, vertical/horizontal layout, boxed/full,
sidebar collapse, card border/shadow, border radius) is preserved as a **settings panel**, backed
by CSS custom properties and persisted per user.

---

## 8. Motion

Standard tier (`--motion 4`). Motion conveys meaning; it never performs.

| Interaction | Duration | Easing |
|-------------|----------|--------|
| Hover, focus | 150 ms | `ease-out` |
| Dropdown, popover, tooltip | 180 ms | `ease-out` |
| Dialog, drawer | 250 ms in / 180 ms out | `ease-out` / `ease-in` |
| Tab, page transition | 200 ms | `ease-in-out` |
| Chart series draw | 400 ms | `ease-out`, once on load only |
| Card grid stagger | 300–450 ms, 60 ms apart | `ease-out` |

Rules: animate `transform` and `opacity` only. Exit is faster than entry. **`prefers-reduced-motion`
is honoured everywhere** — animations resolve immediately to their final state. Never animate a
data table or a number the user is trying to read.

---

## 9. Accessibility — non-negotiable

- Contrast ≥ 4.5:1 body, ≥ 3:1 large text and UI, in both themes. Automated in CI.
- Every interactive element is keyboard reachable, with a **visible** focus ring
  (`--color-ring`, 2 px, 2 px offset). Focus outlines are never removed.
- Touch targets ≥ 44 × 44 px with ≥ 8 px spacing.
- Every form field has a **visible** label. Placeholders are never labels.
- Validation errors appear next to the field **and** in a summary at the top of the form, and are
  announced.
- Icon-only buttons carry an `aria-label`. Decorative icons are `aria-hidden`.
- Charts have an accessible name, a description, and a data-table equivalent.
- Live regions announce toasts, job completion and alarm notifications.
- Modals trap focus and restore it on close. `Escape` closes.
- Language is declared per document and switches with the locale.
- Icons are **SVG** (Lucide or Heroicons). **Never emoji as icons.**

---

## 10. Public marketing site

Same tokens, different rhythm.

- Spacious scale (24–96 px), `body-lg` 16 px base, generous section padding.
- Hero: the product's purpose in one sentence, a real product screenshot (never a stock
  illustration), and one primary call to action.
- Section order: hero → key metrics / social proof → features → carbon module → references →
  documents → news → toolkit → testimonials → leadership → FAQ → pricing teaser → demo CTA →
  contact → footer. Announcement bar above the header.
- Glassmorphism accents are acceptable here — frosted cards over a soft green gradient, 10–20 px
  backdrop blur, 1 px light border. **Not in the dashboard**, where blur costs legibility.
- Real photography of installations and plants wherever possible; illustration only as a fallback.
- Performance budget: LCP < 2.5 s, CLS < 0.1. Images WebP/AVIF with reserved dimensions.
- Every marketing page is fully readable and usable in Turkish first.

---

## 11. Pre-delivery checklist

Run this before any UI phase is called complete.

- [ ] `ui-ux-pro-max` was invoked and its master design system read
- [ ] Zero raw hex values in components — tokens only
- [ ] Light and dark both verified, contrast checked automatically
- [ ] Turkish **and** English verified; no truncation or overflow in either
- [ ] Every interactive element keyboard reachable with a visible focus ring
- [ ] Icons are SVG; no emoji
- [ ] `cursor-pointer` on everything clickable; hover transitions 150–300 ms
- [ ] `prefers-reduced-motion` honoured
- [ ] Responsive at 375 / 768 / 1024 / 1440 px, no horizontal page scroll
- [ ] Loading skeletons sized to the final content — no layout shift
- [ ] Every empty state written, with a next step
- [ ] Every error state written, actionable, and near the thing that failed
- [ ] Every chart has units, a legend, tooltips and a data-table equivalent
- [ ] Numbers use tabular figures and Turkish formatting
- [ ] Any figure derived from incomplete data carries a `DataQualityBadge`
