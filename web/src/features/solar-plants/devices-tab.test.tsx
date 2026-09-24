import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoDevices } from './_fixture';
import { DevicesTabView } from './devices-tab';

describe('DevicesTabView', () => {
  it('lists devices with their count, status and power (R285)', () => {
    const r = renderWithProviders(<DevicesTabView devices={demoDevices} query="" onQueryChange={() => {}} />);
    expect(r.getByText(/2 cihaz/)).toBeVisible();
    expect(r.getByText('A2103456789')).toBeVisible();
    expect(r.getByText('Normal')).toBeVisible();
    expect(r.getByText('Arıza')).toBeVisible();
  });

  it('searches by name or serial through the API', async () => {
    const onQueryChange = vi.fn();
    function Harness() {
      const [q, setQ] = useState('');
      return <DevicesTabView devices={demoDevices} query={q} onQueryChange={(v) => { setQ(v); onQueryChange(v); }} />;
    }
    const r = renderWithProviders(<Harness />);
    await userEvent.type(r.getByRole('searchbox', { name: /Cihaz ara/ }), 'B99');
    expect(onQueryChange).toHaveBeenLastCalledWith('B99');
  });

  it('has an empty state when no device is returned', () => {
    const r = renderWithProviders(<DevicesTabView devices={[]} query="" onQueryChange={() => {}} />);
    expect(r.getByText(/Cihaz bulunamadı/)).toBeVisible();
  });
});
