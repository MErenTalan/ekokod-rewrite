import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { IsolarLinkView } from './isolar-link-dialog';

const plants = [
  { ps_id: 'PS-1', name: 'Konya Solar', installed_kw: '250', linked_plant_id: undefined },
  { ps_id: 'PS-2', name: 'Başka Santral', installed_kw: '100', linked_plant_id: 'p-other' },
  { ps_id: 'PS-3', name: 'Bu Santral', installed_kw: '50', linked_plant_id: 'p-1' },
];
const credentials = [{ value: 'c-1', label: 'iSolarCloud (EU)' }];
const base = { plantId: 'p-1', credentials, credentialId: 'c-1', onCredentialChange: () => {}, plants, onLink: () => {} };

describe('IsolarLinkView', () => {
  it('lists the account plants with installed power (R281)', () => {
    const r = renderWithProviders(<IsolarLinkView {...base} />);
    expect(r.getByRole('radio', { name: /Konya Solar/ })).toBeEnabled();
    expect(r.getByText(/250 kW/)).toBeVisible();
  });

  it('disables a plant already linked to another plant', () => {
    const r = renderWithProviders(<IsolarLinkView {...base} />);
    expect(r.getByRole('radio', { name: /Başka Santral/ })).toBeDisabled();
    expect(r.getByText(/Başka bir santrale bağlı/)).toBeVisible();
    expect(r.getByText(/Bu santrale bağlı/)).toBeVisible();
  });

  it('links the chosen plant', async () => {
    const onLink = vi.fn();
    const r = renderWithProviders(<IsolarLinkView {...base} onLink={onLink} />);
    await r.user.click(r.getByRole('radio', { name: /Konya Solar/ }));
    await r.user.click(r.getByRole('button', { name: /Bağla/ }));
    expect(onLink).toHaveBeenCalledWith('PS-1');
  });

  it('sends the user to the company tab when there is no iSolar credential', () => {
    const r = renderWithProviders(<IsolarLinkView {...base} credentials={[]} credentialId={null} plants={[]} />);
    expect(r.getByText(/iSolarCloud bağlantısı yok/)).toBeVisible();
    expect(r.getByRole('link', { name: /Şirket/ })).toHaveAttribute('href', '/ekorm/settings?tab=company');
  });
});
