import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoPeriods, demoWeekendDays } from './_fixture';
import { VacationDialogView } from './vacation-dialog';

const view = (overrides: Partial<React.ComponentProps<typeof VacationDialogView>> = {}) => (
  <VacationDialogView
    open
    onOpenChange={() => {}}
    weekendDays={demoWeekendDays}
    periods={demoPeriods}
    onSave={() => {}}
    {...overrides}
  />
);

describe('VacationDialogView', () => {
  it('maps the weekday checkboxes onto the API numbers', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(view({ onSave }));
    expect(r.getByRole('checkbox', { name: 'Pazar' })).toBeChecked();
    expect(r.getByRole('checkbox', { name: 'Cuma' })).not.toBeChecked();
    await r.user.click(r.getByRole('checkbox', { name: 'Cuma' }));
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onSave).toHaveBeenCalledWith({ weekend_days: [0, 6, 5], periods: demoPeriods });
  });

  it('warns when every day would be non-working', async () => {
    const r = renderWithProviders(view({ weekendDays: [0, 1, 2, 3, 4, 5, 6] }));
    expect(r.getByText(/Tüm günler tatil olarak işaretlendi/)).toBeInTheDocument();
  });

  it('refuses a period that ends before it starts', () => {
    const r = renderWithProviders(
      view({ periods: [{ id: 'v-1', start_date: '2026-03-10', end_date: '2026-03-09', description: 'Hatalı' }] }),
    );
    expect(r.getByText('Bitiş tarihi başlangıçtan önce olamaz')).toBeInTheDocument();
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
  });

  it('is read-only without the calendar permission', () => {
    const r = renderWithProviders(view({ readOnly: true }));
    expect(r.getByRole('checkbox', { name: 'Pazar' })).toBeDisabled();
    expect(r.queryByRole('button', { name: 'Kaydet' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Dönem ekle' })).toBeNull();
  });
});
