import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { CompanyView } from './company-view';

const company = {
  id: 'c-1', name: 'Anadolu Tekstil', address: 'Organize Sanayi 3. Cad. No 7, Bursa', sector: 'Tekstil', personnel_count: 240,
  total_area_m2: '18500', contact_name: 'Ayşe Kaya', contact_phone: null, created_at: '2026-01-01T00:00:00+03:00', updated_at: '2026-01-01T00:00:00+03:00',
};

describe('CompanyView', () => {
  it('shows the report header details and the settings link to an editor (Q-F10)', () => {
    const r = renderWithProviders(<CompanyView company={company} companyName="Anadolu Tekstil" canEdit />);
    expect(r.getByText('Organize Sanayi 3. Cad. No 7, Bursa')).toBeVisible();
    expect(r.getByText('240')).toBeVisible();
    expect(r.getByText('18.500')).toBeVisible();
    expect(r.getByRole('link', { name: 'Şirket ayarlarında düzenle' })).toHaveAttribute('href', '/ekorm/settings?tab=company');
  });

  it('says what is not set', () => {
    const r = renderWithProviders(<CompanyView company={{ ...company, address: null }} companyName="Anadolu Tekstil" canEdit={false} />);
    expect(r.getAllByText('Belirtilmemiş').length).toBeGreaterThan(0);
    expect(r.queryByRole('link')).toBeNull();
  });

  it('shows a role without company access only the name', () => {
    const r = renderWithProviders(<CompanyView companyName="Anadolu Tekstil" canEdit={false} />);
    expect(r.getByText(/Şirket bilgilerini şirket yöneticileri yönetir/)).toBeVisible();
    expect(r.getByText('Anadolu Tekstil')).toBeVisible();
  });
});
