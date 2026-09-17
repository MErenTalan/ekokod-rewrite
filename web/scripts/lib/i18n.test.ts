// @vitest-environment node
import { describe, expect, it } from 'vitest';

import { checkCatalogues } from './i18n.ts';

describe('checkCatalogues', () => {
  it('missing keys are reported per direction', () => {
    expect(checkCatalogues({ a: { x: '1' } }, { a: {} })).toEqual(['en/a.json missing a.x']);
    expect(checkCatalogues({ a: {} }, { a: { x: '1' } })).toEqual(['tr/a.json missing a.x']);
  });

  it('ICU arguments must match', () => {
    const problems = checkCatalogues({ a: { x: '{count} kayıt' } }, { a: { x: '{total} records' } });
    expect(problems).toHaveLength(1);
    expect(problems[0]).toContain('{count}');
    expect(problems[0]).toContain('{total}');
  });

  it('i18next double braces are rejected', () => {
    expect(checkCatalogues({ a: { x: '{{count}} yeni' } }, { a: { x: '{count} new' } })).toContain(
      'tr/a.json: a.x uses {{…}}',
    );
  });

  it('keys must be identifiers', () => {
    const problems = checkCatalogues({ a: { 'Carbon Footprint': 'x' } }, { a: { 'Carbon Footprint': 'x' } });
    expect(problems.some((p) => p.includes('invalid key') && p.includes('Carbon Footprint'))).toBe(true);
  });

  it('a namespace file missing in one locale is reported', () => {
    expect(checkCatalogues({ a: {}, forms: {} }, { a: {} })).toEqual(['en/forms.json missing']);
  });

  it('plural branches do not count as arguments', () => {
    expect(
      checkCatalogues(
        { a: { x: '{count, plural, one {# kayıt} other {# kayıt}}' } },
        { a: { x: '{count, plural, one {# record} other {# records}}' } },
      ),
    ).toEqual([]);
  });
});
