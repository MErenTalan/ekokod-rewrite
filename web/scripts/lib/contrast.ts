// WCAG 2.x contrast over the light-dark() token declarations in tokens.css (07 §2.4, plan D3/D4).
export type ThemeValues = { light: string; dark: string };
export type Pair = readonly [fg: string, bg: string, min: number];

const HEX = /^#[0-9A-Fa-f]{6}$/;

function luminance(hex: string): number {
  const channel = (i: number) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(1) + 0.7152 * channel(3) + 0.0722 * channel(5);
}

export function contrastRatio(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

/** Every `--color-<name>:` declaration; values that are not light-dark(#hex, #hex) keep their raw text. */
export function parseTokens(css: string): Record<string, ThemeValues> {
  const out: Record<string, ThemeValues> = {};
  for (const [, name, value] of css.matchAll(/--color-([a-z][a-z-]*):\s*([^;]+);/g)) {
    const m = /^light-dark\(\s*(#[0-9A-Fa-f]{6})\s*,\s*(#[0-9A-Fa-f]{6})\s*\)$/.exec(value.trim());
    out[name] = m ? { light: m[1], dark: m[2] } : { light: value.trim(), dark: value.trim() };
  }
  return out;
}

export function evaluate(tokens: Record<string, ThemeValues>, pairs: readonly Pair[], exempt: Record<string, string>) {
  const problems: string[] = [];
  const covered = new Set<string>();
  for (const [fg, bg, min] of pairs) {
    covered.add(fg).add(bg);
    const f = tokens[fg];
    const b = tokens[bg];
    if (!f || !b) {
      problems.push(`unknown token in pair: ${!f ? fg : bg}`);
      continue;
    }
    for (const theme of ['light', 'dark'] as const) {
      if (!HEX.test(f[theme]) || !HEX.test(b[theme])) {
        problems.push(`${fg}/${bg} ${theme} is not a light-dark(#hex, #hex) value`);
        continue;
      }
      const ratio = contrastRatio(f[theme], b[theme]);
      if (ratio < min) problems.push(`${fg}/${bg} ${theme} ${ratio.toFixed(2)} < ${min}`);
    }
  }
  for (const name of Object.keys(tokens)) {
    if (!covered.has(name) && !(name in exempt)) problems.push(`uncovered token: ${name}`);
  }
  return problems;
}
