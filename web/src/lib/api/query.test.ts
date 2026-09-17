import { describe, expect, it } from 'vitest';

import { makeQueryClient } from './query';

const retry = () => makeQueryClient().getDefaultOptions().queries!.retry as (failures: number, error: unknown) => boolean;

const envelope = (code: string) => ({ error: { code, message: 'x' } });

describe('query retry policy', () => {
  it('does not retry what the API refused or rejected', () => {
    // The thrown value is the error envelope, not an HTTP-aware error: the code
    // is the only thing that says whether a retry could ever help.
    for (const code of ['unauthorized', 'forbidden', 'not_found', 'validation_failed']) {
      expect(retry()(0, envelope(code)), code).toBe(false);
    }
  });

  it('retries a server error and a transport failure, twice', () => {
    expect(retry()(0, envelope('internal'))).toBe(true);
    expect(retry()(1, envelope('internal'))).toBe(true);
    expect(retry()(2, envelope('internal'))).toBe(false);
    expect(retry()(0, new TypeError('Failed to fetch'))).toBe(true);
  });
});
