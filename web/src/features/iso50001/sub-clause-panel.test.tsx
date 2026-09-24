import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { BUILDING, file, me, note } from './_fixture';
import { SubClausePanel } from './sub-clause-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

let added: unknown = null;
const routes = {
  [`GET /api/v1/iso50001/${BUILDING}/clauses/5.1/notes`]: { items: [note()] },
  [`GET /api/v1/iso50001/${BUILDING}/clauses/5.1/files`]: { items: [file()] },
  [`POST /api/v1/iso50001/${BUILDING}/clauses/5.1/notes`]: (req: Request) =>
    req.json().then((b: unknown) => {
      added = b;
      return Response.json(note({ id: 'n-new' }), { status: 201 });
    }),
  [`POST /api/v1/iso50001/${BUILDING}/clauses/5.1/files`]: () =>
    Response.json({ error: { code: 'validation_failed', message: 'Geçersiz', details: { file: ['type_not_allowed'] } } }, { status: 422 }),
};

describe('SubClausePanel', () => {
  it('adds a note to the building sub-clause', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('building_admin')}>
        <SubClausePanel buildingId={BUILDING} clause="5.1" />
      </SessionProvider>,
    );
    expect(await r.findByText('Politika.pdf')).toBeVisible();
    await r.user.type(r.getByRole('textbox', { name: /^Not/ }), 'Kanıt eklendi');
    await r.user.click(r.getByRole('button', { name: 'Notu Listeye Ekle' }));
    await waitFor(() => expect(added).toEqual({ body: 'Kanıt eklendi' }));
  });

  it('shows the server refusal of an upload on the files block', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <SubClausePanel buildingId={BUILDING} clause="5.1" />
      </SessionProvider>,
    );
    await r.findByText('Politika.pdf');
    await r.user.upload(r.getByLabelText(/Gerekli Dosyaları Yükle/), new File(['MZ'], 'fatura.pdf', { type: 'application/pdf' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'POST' && c.url.includes('/files'))).toBe(true));
    expect(await r.findByText('Bu dosya türüne izin verilmiyor: PDF, Excel, Word, CSV, PNG veya JPEG yükleyin.')).toBeVisible();
  });
});
