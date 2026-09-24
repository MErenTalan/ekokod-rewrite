import { describe, expect, it } from 'vitest';

import { chartAnimation, seriesStyle, toPlot, type ChartSeries, type SeriesKind } from './_theme';

const s = (kind: SeriesKind, key: string = kind): ChartSeries => ({ key, label: key, kind, unit: 'kWh' });

describe('chart theme', () => {
  it('energy kinds map to their tokens and never change', () => {
    const tokens: [SeriesKind, string][] = [
      ['consumption', 'consumption'],
      ['generation', 'generation'],
      ['reactiveInductive', 'reactive-inductive'],
      ['reactiveCapacitive', 'reactive-capacitive'],
      ['cost', 'cost'],
      ['revenue', 'revenue'],
      ['forecast', 'forecast'],
    ];
    for (const [kind, token] of tokens) {
      for (const index of [0, 3, 7]) expect(seriesStyle(s(kind), index).color).toBe(`var(--color-${token})`);
    }
  });

  it('forecast is always dashed and neutral', () => {
    for (const index of [0, 1, 5]) expect(seriesStyle(s('forecast'), index)).toEqual({ color: 'var(--color-forecast)', dash: '6 4' });
  });

  it('series at different indices get different dashes', () => {
    expect(seriesStyle(s('consumption', 'a'), 0).dash).not.toBe(seriesStyle(s('consumption', 'b'), 1).dash);
  });

  it('reduced motion disables animation', () => {
    expect(chartAnimation(true).isAnimationActive).toBe(false);
    expect(chartAnimation(false).animationDuration).toBe(400);
  });

  it('plot conversion keeps null as a gap', () => {
    expect(toPlot(null)).toBeNull();
    expect(toPlot('1234.5')).toBe(1234.5);
  });
});
