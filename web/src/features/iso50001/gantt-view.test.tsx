import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { clauses, emptyProject, project } from './_fixture';
import { barGeometry, GanttView } from './gantt-view';

describe('barGeometry', () => {
  it('places a clause on the project axis in percent', () => {
    expect(barGeometry('2026-01-01', '2026-12-31', '2026-01-01', '2026-12-31')).toEqual({ left: 0, width: 100 });
    const mid = barGeometry('2026-07-02', '2026-07-02', '2026-01-01', '2026-12-31');
    expect(mid.left).toBeCloseTo(49.86, 1);
    expect(mid.width).toBeGreaterThan(0.2);
  });
});

describe('GanttView', () => {
  it('shows one row per main clause with its dates and status as text (R334)', () => {
    const r = renderWithProviders(<GanttView project={project} clauses={clauses} today="2026-06-15" />);
    const rows = r.getAllByRole('listitem');
    expect(rows).toHaveLength(5);
    expect(rows[0]).toHaveTextContent('5. Liderlik');
    expect(rows[0]).toHaveTextContent('Tamamlandı');
    expect(rows[1]).toHaveTextContent('Devam Ediyor');
    expect(rows[4]).toHaveTextContent('Süresi Doldu');
    expect(r.getByText(/Proje Başlangıcı/)).toBeVisible();
  });

  it('says there is not enough date data', () => {
    const r = renderWithProviders(<GanttView project={emptyProject} clauses={clauses} today="2026-06-15" />);
    expect(r.getByText('Gantt tablosu için yeterli tarih verisi bulunmuyor.')).toBeVisible();
    expect(r.queryByRole('listitem')).toBeNull();
  });
});
