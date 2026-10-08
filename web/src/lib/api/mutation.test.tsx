import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { useApiMutation } from './mutation';

function Form({ withField = false }: { withField?: boolean }) {
  const create = useApiMutation('post', '/api/v1/buildings', {
    success: 'Kaydedildi',
    invalidate: ['/api/v1/buildings'],
  });
  return (
    <>
      {withField ? <Input label="Bina adı" value="" onChange={() => {}} error={create.fieldErrors.name} /> : null}
      <Button onClick={() => create.mutate({ body: { name: 'A1 Fabrika' } })}>Kaydet</Button>
      <output data-testid="errors">{JSON.stringify(create.fieldErrors)}</output>
    </>
  );
}

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

describe('useApiMutation', () => {
  it('toasts on success', async () => {
    api = mockApi({ 'POST /api/v1/buildings': Response.json({ id: 'b-1' }, { status: 201 }) });
    const r = renderWithProviders(<Form />);
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(r.getByText('Kaydedildi')).toBeInTheDocument());
    expect(r.getByTestId('errors')).toHaveTextContent('{}');
  });

  it('turns 422 details into field errors and shows the API message', async () => {
    api = mockApi({
      'POST /api/v1/buildings': Response.json(
        { error: { code: 'validation_failed', message: 'Doğrulama başarısız.', details: { name: ['required'] } } },
        { status: 422 },
      ),
    });
    const r = renderWithProviders(<Form />);
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(r.getByTestId('errors')).toHaveTextContent('{"name":"Zorunlu alan"}'));
    expect(r.getByText('Doğrulama başarısız.')).toBeInTheDocument();
  });

  it('names the marked fields in the toast and focuses the first one', async () => {
    api = mockApi({
      'POST /api/v1/buildings': Response.json(
        { error: { code: 'validation_failed', message: 'Girilen bilgiler geçersiz.', details: { name: ['decimal'] } } },
        { status: 422 },
      ),
    });
    const r = renderWithProviders(<Form withField />);
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() =>
      expect(r.getByText('Düzeltilmesi gereken alanlar: Bina adı (Geçerli bir sayı girin, örn. 1234,56)')).toBeInTheDocument(),
    );
    expect(r.getByText('Girilen bilgiler geçersiz.')).toBeInTheDocument();
    expect(r.getByRole('textbox', { name: 'Bina adı' })).toHaveFocus();
  });

  it('lists a field the form does not show by its API name', async () => {
    api = mockApi({
      'POST /api/v1/buildings': Response.json(
        { error: { code: 'validation_failed', message: 'Girilen bilgiler geçersiz.', details: { sector: ['oneof'] } } },
        { status: 422 },
      ),
    });
    const r = renderWithProviders(<Form />);
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(r.getByText('Düzeltilmesi gereken alanlar: sector (Geçersiz seçim)')).toBeInTheDocument());
  });

  it('gives a support reference for an error the operator cannot fix', async () => {
    api = mockApi({
      'POST /api/v1/buildings': Response.json(
        { error: { code: 'invalid_body', message: 'İstek gövdesi geçersiz.', details: { reason: 'unknown field', field: 'x' }, request_id: 'req-42' } },
        { status: 400 },
      ),
    });
    const r = renderWithProviders(<Form />);
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(r.getByText('İstek gövdesi geçersiz.')).toBeInTheDocument());
    expect(r.getByText(/şu kodu iletin: req-42/)).toBeInTheDocument();
  });

  it('says the server could not be reached on a network failure', async () => {
    api = mockApi({ 'POST /api/v1/buildings': () => Promise.reject(new TypeError('Failed to fetch')) });
    const r = renderWithProviders(<Form />);
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(r.getByText('Kaydedilemedi')).toBeInTheDocument());
    expect(r.getByText('Sunucuya ulaşılamadı. Bağlantınızı kontrol edip tekrar deneyin.')).toBeInTheDocument();
  });

  it('reads a proxy error page as the server not being reached', async () => {
    api = mockApi({ 'POST /api/v1/buildings': new Response('<html>502 Bad Gateway</html>', { status: 502, headers: { 'content-type': 'text/html' } }) });
    const r = renderWithProviders(<Form />);
    await r.user.click(r.getByRole('button', { name: 'Kaydet' }));
    await waitFor(() => expect(r.getByText('Sunucuya ulaşılamadı. Bağlantınızı kontrol edip tekrar deneyin.')).toBeInTheDocument());
  });
});
