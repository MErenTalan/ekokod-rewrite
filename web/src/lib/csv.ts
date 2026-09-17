// Client CSV for Turkish Excel: `;` delimiter, CRLF, UTF-8 BOM (plan D22).
const TURKISH_NUMBER = /^-?\d{1,3}(?:\.\d{3})*(?:,\d+)?$|^-?\d+(?:,\d+)?$/;
const FORMULA_START = /^[=+\-@\t\r]/;

function cell(value: string): string {
  // A formatted number is never a formula; anything else starting with a formula character is neutralised.
  const safe = FORMULA_START.test(value) && !TURKISH_NUMBER.test(value) ? `'${value}` : value;
  return /[;"\r\n]/.test(safe) ? `"${safe.replaceAll('"', '""')}"` : safe;
}

export function toCsv(rows: string[][]): string {
  return `﻿${rows.map((row) => row.map(cell).join(';')).join('\r\n')}`;
}
