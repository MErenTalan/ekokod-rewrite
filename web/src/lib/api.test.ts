import { afterEach, describe, expect, it, vi } from 'vitest';

import { fetchHealth } from './api';

const respond = (body: unknown) => vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(body)));

describe('fetchHealth', () => {
  afterEach(() => vi.restoreAllMocks());

  it('returns a well-formed readiness report', async () => {
    respond({ status: 'ok', checks: [{ name: 'database', status: 'ok', duration_ms: 2 }] });
    expect((await fetchHealth())?.checks).toHaveLength(1);
  });

  it('treats an unexpected body (another service on the port) as unreachable', async () => {
    respond({ hello: 'world' });
    expect(await fetchHealth()).toBeNull();
  });

  it('treats a network error as unreachable', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('ECONNREFUSED'));
    expect(await fetchHealth()).toBeNull();
  });
});
