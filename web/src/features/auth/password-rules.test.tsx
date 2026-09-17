import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { PasswordRules } from './password-rules';

describe('PasswordRules', () => {
  it('announces met and unmet rules', () => {
    const ui = renderWithProviders(<PasswordRules password="kisa" />);
    const items = ui.getAllByRole('listitem');
    expect(items[0]).toHaveTextContent('En az 10 karakter olmalıdır: Karşılanmadı');
    expect(items[1]).toHaveTextContent('En az 1 küçük harf içermelidir: Karşılandı');
    expect(items).toHaveLength(8);
  });

  it('has no axe violations', async () => {
    const ui = renderWithProviders(<PasswordRules password="Guvenli!Sifre-42" />);
    await expectNoAxeViolations(ui.container);
  });
});
