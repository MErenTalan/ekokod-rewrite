import { screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { ReportActionsView, type ActionRow } from './report-actions';

const rows: ActionRow[] = [
  { buildingId: 'b-1', buildingName: 'Merkez', reportId: 'r-1', status: 'succeeded' },
  { buildingId: 'b-2', buildingName: 'Depo', reportId: 'r-2', status: 'running' },
  { buildingId: 'b-3', buildingName: 'Atölye', reportId: 'r-3', status: 'failed', errorCode: 'report_building_not_found' },
];

const view = (over: Partial<Parameters<typeof ReportActionsView>[0]> = {}) => ({
  canGenerate: true, canEmail: true, rows, generating: false,
  onGenerate: vi.fn(), onDownload: vi.fn(), onEmail: vi.fn(), ...over,
});

describe('ReportActionsView', () => {
  it('lists one row per building with its state', () => {
    renderWithProviders(<ReportActionsView {...view()} />);
    const table = screen.getByRole('table', { name: 'İndir ve gönder' });
    expect(within(table).getByRole('row', { name: /Merkez/ })).toHaveTextContent('Hazır');
    expect(within(table).getByRole('row', { name: /Depo/ })).toHaveTextContent('Hazırlanıyor');
    expect(within(table).getByRole('row', { name: /Atölye/ })).toHaveTextContent('Bina bulunamadı.');
  });

  it('offers files and e-mail only for a ready report', async () => {
    const props = view();
    const { user } = renderWithProviders(<ReportActionsView {...props} />);
    const ready = within(screen.getByRole('row', { name: /Merkez/ }));
    await user.click(ready.getByRole('button', { name: 'PDF indir' }));
    expect(props.onDownload).toHaveBeenCalledWith('r-1', 'pdf');
    await user.click(ready.getByRole('button', { name: 'Excel indir' }));
    expect(props.onDownload).toHaveBeenCalledWith('r-1', 'excel');
    await user.click(ready.getByRole('button', { name: 'E-posta ile gönder' }));
    expect(props.onEmail).toHaveBeenCalledWith('r-1');
    const running = within(screen.getByRole('row', { name: /Depo/ }));
    expect(running.queryByRole('button', { name: 'PDF indir' })).toBeNull();
  });

  it('hides generation and e-mail without their permissions', () => {
    renderWithProviders(<ReportActionsView {...view({ canGenerate: false, canEmail: false })} />);
    expect(screen.queryByRole('button', { name: 'Raporu oluştur' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'E-posta ile gönder' })).toBeNull();
    expect(screen.getAllByRole('button', { name: 'PDF indir' }).length).toBe(1);
  });

  it('generates on request', async () => {
    const props = view({ rows: [] });
    const { user } = renderWithProviders(<ReportActionsView {...props} />);
    await user.click(screen.getByRole('button', { name: 'Raporu oluştur' }));
    expect(props.onGenerate).toHaveBeenCalled();
  });
});
