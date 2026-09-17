import { describe, expect, it, vi } from 'vitest';

import type { BuildingDetail, User } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { BuildingFormView, MAX_CONTACTS, buildingDraft, emptyBuilding, toBuildingRequest } from './building-form';

const users = [{ id: 'u-1', name: 'Burak Şahin', email: 'burak@ornek.com.tr' }] as User[];
const detail = {
  id: 'b-1',
  name: 'A1 Fabrika',
  address: 'Ostim OSB',
  sector: 'Üretim',
  latitude: '39.925533',
  longitude: '32.866287',
  floors: 3,
  personnel_count: 40,
  total_area_m2: '5000.50',
  responsible_user_id: 'u-1',
  bill_cutoff_day: 5,
  contacts: [{ name: 'Ayşe', phone: '0532' }],
  tariff_history: [{ id: 't-1', effective_from: '2026-01-01', name: 'Sanayi OG' }],
  created_at: '',
  updated_at: '',
} as BuildingDetail;

describe('toBuildingRequest', () => {
  it('keeps decimals as strings and integers as numbers', () => {
    expect(toBuildingRequest(buildingDraft(detail))).toMatchObject({
      name: 'A1 Fabrika',
      total_area_m2: '5000.50',
      latitude: '39.925533',
      floors: 3,
      personnel_count: 40,
      bill_cutoff_day: 5,
      responsible_user_id: 'u-1',
      contacts: [{ name: 'Ayşe', phone: '0532' }],
    });
  });

  it('says explicitly when the responsible user was cleared (R175)', () => {
    const request = toBuildingRequest({ ...buildingDraft(detail), responsibleUserId: null });
    expect(request.clear_responsible_user).toBe(true);
    expect(request).not.toHaveProperty('responsible_user_id');
  });

  it('omits an empty optional instead of sending an empty string', () => {
    const request = toBuildingRequest({ ...emptyBuilding(), name: 'Yeni Bina' });
    expect(request.address).toBeNull();
    expect(request).not.toHaveProperty('latitude');
    expect(request.floors).toBeNull();
  });
});

describe('BuildingFormView', () => {
  it('refuses a cut-off day outside 1–31 before submitting', async () => {
    const onChange = vi.fn();
    const r = renderWithProviders(
      <BuildingFormView value={{ ...buildingDraft(detail), billCutoffDay: '32' }} onChange={onChange} users={users} tariffHistory={[]} />,
    );
    expect(r.getByText('1 ile 31 arasında olmalı')).toBeInTheDocument();
  });

  it('adds and removes contact rows up to the limit', async () => {
    const onChange = vi.fn();
    const full = { ...buildingDraft(detail), contacts: Array.from({ length: MAX_CONTACTS }, () => ({ name: '', phone: '' })) };
    const r = renderWithProviders(<BuildingFormView value={full} onChange={onChange} users={users} tariffHistory={[]} />);
    expect(r.getByRole('button', { name: 'Kişi ekle' })).toBeDisabled();
    await r.user.click(r.getByRole('button', { name: '1. kişiyi kaldır' }));
    expect(onChange.mock.calls[0][0].contacts).toHaveLength(MAX_CONTACTS - 1);
  });

  it('shows the tariff history read-only with a link to the tariffs screen', () => {
    const r = renderWithProviders(
      <BuildingFormView value={buildingDraft(detail)} onChange={() => {}} users={users} tariffHistory={detail.tariff_history} />,
    );
    expect(r.getByRole('row', { name: /Sanayi OG/ })).toBeInTheDocument();
    expect(r.getByRole('link', { name: 'Tarifeleri yönet' })).toHaveAttribute('href', '/ekorm/tariffs?building_id=b-1');
    expect(r.queryByRole('button', { name: /Tarife ekle/ })).toBeNull();
  });
});
