import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoAlarms } from './_fixture';
import { AlarmsTabView } from './alarms-tab';

describe('AlarmsTabView', () => {
  it('lists translated faults with level and type (R286)', () => {
    const r = renderWithProviders(<AlarmsTabView items={demoAlarms.items} total={demoAlarms.total} />);
    expect(r.getByText(/şebeke kesintisi hatası oluştu/)).toBeVisible();
    expect(r.getByText('Kritik')).toBeVisible();
    expect(r.getAllByText('Arıza').length).toBeGreaterThan(0);
  });

  it('marks a fault the dictionary could not translate', () => {
    const r = renderWithProviders(<AlarmsTabView items={demoAlarms.items} total={demoAlarms.total} />);
    expect(r.getByText(/Çevrilemedi/)).toBeVisible();
  });

  it('has an empty state', () => {
    const r = renderWithProviders(<AlarmsTabView items={[]} total={0} />);
    expect(r.getByText(/Alarm kaydı yok/)).toBeVisible();
  });
});
