import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { catalogue } from './_fixture';
import { SelectionView } from './selection-view';

describe('SelectionView', () => {
  it('lists every sub-category under its main category with the derived scope and ISO category (R323)', () => {
    const r = renderWithProviders(<SelectionView catalogue={catalogue} selected={[]} editable onSave={vi.fn()} />);
    expect(r.getByRole('group', { name: 'Atık' })).toBeVisible();
    expect(r.getByRole('checkbox', { name: 'Atık Bertarafı' })).not.toBeChecked();
    expect(r.getByText('Kapsam 3 · Kategori 6')).toBeVisible();
  });

  it('saves the checked keys sorted', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(<SelectionView catalogue={catalogue} selected={['sub_space_heating']} editable onSave={onSave} />);
    await r.user.click(r.getByRole('checkbox', { name: 'Atık Bertarafı' }));
    await r.user.click(r.getByRole('checkbox', { name: 'Ortam Isıtması' }));
    await r.user.click(r.getByRole('checkbox', { name: 'Şebekeden Elektrik Tüketimi' }));
    await r.user.click(r.getByRole('button', { name: 'Seçimi kaydet' }));
    expect(onSave).toHaveBeenCalledWith(['sub_grid_electricity', 'sub_waste_disposal']);
  });

  it('is read-only without the edit right', () => {
    const r = renderWithProviders(<SelectionView catalogue={catalogue} selected={['sub_waste_disposal']} editable={false} onSave={vi.fn()} />);
    expect(r.getByRole('checkbox', { name: 'Atık Bertarafı' })).toBeDisabled();
    expect(r.getByRole('checkbox', { name: 'Atık Bertarafı' })).toBeChecked();
    expect(r.queryByRole('button', { name: 'Seçimi kaydet' })).toBeNull();
    expect(r.getByText('Faaliyet seçimini yalnızca şirket yöneticileri değiştirebilir.')).toBeVisible();
  });
});
