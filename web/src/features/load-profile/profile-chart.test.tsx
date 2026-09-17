import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { ProfileChart, profileData } from './profile-chart';
import { demoProfiles } from './_fixture';

describe('profileData', () => {
  it('lays the 24 hours out as the x axis and keeps gaps as gaps', () => {
    const data = profileData({ weekday: ['1', null, '3', ...Array(21).fill(null)] } as never, ['weekday']);
    expect(data).toHaveLength(24);
    expect(data[0]).toEqual({ x: '00:00', weekday: '1' });
    expect(data[1]).toEqual({ x: '01:00', weekday: null });
    expect(data[23].x).toBe('23:00');
  });

  it('gives a missing profile 24 empty hours rather than failing', () => {
    const data = profileData({}, ['summer_weekend']);
    expect(data.every((point) => point.summer_weekend === null)).toBe(true);
  });
});

describe('ProfileChart', () => {
  it('names the profile and counts the days behind it', () => {
    const r = renderWithProviders(<ProfileChart data={demoProfiles} keys={['weekday']} />);
    // The title and the legend both carry the label.
    expect(r.getByRole('heading', { name: 'Hafta içi' })).toBeInTheDocument();
    expect(r.getByText(/10 gün/)).toBeInTheDocument();
  });

  it('drops the per-profile note when several are overlaid', () => {
    const r = renderWithProviders(<ProfileChart data={demoProfiles} keys={['weekday', 'weekend']} title="Karşılaştırma" />);
    expect(r.queryByText(/tüm hafta içi günleri için hesaplanan/)).toBeNull();
  });
});
