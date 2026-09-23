import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { factor } from './_fixture';
import { FactorDialog } from './factor-dialog';

describe('FactorDialog', () => {
  it('sends the new value with its source and year (R303)', async () => {
    const onSubmit = vi.fn();
    const r = renderWithProviders(<FactorDialog factor={factor()} saving={false} errors={{}} onSubmit={onSubmit} onClose={vi.fn()} />);
    const value = r.getByRole('textbox', { name: /Faktör değeri \(kg CO₂e \/ m3\)/ });
    await r.user.clear(value);
    await r.user.type(value, '2,1');
    const year = r.getByRole('textbox', { name: 'Kaynak yılı' });
    await r.user.clear(year);
    await r.user.type(year, '2026');
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    expect(onSubmit).toHaveBeenCalledWith({ base_factor: '2.1', source: 'Defra', source_year: 2026, source_url: 'https://www.gov.uk/' });
  });

  it('shows the server refusal on the value', () => {
    const r = renderWithProviders(<FactorDialog factor={factor()} saving={false} errors={{ base_factor: 'Değer aralık dışında.' }} onSubmit={vi.fn()} onClose={vi.fn()} />);
    expect(r.getByText('Değer aralık dışında.')).toBeVisible();
  });
});
