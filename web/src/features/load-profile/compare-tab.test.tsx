import { describe, expect, it, vi } from 'vitest';

import type { LoadProfiles } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { CompareTabView, MAX_COMPARED } from './compare-tab';
import { PROFILE_KEYS, type ProfileKey } from './profile-label';

const curve = (base: number) => Array.from({ length: 24 }, (_, h) => String(base + h));
const data = {
  profiles: Object.fromEntries(PROFILE_KEYS.map((key, i) => [key, curve(10 + i)])),
  days: {},
  config: { weekend_days: [0, 6], weekend_source: 'default', vacations: 0 },
} as unknown as LoadProfiles;

describe('CompareTabView', () => {
  it('overlays the selected profiles', () => {
    const r = renderWithProviders(
      <CompareTabView data={data} selected={['weekday', 'summer_weekend']} onSelectedChange={() => {}} />,
    );
    const legend = r.getAllByRole('list')[0];
    expect(legend).toHaveTextContent('Hafta içi');
    expect(legend).toHaveTextContent('Yaz – Hafta sonu');
  });

  it('refuses a seventh profile and says why (D21)', () => {
    const six = PROFILE_KEYS.slice(0, MAX_COMPARED) as ProfileKey[];
    const onSelectedChange = vi.fn();
    const r = renderWithProviders(<CompareTabView data={data} selected={six} onSelectedChange={onSelectedChange} />);
    expect(r.getByText('En fazla 6 profil seçebilirsiniz')).toBeInTheDocument();
  });
});
