import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { AuthMessage } from './auth-message';

describe('AuthMessage', () => {
  it('shows the message and the way forward', async () => {
    const ui = renderWithProviders(
      <AuthMessage tone="danger" title="Giriş yapılamadı" description="Bir sorun oluştu." action={{ href: '/auth/login', label: 'Girişe Dön' }} />,
    );
    expect(ui.getByRole('heading', { level: 1 })).toHaveTextContent('Giriş yapılamadı');
    expect(ui.getByRole('alert')).toHaveTextContent('Bir sorun oluştu.');
    expect(ui.getByRole('link', { name: 'Girişe Dön' })).toHaveAttribute('href', '/auth/login');
    await expectNoAxeViolations(ui.container);
  });
});
