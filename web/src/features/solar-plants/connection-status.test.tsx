import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoRealtime } from './_fixture';
import { ConnectionStatus } from './connection-status';

describe('ConnectionStatus', () => {
  it('shows the chip and the last sync time (R282)', () => {
    const r = renderWithProviders(<ConnectionStatus realtime={demoRealtime} />);
    expect(r.getByText('Bağlı')).toBeVisible();
    expect(r.getByText(/Son eşitleme/)).toBeVisible();
  });

  it("says what a closed error code means and never the worker's text", () => {
    const r = renderWithProviders(<ConnectionStatus realtime={{ ...demoRealtime, connection: 'error', connection_error: 'isolar_auth' }} />);
    expect(r.getByText('Bağlantı hatası')).toBeVisible();
    expect(r.getByText(/yetkilendirmesi geçersiz/)).toBeVisible();
  });

  it('ignores a code outside the closed set', () => {
    const r = renderWithProviders(<ConnectionStatus realtime={{ ...demoRealtime, connection: 'error', connection_error: 'boom: secret' }} />);
    expect(r.queryByText(/boom/)).toBeNull();
  });
});
