import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { report } from './_fixture';
import { ReportHistory } from './report-history';

describe('ReportHistory', () => {
  it('lists reports with their period and type and downloads the PDF (R326)', async () => {
    const onDownload = vi.fn();
    const rows = [report(), report({ id: 'r-2', report_type: 'iso', name: 'ISO yarıyıl' })];
    const r = renderWithProviders(<ReportHistory rows={rows} hasMore={false} onLoadMore={vi.fn()} onDownload={onDownload} />);
    expect(r.getByText('GHG Protocol 2026-01-01 – 2026-06-30')).toBeVisible();
    expect(r.getAllByText('1 Oca 2026 – 30 Haz 2026')).toHaveLength(2);
    expect(r.getByText('ISO 14064')).toBeVisible();
    await r.user.click(r.getByRole('button', { name: 'ISO yarıyıl PDF indir' }));
    expect(onDownload).toHaveBeenCalledWith(rows[1]);
  });

  it('says so when there are no reports', () => {
    const r = renderWithProviders(<ReportHistory rows={[]} hasMore={false} onLoadMore={vi.fn()} onDownload={vi.fn()} />);
    expect(r.getByText('Henüz rapor yok')).toBeVisible();
  });
});
