// @vitest-environment node
import { describe, expect, it } from 'vitest';

import { metadata } from './layout';

describe('site layout metadata', () => {
  it('suffixes every page title with the brand (found by the public e2e)', () => {
    expect(metadata.title).toEqual({ template: '%s | EkoKod', default: 'EkoKod' });
  });
});
