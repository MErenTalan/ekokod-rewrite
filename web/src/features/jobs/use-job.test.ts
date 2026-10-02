import { describe, expect, it } from 'vitest';

import { childProgress, nothingToFetch } from './use-job';

const base = { already_queued: 0, running: 0, succeeded: 0, partial: 0, failed: 0, errors: [] };

describe('childProgress', () => {
  it('keeps a backfill running until every queued fetch finished', () => {
    expect(childProgress({ ...base, enqueued: 10, succeeded: 3, running: 1 })).toEqual({
      done: 3,
      total: 10,
      status: 'running',
    });
  });

  it('fails a backfill whose every fetch failed (the silent GridBox case)', () => {
    expect(childProgress({ ...base, enqueued: 4, failed: 4 })).toEqual({
      done: 4,
      total: 4,
      status: 'failed',
    });
  });

  it('succeeds once all finished and at least one fetched data', () => {
    expect(childProgress({ ...base, enqueued: 4, failed: 1, succeeded: 3 })).toEqual({
      done: 4,
      total: 4,
      status: 'succeeded',
    });
  });
});

describe('nothingToFetch', () => {
  it('is a refresh the provider has no data for, not a finished 0 of 0', () => {
    expect(nothingToFetch({ ...base, enqueued: 0 })).toBe(true);
    expect(nothingToFetch(base)).toBe(true);
  });

  it('is not a run whose windows were already queued or did some work', () => {
    expect(nothingToFetch({ ...base, already_queued: 2 })).toBe(false);
    expect(nothingToFetch({ ...base, enqueued: 1, succeeded: 1 })).toBe(false);
    expect(nothingToFetch({ ...base, running: 1 })).toBe(false);
  });
});
