import { beforeEach, describe, expect, it, vi } from 'vitest';

const set = vi.fn();
vi.mock('next/headers', () => ({ cookies: async () => ({ set }) }));

describe('setLocale', () => {
  beforeEach(() => set.mockReset());

  it('stores a validated locale for a year', async () => {
    const { setLocale } = await import('./actions');
    await setLocale('en');
    expect(set).toHaveBeenCalledWith('NEXT_LOCALE', 'en', expect.objectContaining({ path: '/', maxAge: 31_536_000, sameSite: 'lax' }));
    await setLocale('de' as 'en');
    expect(set).toHaveBeenLastCalledWith('NEXT_LOCALE', 'tr', expect.anything());
  });
});
