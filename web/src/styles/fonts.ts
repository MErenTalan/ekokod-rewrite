import localFont from 'next/font/local';

// Self-hosted subsets built by scripts/subset-fonts.sh (07 §3, plan D6).
const lexend = localFont({
  src: './fonts/lexend.woff2',
  variable: '--font-lexend',
  display: 'swap',
  weight: '100 900',
});
const sourceSans3 = localFont({
  src: './fonts/source-sans-3.woff2',
  variable: '--font-source-sans-3',
  display: 'swap',
  weight: '200 900',
});
const jetbrainsMono = localFont({
  src: './fonts/jetbrains-mono.woff2',
  variable: '--font-jetbrains-mono',
  display: 'swap',
  // Figures only: not preloaded, so it never competes with the text fonts for first paint (F12b LCP).
  preload: false,
  weight: '100 800',
});

export const fontVariables = [lexend.variable, sourceSans3.variable, jetbrainsMono.variable].join(' ');
