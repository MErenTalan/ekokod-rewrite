import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { BUILDING, me, report } from './_fixture';
import { ReportingPanel } from './reporting-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

let sent: Record<string, unknown> | null = null;
const routes = {
  'GET /api/v1/carbon/reports': { items: [report()], next_cursor: null },
  'POST /api/v1/carbon/reports': (req: Request) =>
    req.json().then((b: Record<string, unknown>) => {
      sent = b;
      return Response.json(report({ id: 'r-new' }), { status: 201 });
    }),
};

describe('ReportingPanel', () => {
  it('generates a report for the selected building', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <ReportingPanel buildingId={BUILDING} />
      </SessionProvider>,
    );
    await r.user.click(await r.findByRole('button', { name: 'Raporu oluştur' }));
    await waitFor(() => expect(sent).toMatchObject({ building_id: BUILDING, report_type: 'ghg' }));
    expect(new URL(api.calls[0].url).searchParams.get('building_id')).toBe(BUILDING);
  });

  it('shows a building admin the history without the form', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('building_admin')}>
        <ReportingPanel buildingId={BUILDING} />
      </SessionProvider>,
    );
    expect(await r.findByText('GHG Protocol 2026-01-01 – 2026-06-30')).toBeVisible();
    expect(r.queryByRole('button', { name: 'Raporu oluştur' })).toBeNull();
  });
});
