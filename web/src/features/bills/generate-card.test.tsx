import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { demoAnalyzers } from './_fixture';
import { GenerateCardView } from './generate-card';

const view = (over: Partial<Parameters<typeof GenerateCardView>[0]> = {}) => {
  const onGenerate = vi.fn();
  const r = renderWithProviders(
    <GenerateCardView
      analyzers={demoAnalyzers}
      period="2026-08"
      buildingId="b-1"
      onGenerate={onGenerate}
      {...over}
    />,
  );
  return { ...r, onGenerate };
};

describe('GenerateCardView', () => {
  it('offers the analyzers with their installation number and customer', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('combobox', { name: /Sayaçlar/ }));
    // §7.10: the picker shows installation number, customer and province/district.
    const option = r.getByRole('option', { name: /40001/ });
    expect(option).toHaveTextContent('A1 Fabrika A.Ş.');
    expect(option).toHaveTextContent('Ankara/Çankaya');
  });

  it('refuses an analyzer invoice with nothing selected and says so', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Sayaç faturası/ }));
    expect(r.onGenerate).not.toHaveBeenCalled();
    expect(r.getByRole('alert')).toHaveTextContent(/En az bir sayaç seçin/);
  });

  it('refuses any generation without a month', async () => {
    const user = userEvent.setup();
    const r = view({ period: null });
    await user.click(r.getByRole('button', { name: /Şirket faturası/ }));
    expect(r.onGenerate).not.toHaveBeenCalled();
    expect(r.getByRole('alert')).toHaveTextContent(/Bir ay seçin/);
  });

  it('starts a company invoice without asking for a meter', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Şirket faturası/ }));
    expect(r.onGenerate).toHaveBeenCalledWith({ scope: 'company', analyzerIds: [], buildingId: 'b-1', period: '2026-08' });
  });

  it('shows the failure the API named', () => {
    const r = view({ failure: 'noTariffForBuilding' });
    expect(r.getByRole('alert')).toHaveTextContent(/Bina için tarife bulunamadı/);
  });

  it('offers the finished invoice for download', async () => {
    const user = userEvent.setup();
    const onDownload = vi.fn();
    const r = view({ readyBillIds: ['bill-9'], onDownload });
    await user.click(r.getByRole('button', { name: /Faturayı indir/ }));
    expect(onDownload).toHaveBeenCalledWith('bill-9');
  });
});
