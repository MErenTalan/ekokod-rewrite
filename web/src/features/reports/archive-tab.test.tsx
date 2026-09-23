import { screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoArchive } from './_fixture';
import { ArchiveTabView, groupArchive } from './archive-tab';

const view = (over: Partial<Parameters<typeof ArchiveTabView>[0]> = {}) => ({
  items: demoArchive, total: demoArchive.length, loading: false,
  filters: { type: 'all' as const, buildingId: 'all' },
  buildings: [{ value: 'b-1', label: 'Merkez' }, { value: 'b-2', label: 'Depo' }],
  onFilters: vi.fn(), onDownload: vi.fn(), ...over,
});

describe('groupArchive', () => {
  it('groups by year, newest first, the yearly report apart from the months', () => {
    const groups = groupArchive(demoArchive);
    expect(groups.map((g) => g.year)).toEqual(['2026', '2025']);
    expect(groups[1].yearly?.id).toBe('r-1');
    expect(groups[1].monthly.map((m) => m.period)).toEqual(['2025-12', '2025-11']);
    expect(groups[0].yearly).toBeUndefined();
  });
});

describe('ArchiveTabView', () => {
  it('counts the reports and shows each one’s availability', () => {
    renderWithProviders(<ArchiveTabView {...view()} />);
    expect(screen.getByText('Toplam rapor: 4')).toBeVisible();
    expect(screen.getByRole('heading', { name: '2025 raporları' })).toBeVisible();
    expect(screen.getByText('2 ay')).toBeVisible();
    const error = screen.getByRole('listitem', { name: /Kasım 2025/ });
    expect(error).toHaveTextContent('Hata');
    expect(within(error).queryByRole('button', { name: /PDF/ })).toBeNull();
    const pending = screen.getByRole('listitem', { name: /Ağustos 2026/ });
    expect(pending).toHaveTextContent('Hazırlanıyor');
    expect(within(pending).queryByRole('button', { name: /PDF/ })).toBeNull();
  });

  it('downloads a completed report', async () => {
    const props = view();
    const { user } = renderWithProviders(<ArchiveTabView {...props} />);
    const december = screen.getByRole('listitem', { name: /Aralık 2025/ });
    await user.click(within(december).getByRole('button', { name: /Excel/ }));
    expect(props.onDownload).toHaveBeenCalledWith('r-2', 'excel');
  });

  it('says what to do when nothing matches', () => {
    renderWithProviders(<ArchiveTabView {...view({ items: [], total: 0 })} />);
    expect(screen.getByText('Rapor bulunamadı')).toBeVisible();
  });
});
