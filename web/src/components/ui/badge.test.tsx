import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Badge } from './badge';

describe('Badge', () => {
  it('forwards span props so it can be a focusable trigger', () => {
    const { getByText } = renderWithProviders(
      <Badge tone="warning" tabIndex={0}>
        Tahmini
      </Badge>,
    );
    expect(getByText('Tahmini')).toHaveAttribute('tabindex', '0');
    expect(getByText('Tahmini')).toHaveClass('text-warning');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <div>
        {(['neutral', 'brand', 'info', 'success', 'warning', 'danger'] as const).map((tone) => (
          <Badge key={tone} tone={tone}>
            {tone}
          </Badge>
        ))}
      </div>,
    );
    await expectNoAxeViolations(container);
  });
});
