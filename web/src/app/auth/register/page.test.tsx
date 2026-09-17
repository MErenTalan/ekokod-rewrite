import { describe, expect, it, vi } from 'vitest';

const redirect = vi.hoisted(() =>
  vi.fn((href: string) => {
    throw new Error(`NEXT_REDIRECT ${href}`);
  }),
);
vi.mock('next/navigation', () => ({ redirect }));

import RegisterPage from './page';

describe('register page', () => {
  it('redirects to login', () => {
    expect(() => RegisterPage()).toThrow('NEXT_REDIRECT /auth/login');
    expect(redirect).toHaveBeenCalledWith('/auth/login');
  });
});
