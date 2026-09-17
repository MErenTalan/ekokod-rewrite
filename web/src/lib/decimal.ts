// Plain decimal strings ('-12.345') compared and scaled without floats (plan D11).

/** Sign-aware comparison of two plain decimal strings. */
export function compareDecimal(a: string, b: string): number {
  const neg = (s: string) => s.startsWith('-');
  if (neg(a) !== neg(b)) return neg(a) ? -1 : 1;
  const sign = neg(a) ? -1 : 1;
  const [ai, af = ''] = a.replace('-', '').split('.');
  const [bi, bf = ''] = b.replace('-', '').split('.');
  const [ia, ib] = [ai.replace(/^0+(?=\d)/, ''), bi.replace(/^0+(?=\d)/, '')];
  if (ia.length !== ib.length) return (ia.length > ib.length ? 1 : -1) * sign;
  const width = Math.max(af.length, bf.length);
  const [x, y] = [ia + af.padEnd(width, '0'), ib + bf.padEnd(width, '0')];
  return x === y ? 0 : (x > y ? 1 : -1) * sign;
}

/** '0.25' → '25': a ratio shown as percent, by shifting the decimal point two places. */
export function fractionToPercent(fraction: string): string {
  const sign = fraction.startsWith('-') ? '-' : '';
  const [int, frac = ''] = fraction.replace('-', '').split('.');
  const digits = frac.padEnd(2, '0');
  const whole = `${int}${digits.slice(0, 2)}`.replace(/^0+(?=\d)/, '');
  const rest = digits.slice(2).replace(/0+$/, '');
  return `${sign}${whole}${rest ? `.${rest}` : ''}`;
}
