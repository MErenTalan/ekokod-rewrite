import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { catalogue } from './_fixture';
import { EntryCards } from './entry-cards';

const subs = catalogue.items.flatMap((m) => m.subs.map((s) => ({ ...s, main: m.key }))).filter((s) => ['sub_space_heating', 'sub_waste_disposal'].includes(s.key));

describe('EntryCards', () => {
  it('shows one card per declared sub-category with its record count (R324)', async () => {
    const onAdd = vi.fn();
    const r = renderWithProviders(<EntryCards subs={subs} counts={{ sub_space_heating: 3 }} full={false} editable onAdd={onAdd} onGoSelection={vi.fn()} />);
    expect(r.getByRole('heading', { name: 'Ortam Isıtması' })).toBeVisible();
    expect(r.getByText('3 kayıt')).toBeVisible();
    expect(r.getByText('0 kayıt')).toBeVisible();
    await r.user.click(r.getAllByRole('button', { name: 'Veri ekle' })[1]);
    expect(onAdd).toHaveBeenCalledWith(expect.objectContaining({ key: 'sub_waste_disposal' }));
  });

  it('says "500+" when the count is capped', () => {
    const r = renderWithProviders(<EntryCards subs={subs} counts={{ sub_space_heating: 500 }} full editable onAdd={vi.fn()} onGoSelection={vi.fn()} />);
    expect(r.getAllByText('500+ kayıt').length).toBe(2);
  });

  it('sends an empty declaration to the selection tab', async () => {
    const onGoSelection = vi.fn();
    const r = renderWithProviders(<EntryCards subs={[]} counts={{}} full={false} editable onAdd={vi.fn()} onGoSelection={onGoSelection} />);
    expect(r.getByText('Henüz faaliyet seçilmedi')).toBeVisible();
    await r.user.click(r.getByRole('button', { name: 'Faaliyet Seçimine git' }));
    expect(onGoSelection).toHaveBeenCalled();
  });

  it('offers no entry to a read-only role', () => {
    const r = renderWithProviders(<EntryCards subs={subs} counts={{}} full={false} editable={false} onAdd={vi.fn()} onGoSelection={vi.fn()} />);
    expect(r.queryByRole('button', { name: 'Veri ekle' })).toBeNull();
    expect(r.getByText('Veri girişini yalnızca şirket yöneticileri yapabilir.')).toBeVisible();
  });
});
