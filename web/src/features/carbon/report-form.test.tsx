import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { ReportForm, reportRangeError } from './report-form';

describe('reportRangeError', () => {
  it('refuses more than 366 inclusive days and a future end (R312)', () => {
    expect(reportRangeError({ from: '2026-01-01', to: '2026-06-30' }, '2026-09-10')).toBeNull();
    expect(reportRangeError({ from: '2025-09-10', to: '2026-09-10' }, '2026-09-10')).toBeNull();
    expect(reportRangeError({ from: '2025-09-09', to: '2026-09-10' }, '2026-09-10')).toBe('tooLong');
    expect(reportRangeError({ from: '2026-09-01', to: '2026-09-11' }, '2026-09-10')).toBe('future');
  });
});

describe('ReportForm', () => {
  it('sends the type, period and trimmed name', async () => {
    const onSubmit = vi.fn();
    const r = renderWithProviders(<ReportForm today="2026-09-10" initialRange={{ from: '2026-01-01', to: '2026-06-30' }} saving={false} errors={{}} onSubmit={onSubmit} />);
    await r.user.click(r.getByRole('radio', { name: 'ISO 14064' }));
    await r.user.type(r.getByRole('textbox', { name: 'Rapor adı (isteğe bağlı)' }), '  İlk yarı  ');
    await r.user.click(r.getByRole('button', { name: 'Raporu oluştur' }));
    expect(onSubmit).toHaveBeenCalledWith({ report_type: 'iso', from: '2026-01-01', to: '2026-06-30', name: 'İlk yarı' });
  });

  it('explains a period it cannot send', () => {
    const r = renderWithProviders(<ReportForm today="2026-09-10" initialRange={{ from: '2025-01-01', to: '2026-06-30' }} saving={false} errors={{}} onSubmit={vi.fn()} />);
    expect(r.getByText('Dönem en fazla 366 gün olabilir.')).toBeVisible();
    expect(r.getByRole('button', { name: 'Raporu oluştur' })).toBeDisabled();
  });
});
