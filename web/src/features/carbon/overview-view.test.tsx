import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { emptyOverview, overview } from './_fixture';
import { OverviewView } from './overview-view';

describe('OverviewView', () => {
  it('shows the four tiles and the highest source (R322)', () => {
    const r = renderWithProviders(<OverviewView overview={overview} years={[2026, 2025]} onYearChange={vi.fn()} onSeeAll={vi.fn()} />);
    expect(r.getByText('Toplam karbon ayak izi')).toBeVisible();
    expect(r.getByText('12,35')).toBeVisible();
    expect(r.getByText('12.345,68 kg CO₂e')).toBeVisible();
    expect(r.getByText('14')).toBeVisible();
    expect(r.getByText('6')).toBeVisible();
    expect(r.getAllByText('Ortam Isıtması').length).toBeGreaterThan(0);
  });

  it('says pending records are included only when there are some (Q-F4)', () => {
    const r = renderWithProviders(<OverviewView overview={overview} years={[2026]} onYearChange={vi.fn()} onSeeAll={vi.fn()} />);
    expect(r.getByText('2 kayıt onay bekliyor; toplamlara dahildir.')).toBeVisible();
    r.unmount();
    const empty = renderWithProviders(<OverviewView overview={emptyOverview} years={[2026]} onYearChange={vi.fn()} onSeeAll={vi.fn()} />);
    expect(empty.queryByText(/onay bekliyor; toplamlara/)).toBeNull();
  });

  it('lists recent records with status as text and sends "see all" to the status tab', async () => {
    const onSeeAll = vi.fn();
    const r = renderWithProviders(<OverviewView overview={overview} years={[2026]} onYearChange={vi.fn()} onSeeAll={onSeeAll} />);
    expect(r.getByText('Onay Bekliyor')).toBeVisible();
    expect(r.getByText('Onaylandı')).toBeVisible();
    expect(r.getByText('Otomatik')).toBeVisible();
    await r.user.click(r.getByRole('button', { name: 'Tümünü gör' }));
    expect(onSeeAll).toHaveBeenCalled();
  });

  it('says so for a year without records', () => {
    const r = renderWithProviders(<OverviewView overview={emptyOverview} years={[2026]} onYearChange={vi.fn()} onSeeAll={vi.fn()} />);
    expect(r.getAllByText('Bu yıl için kayıt yok').length).toBe(3);
    expect(r.getByText('Kayıt yok')).toBeVisible();
  });
});
