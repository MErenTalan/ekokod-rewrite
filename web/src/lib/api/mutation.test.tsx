import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { Button } from '@/components/ui/button';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { useApiMutation } from './mutation';

function Form() {
  const create = useApiMutation('post', '/api/v1/buildings', {
    success: 'Kaydedildi',
    invalidate: ['/api/v1/buildings'],
  });
  return (
    <>
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
});
