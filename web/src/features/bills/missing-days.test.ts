import { describe, expect, it } from 'vitest';

import { dayRanges } from './missing-days';

describe('dayRanges', () => {
  it('collapses consecutive days, across a month end', () => {
    expect(dayRanges(['2026-09-01', '2026-09-02', '2026-09-03', '2026-09-05', '2026-09-30', '2026-10-01'])).toEqual([
      '01.09.2026 – 03.09.2026', '05.09.2026', '30.09.2026 – 01.10.2026',
    ]);
    expect(dayRanges([])).toEqual([]);
  });
});
