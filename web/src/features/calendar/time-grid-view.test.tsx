import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoEvents, demoPeriods, demoWeekendDays } from './_fixture';
import { TimeGridView } from './time-grid-view';

const view = (overrides: Partial<React.ComponentProps<typeof TimeGridView>> = {}) => (
  <TimeGridView
    anchor="2026-03-10"
    view="week"
    events={demoEvents}
    weekendDays={demoWeekendDays}
    periods={demoPeriods}
    canEdit
    onSelectDate={() => {}}
    onSelectEvent={() => {}}
    {...overrides}
  />
);

describe('TimeGridView', () => {
  it('lays overlapping events side by side', () => {
    const r = renderWithProviders(view());
    const audit = r.getByRole('button', { name: /Enerji denetimi/ });
    const meeting = r.getByRole('button', { name: /Tedarikçi toplantısı/ });
    expect(audit.style.width).toBe('50%');
    expect(meeting.style.width).toBe('50%');
    expect(audit.style.insetInlineStart).not.toBe(meeting.style.insetInlineStart);
  });

  it('puts an all-day event in its own row and a timed one on the grid', () => {
    const r = renderWithProviders(view());
    expect(r.getByRole('button', { name: 'Bakım günü' })).toBeInTheDocument();
    expect(r.getByRole('button', { name: /09:00–11:00/ })).toBeInTheDocument();
  });

  it('shows one column for a day view and seven for a week', () => {
    const week = renderWithProviders(view());
    expect(week.container.querySelectorAll('[data-day]')).toHaveLength(7);
    week.unmount();
    const day = renderWithProviders(view({ view: 'day', anchor: '2026-03-10' }));
    expect(day.container.querySelectorAll('[data-day]')).toHaveLength(1);
  });
});
