import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Alert } from './alert';

describe('Alert', () => {
  it('danger announces, info does not interrupt', () => {
    const { getByRole, rerender } = renderWithProviders(<Alert tone="danger" title="Fatura hesaplanamadı" />);
    expect(getByRole('alert')).toHaveTextContent('Fatura hesaplanamadı');
    rerender(<Alert tone="info" title="Veriler güncelleniyor" />);
    expect(getByRole('status')).toHaveTextContent('Veriler güncelleniyor');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <Alert tone="warning" title="Eksik veri" action={<button type="button">Yenile</button>}>
        3 saatlik ölçüm eksik
      </Alert>,
    );
    await expectNoAxeViolations(container);
  });
});
