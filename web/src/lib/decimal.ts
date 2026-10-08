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

/** Moves the decimal point: shiftDecimal('70.5', -3) → '0.0705' (a per-MWh price as per-kWh). */
export function shiftDecimal(value: string, places: number): string {
  const sign = value.startsWith('-') ? '-' : '';
  const [int, frac = ''] = value.replace('-', '').split('.');
  const point = int.length + places;
  const digits = `${'0'.repeat(Math.max(0, -point))}${int}${frac}${'0'.repeat(Math.max(0, point - int.length - frac.length))}`;
  const at = Math.max(point, 0);
  const whole = digits.slice(0, at).replace(/^0+(?=\d)/, '') || '0';
  const rest = digits.slice(at).replace(/0+$/, '');
  const out = `${whole}${rest ? `.${rest}` : ''}`;
  return out === '0' ? '0' : `${sign}${out}`;
}

/**
 * A typed decimal as the API's plain string: a Turkish comma becomes the point
 * ('2,45' → '2.45', '1.234,56' → '1234.56'); a lone dot stays a decimal point,
 * as the API has always read it. Text that is no number is returned trimmed,
 * so the API names the field instead of the form guessing.
 */
export function toDecimalString(text: string): string {
  const s = text.replace(/\s/g, '');
  const comma = s.lastIndexOf(',');
  const dot = s.lastIndexOf('.');
  let out = s;
  if (comma >= 0 && dot < 0) out = s.replace(',', '.');
  else if (comma > dot) out = s.replace(/\./g, '').replace(',', '.');
  else if (comma >= 0) out = s.replace(/,/g, '');
  return /^-?\d+(\.\d+)?$/.test(out) ? out : text.trim();
}

/** Whether toDecimalString found a number in the text. */
export const isDecimalText = (text: string) => /^-?\d+(\.\d+)?$/.test(toDecimalString(text));
