import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { activity, factor } from './_fixture';
import { ActivityDialog, type ActivityDialogProps } from './activity-dialog';

const sub = { key: 'sub_space_heating', scope: 'scope_1' as const, iso_category: 'category_1' };
const factors = [
  factor(),
  factor({ id: 'f-lpg', key: 'lpg', label: 'Ortam Isıtması > LPG', category_path: ['lpg'], base_factor: '1.55', base_unit: 'litre',
    conversions: [], source: 'Defra', source_year: 2025 }),
];

const render = (over: Partial<ActivityDialogProps> = {}) =>
  renderWithProviders(
    <ActivityDialog open sub={sub} factors={factors} today="2026-09-10" errors={{}} saving={false} onSubmit={vi.fn()} onClose={vi.fn()} {...over} />,
  );

describe('ActivityDialog', () => {
  it('shows the derived scope and ISO category and that the server computes the emission (R324)', () => {
    const r = render();
    expect(r.getByRole('dialog', { name: 'Ortam Isıtması için veri ekle' })).toBeVisible();
    expect(r.getByText('GHG kapsamı ve ISO 14064 kategorisi: Kapsam 1 · Kategori 1')).toBeVisible();
    expect(r.getByText('Emisyon, kaydettiğinizde sunucuda katalogdaki faktörle hesaplanır.')).toBeVisible();
  });

  it('sends the entry with the chosen factor, unit and decimal quantity', async () => {
    const onSubmit = vi.fn();
    const r = render({
      onSubmit,
      initial: activity({ factor_key: 'natural_gas', unit: 'kWh', quantity: '1234.5', period_start: '2026-08-01', period_end: '2026-08-31', description: 'Kazan' }),
    });
    expect(r.getByText('Faktör: 2,06672 kg CO₂e / m3')).toBeVisible();
    expect(r.getByText('Kaynak: Defra, 2025')).toBeVisible();
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onSubmit).toHaveBeenCalledWith({
      sub_category: 'sub_space_heating', factor_key: 'natural_gas', unit: 'kWh', quantity: '1234.5',
      period_start: '2026-08-01', period_end: '2026-08-31', description: 'Kazan', details: { path: 'natural_gas' },
    });
  });

  it('cannot save without a factor, period and positive quantity', () => {
    const r = render();
    expect(r.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
  });

  it('shows the server refusal on its field', () => {
    const r = render({ errors: { quantity: 'Miktar geçersiz.' } });
    expect(r.getByText('Miktar geçersiz.')).toBeVisible();
  });

  it('says when the activity has no factor', () => {
    const r = render({ factors: [] });
    expect(r.getByText('Bu faaliyet için emisyon faktörü yok.')).toBeVisible();
  });
});
