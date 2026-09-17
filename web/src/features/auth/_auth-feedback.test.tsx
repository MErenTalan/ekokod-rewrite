import { waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { AuthFeedback } from './_auth-feedback';

describe('AuthFeedback', () => {
  it('takes focus whenever the message changes', async () => {
    const ui = renderWithProviders(<AuthFeedback tone="danger" message="E-posta veya şifre hatalı." />);
    const box = ui.getByRole('alert').parentElement!;
    await waitFor(() => expect(box).toHaveFocus());
    (document.activeElement as HTMLElement).blur();
    ui.rerender(<AuthFeedback tone="danger" message="Çok fazla deneme yapıldı." />);
    await waitFor(() => expect(box).toHaveFocus());
  });
});
