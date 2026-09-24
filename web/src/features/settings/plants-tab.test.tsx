import { describe, expect, it, vi } from 'vitest';

import type { Plant } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { PlantsTabView } from './plants-tab';

const plants = [
  {
    id: 'p-1',
    name: 'Çatı GES',
    plant_kind: 'rooftop',
    installation_number: '400123',
    total_capacity_kw: '250.00',
    created_at: '',
  },
] as Plant[];

describe('PlantsTabView', () => {
  it('names the plant type and formats the capacity', () => {
    const r = renderWithProviders(<PlantsTabView plants={plants} canEdit onAdd={() => {}} onEdit={() => {}} onDelete={() => {}} />);
    const row = r.getByRole('row', { name: /Çatı GES/ });
    expect(row).toHaveTextContent('Çatı GES');
    expect(row).toHaveTextContent('250');
  });

  it('hides every control from a read-only role', () => {
    const r = renderWithProviders(
      <PlantsTabView plants={plants} canEdit={false} onAdd={() => {}} onEdit={() => {}} onDelete={() => {}} />,
    );
    expect(r.queryByRole('button', { name: 'Santral ekle' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Santralı düzenle' })).toBeNull();
  });

  it('reports a delete request', async () => {
    const onDelete = vi.fn();
    const r = renderWithProviders(<PlantsTabView plants={plants} canEdit onAdd={() => {}} onEdit={() => {}} onDelete={onDelete} />);
    await r.user.click(r.getByRole('button', { name: 'Santralı sil' }));
    expect(onDelete).toHaveBeenCalledWith(plants[0]);
  });
});

describe('iSolar link actions (R281)', () => {
  it('offers link for an unlinked plant and unlink for a linked one', async () => {
    const onLink = vi.fn();
    const onUnlink = vi.fn();
    const linked = { ...plants[0], id: 'p-2', name: 'Arazi GES', isolar_ps_id: 'PS-9', isolar_ps_name: 'Arazi' } as Plant;
    const r = renderWithProviders(
      <PlantsTabView plants={[plants[0], linked]} canEdit onAdd={() => {}} onEdit={() => {}} onDelete={() => {}} onLink={onLink} onUnlink={onUnlink} />,
    );
    await r.user.click(r.getByRole('button', { name: 'iSolarCloud’a bağla' }));
    expect(onLink).toHaveBeenCalledWith(plants[0]);
    await r.user.click(r.getByRole('button', { name: 'iSolar bağlantısını kaldır' }));
    expect(onUnlink).toHaveBeenCalledWith(linked);
  });
});
