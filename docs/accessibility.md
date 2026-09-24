# Accessibility audit (09 §F15, 07 §9)

Target: WCAG 2.1 AA. The bar for release is no critical or serious axe finding (09 §F15). Run on
2026-09-24 against the F15c tip.

## What runs

| sweep | scope | result |
|---|---|---|
| Storybook (`make web-a11y`, `tests/a11y/stories.spec.ts`) | 577 stories. Each component's first story and every open overlay run in both themes × both locales; the rest run light/tr and dark/en. Axe tags wcag2a/aa, 21a/aa, 22aa. Also checks no horizontal scroll and no clipped text. | 2,730 passed |
| Keyboard, focus, motion, touch targets, shell (`tests/a11y/*.spec.ts`) | focus order and traps, Escape restores focus, reduced motion, 44 px pointer-coarse targets, skip link | 1,186 passed |
| Pages (`tests/e2e/a11y-pages.spec.ts`, R481) | every `/ekorm` route as demo, platform admin and building read-only; every `/auth` page as a visitor; real API and data | 60 passed, **0 findings of any impact** |
| Public site (`tests/e2e/public/site.spec.ts`) | every public page in Turkish | passed |

## What the audit fixed

- `/auth/error` and `/auth/maintenance` failed to render: a server component called
  `buttonVariants` from a client module. The page sweep caught it; unit tests (jsdom) could not.
- The AI analysis page had no heading for users who cannot run forecasts.

## Not covered by automation

Axe finds about a third of WCAG issues. Screen-reader passes (NVDA + Firefox, VoiceOver + Safari)
over login, the dashboard, consumption, bills and settings are **still to do**. They need a person
and real assistive technology.
