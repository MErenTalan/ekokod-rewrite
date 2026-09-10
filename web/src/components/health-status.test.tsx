import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { HealthStatus } from './health-status';
import messages from '../../messages/tr.json';

function renderWithIntl(ui: React.ReactNode) {
  return render(
    <NextIntlClientProvider locale="tr" messages={messages}>
      {ui}
    </NextIntlClientProvider>,
  );
}

describe('HealthStatus', () => {
  it('renders each dependency with its status', () => {
    renderWithIntl(
      <HealthStatus
        report={{
          status: 'ok',
          checks: [
            { name: 'database', status: 'ok', duration_ms: 3 },
            { name: 'redis', status: 'ok', duration_ms: 1 },
          ],
        }}
      />,
    );

    expect(screen.getByText('database')).toBeInTheDocument();
    expect(screen.getByText('redis')).toBeInTheDocument();
    expect(screen.getAllByText('ok')).toHaveLength(2);
  });

  it('shows the failure reason when a check failed', () => {
    renderWithIntl(
      <HealthStatus
        report={{
          status: 'degraded',
          checks: [{ name: 'redis', status: 'failed', error: 'connection refused', duration_ms: 5 }],
        }}
      />,
    );

    expect(screen.getByText(/connection refused/)).toBeInTheDocument();
  });

  it('renders an explicit unavailable state when the API cannot be reached', () => {
    renderWithIntl(<HealthStatus report={null} />);
    expect(screen.getByText(messages.health.unreachable)).toBeInTheDocument();
  });
});
