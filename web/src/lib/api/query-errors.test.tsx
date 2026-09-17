import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { $api } from './query';

function Screen() {
  const q = $api.useQuery('get', '/api/v1/buildings', { params: { query: {} } });
  return <p>{q.isError ? 'hata' : q.data ? 'yüklendi' : 'boş'}</p>;
}

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

describe('QueryErrorToaster', () => {
  it('says once that a read failed, instead of an empty screen (07 §11)', async () => {
    api = mockApi({
      'GET /api/v1/buildings': Response.json({ error: { code: 'internal', message: 'Bina listesi alınamadı' } }, { status: 500 }),
    });
    const r = renderWithProviders(<Screen />);
    // The client retries a 5xx twice before it gives up, so give it room.
    await waitFor(() => expect(r.getByText('Bina listesi alınamadı')).toBeInTheDocument(), { timeout: 10_000 });
    expect(r.getAllByText('Bina listesi alınamadı')).toHaveLength(1);
  });

  it('falls back to its own wording when the API sends none', async () => {
    api = mockApi({ 'GET /api/v1/buildings': new Response(null, { status: 503 }) });
    const r = renderWithProviders(<Screen />);
    await waitFor(() => expect(r.getByText('Veriler yüklenemedi')).toBeInTheDocument(), { timeout: 10_000 });
  });

  it('stays quiet about a refused read: the UI already hides what a role may not see', async () => {
    api = mockApi({
      'GET /api/v1/buildings': Response.json({ error: { code: 'forbidden', message: 'Yetkiniz yok' } }, { status: 403 }),
    });
    const r = renderWithProviders(<Screen />);
    // A refused read is not retried, so the query is settled once this shows.
    await waitFor(() => expect(r.getByText('hata')).toBeInTheDocument());
    expect(r.queryByText('Yetkiniz yok')).toBeNull();
    expect(r.queryByText('Veriler yüklenemedi')).toBeNull();
  });

  it('stays quiet when the read succeeds', async () => {
    api = mockApi({ 'GET /api/v1/buildings': { items: [] } });
    const r = renderWithProviders(<Screen />);
    await waitFor(() => expect(r.getByText('yüklendi')).toBeInTheDocument());
    expect(r.queryByText('Veriler yüklenemedi')).toBeNull();
  });
});
