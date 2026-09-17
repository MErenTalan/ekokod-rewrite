import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Map, type MapMarker } from './map';

const loaded = vi.hoisted(() => vi.fn());
vi.mock('maplibre-gl', () => {
  loaded();
  return {};
});

const markers: MapMarker[] = [
  { id: 'm1', name: 'Merkez Bina', lat: 41.015137, lng: 28.97953, status: 'active' },
  { id: 'm2', name: 'Eski Depo', lat: 39.92077, lng: 32.85411, status: 'passive' },
];

describe('Map', () => {
  it('without a tile URL the coordinate list renders', () => {
    const { getByRole } = renderWithProviders(<Map markers={markers} label="Bina konumları" tileUrl="" />);
    expect(getByRole('table', { name: 'Bina konumları' })).toBeInTheDocument();
    expect(getByRole('status')).toHaveTextContent('Harita altlığı yüklenemedi');
    expect(loaded).not.toHaveBeenCalled();
  });

  it('markers carry status in text', async () => {
    const onMarkerSelect = vi.fn();
    const { getByRole, user } = renderWithProviders(<Map markers={markers} label="Bina konumları" tileUrl="" onMarkerSelect={onMarkerSelect} />);
    const row = getByRole('row', { name: /Merkez Bina/ });
    expect(row).toHaveTextContent('Aktif');
    expect(row).toHaveTextContent('41,01514');
    await user.click(getByRole('button', { name: 'Merkez Bina seç' }));
    expect(onMarkerSelect).toHaveBeenCalledWith('m1');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Map markers={markers} label="Bina konumları" tileUrl="" />);
    await expectNoAxeViolations(container);
  });
});
