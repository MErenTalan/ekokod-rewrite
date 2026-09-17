import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { DailyTabView } from './daily-tab';
import { demoProfiles } from './_fixture';

describe('DailyTabView', () => {
  it('shows the weekday and weekend curves with their notes', () => {
    const r = renderWithProviders(<DailyTabView data={demoProfiles} />);
    const keys = [...r.container.querySelectorAll('figure[data-profile-key]')].map((f) => f.getAttribute('data-profile-key'));
    expect(keys).toEqual(['weekday', 'weekend']);
    expect(r.getByText(/tüm hafta içi günleri için hesaplanan/)).toBeInTheDocument();
    expect(r.getByText(/tüm hafta sonu günleri için hesaplanan/)).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const r = renderWithProviders(<DailyTabView data={demoProfiles} />);
    await expectNoAxeViolations(r.container);
  });
});
