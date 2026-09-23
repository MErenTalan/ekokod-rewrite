import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { demoAssignments, demoBuildingStates } from './_fixture';
import { BulkTabView } from './bulk-tab';

const view = (over: Partial<Parameters<typeof BulkTabView>[0]> = {}) => {
  const onAssign = vi.fn();
  const r = renderWithProviders(
    <BulkTabView buildings={demoBuildingStates} assignments={demoAssignments} canEdit onAssign={onAssign} {...over} />,
  );
  return { ...r, onAssign };
};

describe('BulkTabView', () => {
  it('shows buildings that have no tariff at all', () => {
    // "Which buildings have no tariff" is the question this table answers.
    const r = view();
    expect(r.getByRole('cell', { name: 'A2 Depo' })).toBeVisible();
    expect(r.getByRole('cell', { name: /Tarife yok/ })).toBeVisible();
  });

  it('lists the assignment history newest first with its building count', () => {
    const r = view();
    const rows = r.getAllByRole('row', { name: /bina|Şablondan|Elle/ });
    expect(rows[0]).toHaveTextContent('Şablondan');
    expect(rows[0]).toHaveTextContent('2');
  });

  it('assigns to the checked buildings only', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('checkbox', { name: /A2 Depo/ }));
    await user.click(r.getByRole('button', { name: /^Uygula$/ }));
    expect(r.onAssign).toHaveBeenCalledWith(['b-2']);
  });

  it('selects and clears every building at once', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Tümünü seç/ }));
    expect(r.getByText(/2 bina seçili/)).toBeVisible();
    await user.click(r.getByRole('button', { name: /Seçimi temizle/ }));
    expect(r.queryByText(/bina seçili/)).toBeNull();
  });

  it('refuses to assign with nothing selected', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /^Uygula$/ }));
    expect(r.onAssign).not.toHaveBeenCalled();
    expect(r.getByRole('alert')).toHaveTextContent(/En az bir bina seçin/);
  });

  it('shows an empty history state', () => {
    expect(view({ assignments: [] }).getByText(/Henüz toplu atama yapılmadı/)).toBeVisible();
  });
});
