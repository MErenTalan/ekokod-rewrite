import { describe, expect, it } from 'vitest';

import { cn } from './cn';

describe('cn', () => {
  it('later type utility wins and does not drop the text colour', () => {
    expect(cn('type-body text-foreground', 'type-small')).toBe('text-foreground type-small');
  });
});
