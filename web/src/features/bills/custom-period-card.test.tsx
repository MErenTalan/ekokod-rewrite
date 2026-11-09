import { describe, expect, it, vi } from 'vitest';
import { waitFor, within } from '@testing-library/react';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import type { BillPreview } from '@/lib/api/types';

import { demoAnalyzers, demoPreview, demoPreview as preview, demoSavedPeriod as saved } from './_fixture';
import { CustomPeriodCardView } from './custom-period-card';

const view = (over: Partial<Parameters<typeof CustomPeriodCardView>[0]> = {}) => {
  const props = { onPreview: vi.fn(), onSave: vi.fn(), onDelete: vi.fn() };
  const r = renderWithProviders(
    <CustomPeriodCardView analyzers={demoAnalyzers} buildingId="b-1" savedPeriods={[saved]} {...props} {...over} />,
  );
  return { ...r, ...props };
};

describe('CustomPeriodCardView', () => {
  it('marks both dates required and asks nothing of the API without them', async () => {
    const r = view({ savedPeriods: [] });
    await r.user.click(r.getByRole('button', { name: 'Önizle' }));
    expect(r.getAllByText('Zorunlu alan')).toHaveLength(2);
    expect(r.onPreview).not.toHaveBeenCalled();
  });

  it('previews the building bill over Istanbul instants with the reading hours', async () => {
    const r = view();
    await r.user.click(r.getByRole('button', { name: 'Forma yükle' }));
    await r.user.click(r.getByRole('button', { name: 'Önizle' }));
    expect(r.onPreview).toHaveBeenCalledWith({
      scope: 'building', subjectId: 'b-1', from: '2026-07-17T16:00:00+03:00', to: '2026-08-18T16:00:00+03:00',
    });
  });

  it('saves under the start month by default (R554)', async () => {
    const r = view();
    await r.user.click(r.getByRole('button', { name: 'Forma yükle' }));
    await r.user.click(r.getByRole('button', { name: 'Bu dönemi resmi dönem olarak kaydet' }));
    expect(r.onSave).toHaveBeenCalledWith({ periodKey: '2026-07', from: '2026-07-17T16:00:00+03:00', to: '2026-08-18T16:00:00+03:00' });
  });

  it('defaults the key to the month of a freshly picked start date', async () => {
    const r = view({ savedPeriods: [] });
    const pick = async (label: string) => {
      await r.user.click(r.getByRole('button', { name: new RegExp(label) }));
      const cell = (await r.findByRole('grid')).querySelector<HTMLElement>('[data-day]:not([data-outside]) button')!;
      const day = cell.closest('[data-day]')!.getAttribute('data-day')!;
      await r.user.click(cell);
      return day;
    };
    const from = await pick('Başlangıç tarihi');
    await pick('Bitiş tarihi');
    await r.user.click(r.getByRole('button', { name: 'Bu dönemi resmi dönem olarak kaydet' }));
    expect(r.onSave).toHaveBeenCalledWith(expect.objectContaining({ periodKey: from.slice(0, 7), from: `${from}T00:00:00+03:00` }));
  });

  it('cannot save without a building', () => {
    const r = view({ buildingId: null, savedPeriods: [] });
    expect(r.queryByRole('button', { name: /resmi dönem/ })).toBeNull();
    expect(r.getByText(/bir bina seçin/)).toBeInTheDocument();
  });

  it('shows the preview with hours, its lines and the days without data', () => {
    const r = view({ preview });
    expect(r.getByText(/2026-07 · 17.07.2026 16:00 – 18.08.2026 16:00 · 32 gün/)).toBeInTheDocument();
    expect(r.getByText('Enerji bedeli')).toBeInTheDocument();
    expect(r.getByText('Vergi/fon: BTV')).toBeInTheDocument();
    expect(r.getByText(/Birim fiyat: Elektrik enerji bedeli 3,593417 \+ Dağıtım bedeli 2,479400 = Toplam birim fiyat 6,072817 TL\/kWh/)).toBeInTheDocument();
    expect(r.getByText(/Verisi olmayan günler/)).toHaveTextContent('07.08.2026 – 08.08.2026');
  });

  it('keeps an unpriced PTF energy line and dashes the unknown unit prices (R560)', () => {
    const unpriced = {
      ...demoPreview,
      lines: [{ code: 'energy', label: 'energy', quantity: '100', unit: 'kWh', amount: '0' }, demoPreview.lines[1]],
      unit_prices: { distribution: '2.4794' },
    } as BillPreview;
    const r = view({ preview: unpriced });
    expect(r.getByText('PTF yok')).toBeInTheDocument();
    expect(r.getByText(/Elektrik enerji bedeli — \+ Dağıtım bedeli 2,479400 = Toplam birim fiyat —/)).toBeInTheDocument();
  });

  it('names a reading gap as such, and missing PTF only when it is missing', () => {
    const gap = { ...demoPreview, bill: { ...demoPreview.bill, status: 'flagged', flag_reason: 'hourly_consumption_missing' } } as BillPreview;
    const r = view({ preview: gap });
    expect(r.getByText(/Saatlik tüketim verisi eksik/)).toBeInTheDocument();
    expect(r.queryByText(/PTF\/YEKDEM verisi yok/)).toBeNull();
    r.unmount();

    const both = { ...gap, bill: { ...gap.bill, flag_reason: 'installed_power_missing,ptf_data_missing' } } as BillPreview;
    const r2 = view({ preview: both });
    expect(r2.getByText(/PTF\/YEKDEM verisi yok/)).toBeInTheDocument();
  });

  it('marks the field the API names', () => {
    const r = view({ errors: { to: 'Geçersiz değer' } });
    expect(r.getByText('Geçersiz değer')).toBeInTheDocument();
  });

  it('closes the delete confirmation only after the delete succeeded', async () => {
    let done: () => void = () => {};
    const r = view({ onDelete: vi.fn((_key: string, d: () => void) => { done = d; }) });
    await r.user.click(r.getByRole('button', { name: 'Sil' }));
    const dialog = await r.findByRole('dialog');
    await r.user.click(within(dialog).getByRole('button', { name: 'Sil' }));
    expect(r.getByRole('dialog')).toBeInTheDocument();
    done();
    await waitFor(() => expect(r.queryByRole('dialog')).toBeNull());
  });

  it('has no axe violations with a preview', async () => {
    const r = view({ preview });
    await expectNoAxeViolations(r.container);
  });
});
