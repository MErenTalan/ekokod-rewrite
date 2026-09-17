import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Skeleton, SkeletonText } from './skeleton';

describe('Skeleton', () => {
  it('is hidden from assistive technology and takes caller size', () => {
    const { container } = renderWithProviders(
      <div>
        <Skeleton className="h-8 w-40" />
        <SkeletonText lines={3} />
      </div>,
    );
    const blocks = container.querySelectorAll('[aria-hidden="true"]');
    expect(blocks[0]).toHaveClass('h-8', 'w-40');
    expect(container.querySelectorAll('[data-skeleton-line]')).toHaveLength(3);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<SkeletonText lines={2} />);
    await expectNoAxeViolations(container);
  });
});
