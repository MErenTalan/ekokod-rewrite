import { FlatCompat } from '@eslint/eslintrc';

import owners from './src/modules/feature-owners.json' with { type: 'json' };

const compat = new FlatCompat({ baseDirectory: import.meta.dirname });

const config = [
  ...compat.extends('next/core-web-vitals', 'next/typescript'),
  { ignores: ['.next/**', 'next-env.d.ts', 'node_modules/**', 'storybook-static/**', 'playwright-report/**', 'test-results/**', 'public/maplibre/**'] },
];

// Design-system guards (07 §2.4, §5, §9; plan D3, D4, D5, D10, D23).
const RAW_COLOR = String.raw`(?<![\w&-])#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})(?![\w-])|\b(?:rgba?|hsla?|oklch|oklab|lab|lch|hwb)\(`;
const PALETTE = String.raw`(?:^|[\s:])(?:bg|text|border|ring|fill|stroke|outline|decoration|divide|accent|caret|placeholder|from|via|to|shadow)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|black|white)(?:-\d{2,3})?(?:\/\d+)?(?=\s|$)`;
const ENERGY_TEXT = String.raw`(?:^|[\s:])text-(?:brand|consumption|generation|reactive-inductive|reactive-capacitive|cost|revenue|forecast)(?=\s|$)`;
const FORBIDDEN = String.raw`(?:^|[\s:])(?:outline-none|outline-hidden|outline-0|\[outline:(?:none|0)\])(?=\s|$)|(?:^|\s)dark:`;
const ban = (re, message) => [
  { selector: `Literal[value=/${re}/]`, message },
  { selector: `TemplateElement[value.raw=/${re}/]`, message },
];
const LIBRARY_PATHS = [
  { name: 'recharts', message: 'Use src/components/charts (07 §5, plan D10).' },
  { name: 'maplibre-gl', message: 'Use src/components/map (plan D23).' }];
// F17 R490: a module never imports another module's features; shared code imports neither.
const featureDirs = (ids) => ids.flatMap((f) => [`src/features/${f}/**`]);
const noFeatures = (ids, message) => ['error', { paths: LIBRARY_PATHS, patterns: [
  { group: ids.flatMap((f) => [`@/features/${f}`, `@/features/${f}/**`]), message }] }];
config.push(
  { files: ['src/**/*.{ts,tsx}'], ignores: ['src/styles/**'], rules: {
    'no-restricted-syntax': ['error',
      ...ban(RAW_COLOR, 'Raw colour literal: use a design token (07 §2.4, plan D5).'),
      ...ban(PALETTE, 'Tailwind default palette: use a token utility (plan D5).'),
      ...ban(ENERGY_TEXT, 'Energy colours are graphics only, never text colour (plan D3, M-1).'),
      ...ban(FORBIDDEN, 'outline-none / dark: are forbidden: focus rings are global, dark mode is token-driven (plan D4).')],
    'no-restricted-imports': ['error', { paths: LIBRARY_PATHS }],
  } },
  { files: ['src/app/ekocm/**', 'src/modules/ekocm/**', ...featureDirs(owners.ekocm)],
    rules: { 'no-restricted-imports': noFeatures(owners.ekorm, 'EKOCM code must not import EKORM features (F17 R490).') } },
  { files: ['src/app/ekorm/**', 'src/modules/ekorm/**', ...featureDirs(owners.ekorm)],
    rules: { 'no-restricted-imports': noFeatures(owners.ekocm, 'EKORM code must not import EKOCM features (F17 R490).') } },
  { files: ['src/components/**', 'src/lib/**', 'src/modules/*.{ts,tsx}', ...featureDirs(owners.shared)],
    ignores: ['**/*.stories.tsx', '**/*.test.{ts,tsx}'],
    rules: { 'no-restricted-imports': noFeatures([...owners.ekorm, ...owners.ekocm], 'Shared code must not import a module’s features (F17 R490).') } },
  { files: ['src/components/charts/**'], rules: { 'no-restricted-imports': 'off' } },
  { files: ['src/components/map/**'], rules: { 'no-restricted-imports': 'off' } },
);

export default config;
