import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { demoEvents, demoPeriods, demoWeekendDays } from './_fixture';
import { MonthView } from './month-view';

const view = (overrides: Partial<React.ComponentProps<typeof MonthView>> = {}) => (
  <MonthView
    anchor="2026-03-14"
    events={demoEvents}
    weekendDays={demoWeekendDays}
    periods={demoPeriods}
    canEdit
    onSelectDate={() => {}}
    onSelectEvent={() => {}}
    {...overrides}
  />
);

describe('MonthView', () => {
  it('draws six Monday-first weeks and marks non-working days in text', () => {
    const r = renderWithProviders(view());
    expect(r.container.querySelectorAll('[data-day]')).toHaveLength(42);
    const saturday = r.container.querySelector('[data-day="2026-03-14"]')!;
    expect(saturday).toHaveAttribute('data-non-working', 'true');
    expect(saturday).toHaveTextContent('Hafta sonu');
    const vacation = r.container.querySelector('[data-day="2026-03-13"]')!;
    expect(vacation).toHaveTextContent('Yıllık bakım tatili');
    const workday = r.container.querySelector('[data-day="2026-03-12"]')!;
    expect(workday).not.toHaveAttribute('data-non-working');
  });

  it('shows three chips and hides the rest behind a popover', async () => {
    const many = Array.from({ length: 5 }, (_, i) => ({ ...demoEvents[1], id: `x-${i}`, title: `Etkinlik ${i}` }));
    const r = renderWithProviders(view({ events: many }));
    const day = r.container.querySelector('[data-day="2026-03-10"]')!;
    expect(day.querySelectorAll('button[style*="border-inline-start-color"]')).toHaveLength(3);
    await r.user.click(r.getByRole('button', { name: '+2 daha' }));
    expect(await r.findByRole('button', { name: 'Etkinlik 4' })).toBeInTheDocument();
  });

  it('opens an event and a day only when the role may edit', async () => {
    const onSelectEvent = vi.fn();
    const onSelectDate = vi.fn();
    const editable = renderWithProviders(view({ onSelectEvent, onSelectDate }));
    await editable.user.click(editable.getAllByRole('button', { name: 'Bakım günü' })[0]);
    expect(onSelectEvent).toHaveBeenCalled();
    expect(editable.getAllByRole('button', { name: 'Etkinlik ekle' }).length).toBeGreaterThan(0);
    editable.unmount();

    const readOnly = renderWithProviders(view({ canEdit: false }));
    expect(readOnly.queryByRole('button', { name: 'Etkinlik ekle' })).toBeNull();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(view());
    await expectNoAxeViolations(r.container);
  });
});
