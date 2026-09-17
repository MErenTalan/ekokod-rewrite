import { describe, expect, it, vi } from 'vitest';

import type { CompanyDetail } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { CompanyTabView } from './company-tab';

const company: CompanyDetail = {
  id: 'c-1',
  name: 'Anadolu Tekstil',
  address: 'Ostim OSB, Ankara',
  sector: 'Üretim',
  total_area_m2: '5000.00',
  personnel_count: 120,
  contact_name: 'Ayşe Kaya',
  contact_phone: '0312 000 00 00',
  analyzer_counts: [
    { provider: 'osos', subtype: 'Baskent', count: 2 },
    { provider: 'gridbox', subtype: 'default', count: 1 },
  ],
  created_at: '',
  updated_at: '',
};

describe('CompanyTabView', () => {
  it('groups the analyzers by integration', () => {
    const r = renderWithProviders(<CompanyTabView company={company} canEdit onSave={() => {}} />);
    expect(r.getByRole('row', { name: /osos \/ Baskent/ })).toHaveTextContent('2');
    expect(r.getByRole('row', { name: /gridbox \/ default/ })).toHaveTextContent('1');
  });

  it('saves the details a company admin may edit', async () => {
    const onSave = vi.fn();
    const r = renderWithProviders(<CompanyTabView company={company} canEdit onSave={onSave} />);
    await r.user.clear(r.getByLabelText(/Sektör/));
    await r.user.type(r.getByLabelText(/Sektör/), 'Tekstil');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onSave.mock.calls[0][0]).toMatchObject({ sector: 'Tekstil', name: 'Anadolu Tekstil', personnel_count: 120 });
  });

  it('is read-only without the edit permission', () => {
    const r = renderWithProviders(<CompanyTabView company={company} canEdit={false} onSave={() => {}} />);
    expect(r.getByLabelText(/Şirket adı/)).toBeDisabled();
    expect(r.queryByRole('button', { name: 'Kaydet' })).toBeNull();
  });
});
