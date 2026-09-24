import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { AuthSplitLayout } from './auth-split-layout';

describe('AuthSplitLayout', () => {
  it('renders the form in main and hides the brand panel below md', () => {
    const ui = renderWithProviders(
      <AuthSplitLayout>
        <h1>Kullanıcı Girişi</h1>
      </AuthSplitLayout>,
    );
    expect(ui.getByRole('main')).toHaveTextContent('Kullanıcı Girişi');
    const aside = ui.getByRole('complementary');
    expect(aside).toHaveClass('hidden', 'md:flex');
    expect(aside).toHaveTextContent('Enerjinizi ölçün, anlayın, azaltın');
    expect(ui.getByRole('button', { name: /dil/i })).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const ui = renderWithProviders(
      <AuthSplitLayout>
        <h1>Kullanıcı Girişi</h1>
      </AuthSplitLayout>,
    );
    await expectNoAxeViolations(ui.container);
  });
});
