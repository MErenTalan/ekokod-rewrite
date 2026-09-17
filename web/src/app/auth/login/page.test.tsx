import { describe, expect, it, vi } from 'vitest';

vi.mock('@/features/auth/login-form', () => ({ LoginForm: (props: object) => ({ type: 'LoginForm', props }) }));

import LoginPage from './page';

describe('login page', () => {
  it('passes the first next and reason values to the form', async () => {
    const element = await LoginPage({ searchParams: Promise.resolve({ next: ['/ekorm/a', '/ekorm/b'], reason: 'device_mismatch' }) });
    expect(element.props).toEqual({ next: '/ekorm/a', reason: 'device_mismatch' });
  });
});
