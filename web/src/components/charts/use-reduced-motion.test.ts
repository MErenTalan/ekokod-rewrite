import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { setMatchMedia } from '@/test/match-media';

import { usePrefersReducedMotion } from './use-reduced-motion';

describe('usePrefersReducedMotion', () => {
  afterEach(() => setMatchMedia(() => false));

  it('reads prefers-reduced-motion', () => {
    expect(renderHook(() => usePrefersReducedMotion()).result.current).toBe(false);
    setMatchMedia((q) => q.includes('prefers-reduced-motion: reduce'));
    expect(renderHook(() => usePrefersReducedMotion()).result.current).toBe(true);
  });
});
