import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { FieldMessages } from './_field-messages';

describe('FieldMessages', () => {
  it('ids match describedBy and nothing renders without messages', async () => {
    expect(FieldMessages.describedBy('x', 'd', 'e')).toBe('x-description x-error');
    expect(FieldMessages.describedBy('x')).toBeUndefined();
    const { container, rerender } = renderWithProviders(<FieldMessages controlId="x" description="Açıklama" error="Hata" />);
    expect(container.querySelector('#x-error')).toHaveTextContent('Hata');
    await expectNoAxeViolations(container);
    rerender(<FieldMessages controlId="x" />);
    expect(container.querySelector('#x-error, #x-description')).toBeNull();
  });
});
