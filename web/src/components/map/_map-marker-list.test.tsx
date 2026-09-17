import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { MapMarkerList } from './_map-marker-list';

describe('MapMarkerList', () => {
  it('passive markers say so in text and coordinates keep five digits', async () => {
    const { getByRole, container } = renderWithProviders(
      <MapMarkerList label="Konumlar" markers={[{ id: 'x', name: 'Eski Depo', lat: 39.9, lng: 32.85411, status: 'passive' }]} />,
    );
    expect(getByRole('row', { name: /Eski Depo/ })).toHaveTextContent('Pasif');
    expect(getByRole('row', { name: /Eski Depo/ })).toHaveTextContent('39,90000, 32,85411');
    await expectNoAxeViolations(container);
  });
});
