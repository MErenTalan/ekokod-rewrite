import { afterEach, describe, expect, it } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { BUILDING, clauses, me } from './_fixture';
import { ChecklistPanel } from './checklist-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

describe('ChecklistPanel', () => {
  it('loads a sub-clause only when its clause opens', async () => {
    api = mockApi({
      'GET /api/v1/iso50001/clauses': clauses,
      'GET /api/v1/iso50001/templates': { items: [] },
      [`GET /api/v1/iso50001/${BUILDING}/clauses/5.1/notes`]: { items: [] },
      [`GET /api/v1/iso50001/${BUILDING}/clauses/5.1/files`]: { items: [] },
      [`GET /api/v1/iso50001/${BUILDING}/clauses/5.2/notes`]: { items: [] },
      [`GET /api/v1/iso50001/${BUILDING}/clauses/5.2/files`]: { items: [] },
    });
    const r = renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <ChecklistPanel buildingId={BUILDING} />
      </SessionProvider>,
    );
    await r.findByRole('button', { name: '5. Liderlik' });
    expect(api.calls.some((c) => c.url.includes('/notes'))).toBe(false);
    await r.user.click(r.getByRole('button', { name: '5. Liderlik' }));
    expect((await r.findAllByText('Henüz not eklenmedi.')).length).toBe(2);
  });
});
