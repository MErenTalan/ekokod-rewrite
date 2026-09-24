import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoEvents, demoPeriods, demoWeekendDays } from './_fixture';
import { AgendaView } from './agenda-view';

const view = (overrides: Partial<React.ComponentProps<typeof AgendaView>> = {}) => (
  <AgendaView
    anchor="2026-03-10"
    events={demoEvents}
    weekendDays={demoWeekendDays}
    periods={demoPeriods}
    canEdit
    onSelectDate={() => {}}
    onSelectEvent={() => {}}
    {...overrides}
  />
);

describe('AgendaView', () => {
  it('lists only the days that have something on them, in order', () => {
    const r = renderWithProviders(view());
    const headings = r.getAllByRole('heading', { level: 3 }).map((h) => h.textContent);
    expect(headings).toEqual(['10 Mar 2026', '12 Mar 2026']);
    expect(r.getByText('Tüm gün')).toBeInTheDocument();
    expect(r.getByText('09:00–11:00')).toBeInTheDocument();
  });

  it('says so when the range is empty', () => {
    const r = renderWithProviders(view({ events: [] }));
    expect(r.getByText('Bu aralıkta etkinlik yok')).toBeInTheDocument();
  });
});
