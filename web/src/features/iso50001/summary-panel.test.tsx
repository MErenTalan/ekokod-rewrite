import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { BUILDING, clauses, me, project } from './_fixture';
import { SummaryPanel } from './summary-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

let sent: unknown = null;
const routes = {
  [`GET /api/v1/iso50001/${BUILDING}`]: project,
  'GET /api/v1/iso50001/clauses': clauses,
  [`PUT /api/v1/iso50001/${BUILDING}/dates`]: (req: Request) =>
    req.json().then((b: unknown) => {
      sent = b;
      return Response.json(project);
    }),
};

describe('SummaryPanel', () => {
  it('saves the calendar for the selected building', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('building_admin')}>
        <SummaryPanel buildingId={BUILDING} />
      </SessionProvider>,
    );
    await r.user.click(await r.findByRole('button', { name: 'Takvimi Güncelle' }));
    await r.user.click(await r.findByRole('button', { name: 'Tarihleri Kaydet' }));
    await waitFor(() => expect(sent).toEqual({ clauses: project.clauses.map((c) => ({ clause_id: c.clause_id, start: c.start, end: c.end })) }));
  });

  it('shows a read-only role no calendar', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('building_readonly_admin')}>
        <SummaryPanel buildingId={BUILDING} />
      </SessionProvider>,
    );
    expect(await r.findByRole('button', { name: 'ISO 50001 Klasörünü İndir' })).toBeVisible();
    expect(r.queryByRole('button', { name: 'Takvimi Güncelle' })).toBeNull();
  });
});
