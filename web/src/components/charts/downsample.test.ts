import { describe, expect, it } from 'vitest';

import { downsampleLttb } from './downsample';

describe('downsampleLttb', () => {
  it('reduces to the threshold, keeps endpoints', () => {
    const data = Array.from({ length: 5000 }, (_, i) => ({ x: i, v: String(Math.sin(i / 50) * 100) }));
    const out = downsampleLttb(data, 'v', 1000);
    expect(out).toHaveLength(1000);
    expect(out[0].x).toBe(0);
    expect(out.at(-1)!.x).toBe(4999);
  });

  it('keeps a spike that averaging would flatten', () => {
    const data = Array.from({ length: 3000 }, (_, i) => ({ x: i, v: i === 1500 ? '1000' : '1' }));
    expect(downsampleLttb(data, 'v', 100).some((d) => d.v === '1000')).toBe(true);
  });

  it('returns short input unchanged', () => {
    const data = [{ x: 1, v: '2' }];
    expect(downsampleLttb(data, 'v', 1000)).toBe(data);
  });
});
