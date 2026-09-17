# F5 pre-delivery checklist (07-design-system.md §11)

Run on `phase/f5-design-system` (F5 Task 8), 2026-09-17. Each item names the evidence that proves it. Guards were
proven red by mutation in their task (ledger: `.superpowers/sdd/2026-09-17-f5-design-system/inline-ledger.md` in the
main checkout).

- [x] **`ui-ux-pro-max` was invoked and its master design system read.**
  `design-system/bcem-energy/MASTER.md` header `**Generated:** 2026-09-17 09:41:38` (skill run with
  `--design-system --density 8 --motion 4 --variance 5 -p "BCEM Energy" --persist`); `OVERRIDES.md` resolves every
  conflict with 07 (D1). Read before T1 and before every component task.
- [x] **Zero raw hex values in components — tokens only.**
  `pnpm lint` (ESLint `no-restricted-syntax`: colour literals, palette classes, energy text colours,
  `outline-none|outline-hidden|outline-0|[outline:none]`, `dark:`), `src/styles/raw-color-lint.test.ts` (17 cases),
  `src/styles/raw-color-css.test.ts`, and
  `git grep -nE "#[0-9a-fA-F]{6}\b" -- 'web/src/**' ':!web/src/styles/tokens.css' ':!web/src/styles/*.test.ts'` → empty.
- [x] **Light and dark both verified, contrast checked automatically.**
  `pnpm check:contrast` → `contrast ok — 70 pairs × 2 themes; lowest generation/background light 3.13` (every
  `--color-*` covered or exempt with a reason); `stories.spec.ts` axe `color-contrast` over every story in both themes
  (full sweep: 1,102 Playwright checks over 224 stories, 0 failures); shell screenshots in both themes reviewed by hand.
- [x] **Turkish and English verified; no truncation or overflow in either.**
  `stories.spec.ts` runs tr and en (full matrix for each component's first story and every open overlay, diagonal
  light/tr + dark/en for the rest) with `expectNoClippedText` and `expectNoHorizontalScroll`; `LongTurkishLabel` stories
  across the library; `check:i18n-parity` → `11 namespaces, 218 keys in both locales`; next-intl `onError` rethrows, so
  a missing message fails the story.
- [x] **Every interactive element keyboard reachable with a visible focus ring.**
  `keyboard.spec.ts` over every story (all tabbables reached by Tab/Shift+Tab, computed outline ≥ 2 px; overlays: focus
  starts inside, first arrow-key item shows the ring, Escape restores focus); global `:focus-visible` ring in
  `globals.css`; component keyboard tests (Select, Combobox, RadioGroup, Tabs, DropdownMenu, Dialog, Slider, Switch).
- [x] **Icons are SVG; no emoji.** Lucide icons with `aria-hidden` throughout;
  `git grep -nIP "[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]" -- web/src web/messages` → empty.
- [x] **`cursor-pointer` on everything clickable; hover transitions 150–300 ms.**
  `globals.css` base rule (buttons, links, `summary`, `select`, `label[for]`, tab/menuitem/option roles; `not-allowed`
  when disabled); `--duration-hover: 150ms`; `reduced-motion.spec.ts` asserts Button `transition-duration: 0.15s`.
- [x] **`prefers-reduced-motion` honoured.** Global reduce rule in `globals.css`; `reduced-motion.spec.ts` (button
  transition and tooltip animation collapse), `overlay-motion.spec.ts` (Dialog 250 ms in, collapses under reduce),
  `charts/_theme.test.ts` + `line-chart.test.tsx` (Recharts animation off), `use-reduced-motion.test.ts`.
- [x] **Responsive at 375 / 768 / 1024 / 1440 px, no horizontal page scroll.**
  `shell.spec.ts`: 4 layouts (vertical, horizontal, collapsed, boxed) × 4 widths × 2 themes — no horizontal scroll,
  navigation docked at ≥1024 and in a drawer below, disabled-entry tooltip, axe, skip link first; screenshots
  `web/test-results/shell-<layout>-<width>-<theme>.png` (not committed). Mutant (sidebar docked at every width) → red at 375.
- [x] **Loading skeletons sized to the final content — no layout shift.**
  `_chart-frame.test.tsx` › loading keeps the final height (320 px); `data-table.test.tsx` › 5 skeleton rows at row
  height; MetricCard/Card skeletons sized by the caller.
- [x] **Every empty state written, with a next step.** `EmptyState` requires `title` and `description`
  (`empty-state.test.tsx`); `Empty` stories for DataTable, charts, pickers, InvoiceLineTable; charts never draw an empty
  axis frame (`_chart-frame.test.tsx`).
- [x] **Every error state written, actionable, and near the thing that failed.** `field.test.tsx` (error under the field,
  `aria-describedby`, `aria-invalid`), `form-error-summary.test.tsx` (focus + links to fields), `file-upload.test.tsx`
  (type and size errors), `Error` stories for every form control, JobStatusBanner failure message.
- [x] **Every chart has units, a legend, tooltips and a data-table equivalent.** `_chart-legend.test.tsx` (label + unit +
  dash sample; bars get solid swatches), `_chart-tooltip.test.tsx` (every series with units), `_chart-frame.test.tsx`
  (toggleable exact table over full data), axis unit labels in `line-chart.test.tsx`; keyboard.spec reaches the table
  toggle in every chart story.
- [x] **Numbers use tabular figures and Turkish formatting.** `type-data`/`type-metric` (JetBrains Mono, tabular, no
  ligatures); `src/lib/format.test.ts` (tr-TR separators in both locales, decimal strings kept exact);
  `invoice-line-table.test.tsx`, `metric-card.test.tsx`, `number-input.test.tsx` (Turkish input → decimal string).
- [x] **Any figure derived from incomplete data carries a `DataQualityBadge`.** `data-quality-badge.test.tsx`
  (estimated/incomplete/suspect, keyboard-reachable reason and coverage), `metric-card.test.tsx` › quality badge shows.

**Checked by hand (not automatable):** the visual quality of both themes and the ≥ 8 px spacing between touch targets,
from Playwright screenshots of the shell (4 layouts × 4 widths × 2 themes), charts (line, forecast band, diverging bar,
stacked bar, combo, heatmap, gauge) and domain components (MetricCard, ReactiveStatusCard, InvoiceLineTable). Two
visual defects found this way were fixed before sign-off (dashed legend swatches for bars; a clipped direct label).
