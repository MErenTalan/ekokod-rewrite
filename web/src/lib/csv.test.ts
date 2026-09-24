import { describe, expect, it } from 'vitest';

import { toCsv } from './csv';

describe('toCsv', () => {
  it('semicolon, CRLF, BOM and quoting', () => {
    expect(toCsv([['a;b', '1.234,5'], ['"q"', 'x']])).toBe('﻿"a;b";1.234,5\r\n"""q""";x');
  });

  it('escapes formula-injection prefixes', () => {
    expect(toCsv([['=SUM(A1)', '+1', '-1+cmd', '@x', '\tTAB', 'plain']])).toBe("﻿'=SUM(A1);'+1;'-1+cmd;'@x;'\tTAB;plain");
  });

  it('leaves Turkish-formatted numbers untouched, including negatives', () => {
    expect(toCsv([['-1.234,5', '-12', '0,5', '1.000']])).toBe('﻿-1.234,5;-12;0,5;1.000');
  });

  it('quotes cells with line breaks', () => {
    expect(toCsv([['satır\nikinci']])).toBe('﻿"satır\nikinci"');
  });
});
