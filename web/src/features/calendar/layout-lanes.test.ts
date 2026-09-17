import { describe, expect, it } from 'vitest';

import { layoutLanes } from './layout-lanes';

const at = (id: string, from: string, to: string) => {
  const minutes = (hhmm: string) => Number(hhmm.slice(0, 2)) * 60 + Number(hhmm.slice(3));
  return { id, startMin: minutes(from), endMin: minutes(to) };
};

describe('layoutLanes', () => {
  it('puts overlapping events side by side and reuses a freed lane', () => {
    const lanes = layoutLanes([at('a', '09:00', '10:00'), at('b', '09:30', '11:00'), at('c', '10:00', '10:30')]);
    expect(lanes.a).toEqual({ lane: 0, lanes: 2 });
    expect(lanes.b).toEqual({ lane: 1, lanes: 2 });
    expect(lanes.c).toEqual({ lane: 0, lanes: 2 });
  });

  it('gives disjoint events the whole column each', () => {
    const lanes = layoutLanes([at('a', '09:00', '10:00'), at('b', '11:00', '12:00')]);
    expect(lanes.a).toEqual({ lane: 0, lanes: 1 });
    expect(lanes.b).toEqual({ lane: 0, lanes: 1 });
  });

  it('handles three events that all overlap', () => {
    const lanes = layoutLanes([at('a', '09:00', '12:00'), at('b', '09:30', '12:00'), at('c', '10:00', '12:00')]);
    expect(new Set([lanes.a.lane, lanes.b.lane, lanes.c.lane])).toEqual(new Set([0, 1, 2]));
    expect(lanes.a.lanes).toBe(3);
  });

  it('is empty for no events', () => {
    expect(layoutLanes([])).toEqual({});
  });
});
