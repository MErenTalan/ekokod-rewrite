import { describe, expect, it } from 'vitest';

import type { LoadProfiles } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { SeasonalTabView } from './seasonal-tab';
import { SEASONAL_KEYS } from './profile-label';

const curve = (base: number) => Array.from({ length: 24 }, (_, h) => String(base + h));
const data = {
  profiles: Object.fromEntries(SEASONAL_KEYS.map((key, i) => [key, curve(10 + i)])),
  days: Object.fromEntries(SEASONAL_KEYS.map((key) => [key, 12])),
  config: { weekend_days: [0, 6], weekend_source: 'company', vacations: 1 },
} as unknown as LoadProfiles;

describe('SeasonalTabView', () => {
  it('draws the eight seasonal curves with the right label and note on each', () => {
    const r = renderWithProviders(<SeasonalTabView data={data} />);
    const figures = [...r.container.querySelectorAll('figure[data-profile-key]')];
    expect(figures).toHaveLength(8);

    const summerWeekend = figures.find((f) => f.getAttribute('data-profile-key') === 'summer_weekend')!;
    expect(summerWeekend).toHaveTextContent('Yaz – Hafta sonu');
    expect(summerWeekend).toHaveTextContent('Bu grafik, Yaz mevsimindeki Hafta sonu günlerine ait');
    expect(summerWeekend).toHaveTextContent('12 gün');

    const autumnWeekday = figures.find((f) => f.getAttribute('data-profile-key') === 'autumn_weekday')!;
    expect(autumnWeekday).toHaveTextContent('Sonbahar – Hafta içi');
    expect(autumnWeekday).not.toHaveTextContent('Kış');
  });
});
