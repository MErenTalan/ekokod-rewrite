import { describe, expect, it, vi } from 'vitest';

import type { PlantDetail } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { MONTH_KEYS, PlantFormView, emptyPlant, monthlyTargetsComplete, plantDraft, toPlantRequest } from './plant-form';

const detail = {
  id: 'p-1',
  name: 'Çatı GES',
  plant_kind: 'rooftop',
  installation_number: '400123',
  total_capacity_kw: '250.00',
  monthly_targets: Array.from({ length: 12 }, (_, i) => String(1000 + i)),
  alarm_recipients: ['ges@ornek.com.tr'],
  devices: [
    { id: 'd-1', device_sn: 'INV-1', device_name: 'İnvertör 1', brand: 'Sungrow', model: 'SG50', rated_power_kw: '50', status: 'ok' },
  ],
  created_at: '',
  updated_at: '',
} as PlantDetail;

describe('plant form', () => {
  it('sends the twelve monthly targets as decimal strings', () => {
    const request = toPlantRequest(plantDraft(detail));
    expect(request.monthly_targets).toHaveLength(12);
    expect(request.monthly_targets?.[0]).toBe('1000');
    expect(request.plant_kind).toBe('rooftop');
  });

  it('refuses to save while a month is empty, never substituting a zero', () => {
    const draft = { ...plantDraft(detail), monthlyTargets: plantDraft(detail).monthlyTargets.map((v, i) => (i === 3 ? '' : v)) };
    expect(monthlyTargetsComplete(draft)).toBe(false);
    const r = renderWithProviders(<PlantFormView value={draft} onChange={() => {}} devices={[]} />);
    expect(r.getByText('12 ayın hedefi girilmeli')).toBeInTheDocument();
  });

  it('labels the twelve months and the eight orientations', () => {
    const r = renderWithProviders(<PlantFormView value={emptyPlant()} onChange={() => {}} devices={[]} />);
    expect(MONTH_KEYS).toHaveLength(12);
    expect(r.getByLabelText('Ocak')).toBeInTheDocument();
    expect(r.getByLabelText('Aralık')).toBeInTheDocument();
    expect(r.getByRole('combobox', { name: 'Panellerin yönü' })).toBeInTheDocument();
  });

  it('rejects an alarm recipient that is not an e-mail', async () => {
    const onChange = vi.fn();
    const r = renderWithProviders(<PlantFormView value={plantDraft(detail)} onChange={onChange} devices={[]} />);
    await r.user.type(r.getByLabelText(/Alıcı ekle/), 'abc');
    expect(r.getByText('Geçerli bir e-posta girin')).toBeInTheDocument();
    expect(r.getByRole('button', { name: 'Alıcı ekle' })).toBeDisabled();
  });

  it('lists the devices read-only and says where they come from', () => {
    const r = renderWithProviders(<PlantFormView value={plantDraft(detail)} onChange={() => {}} devices={detail.devices} />);
    expect(r.getByText('Cihazlar iSolarCloud bağlantısıyla içe aktarılır')).toBeInTheDocument();
    expect(r.getByRole('row', { name: /İnvertör 1/ })).toBeInTheDocument();
  });
});

describe('netting analyzer (R289)', () => {
  const analyzers = [{ value: 'a-1', label: 'Sayaç 1 · 40001' }];

  it('offers the company analyzers and "none"', () => {
    const r = renderWithProviders(<PlantFormView value={emptyPlant()} onChange={() => {}} devices={[]} analyzers={analyzers} />);
    expect(r.getByRole('combobox', { name: /Mahsuplaşma analizörü/ })).toBeVisible();
  });

  it('sends the chosen analyzer, and clears it on an edit that removes it', () => {
    expect(toPlantRequest({ ...plantDraft(detail), nettingAnalyzerId: 'a-1' }).netting_analyzer_id).toBe('a-1');
    const cleared = toPlantRequest({ ...plantDraft({ ...detail, netting_analyzer_id: 'a-1' }), nettingAnalyzerId: '' });
    expect(cleared.netting_analyzer_id).toBeUndefined();
    expect((cleared as { clear_netting_analyzer?: boolean }).clear_netting_analyzer).toBe(true);
    expect((toPlantRequest(emptyPlant()) as { clear_netting_analyzer?: boolean }).clear_netting_analyzer).toBeUndefined();
  });
});
