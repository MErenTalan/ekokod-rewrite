import { describe, expect, it, vi } from 'vitest';

import type { Building } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { BuildingsTabView } from './buildings-tab';

const buildings = [
  {
    id: 'b-1',
    name: 'A1 Fabrika',
    address: 'Ostim OSB',
    sector: 'Üretim',
    bill_cutoff_day: 5,
    analyzer_count: 2,
    activity_status: 'active',
    created_at: '',
    updated_at: '',
  },
] as Building[];

describe('BuildingsTabView', () => {
  it('offers no way to change anything to a read-only role', () => {
    const r = renderWithProviders(
      <BuildingsTabView buildings={buildings} canEdit={false} onAdd={() => {}} onEdit={() => {}} onDelete={() => {}} />,
    );
    expect(r.getByRole('row', { name: /A1 Fabrika/ })).toBeInTheDocument();
    expect(r.queryByRole('button', { name: 'Bina ekle' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Binayı düzenle' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Binayı sil' })).toBeNull();
  });

  it('edits and deletes for a role that may write', async () => {
    const onEdit = vi.fn();
    const onDelete = vi.fn();
    const r = renderWithProviders(
      <BuildingsTabView buildings={buildings} canEdit onAdd={() => {}} onEdit={onEdit} onDelete={onDelete} />,
    );
    await r.user.click(r.getByRole('button', { name: 'Binayı düzenle' }));
    expect(onEdit).toHaveBeenCalledWith(buildings[0]);
    await r.user.click(r.getByRole('button', { name: 'Binayı sil' }));
    expect(onDelete).toHaveBeenCalledWith(buildings[0]);
  });
});
