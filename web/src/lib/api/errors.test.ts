import { describe, expect, it } from 'vitest';

import { authRedirectFor, errorCode, safeNext } from './errors';

describe('errorCode', () => {
  it('reads the envelope code', () => {
    expect(errorCode({ error: { code: 'token_expired', message: 'x' } })).toBe('token_expired');
    expect(errorCode({ nope: true })).toBeUndefined();
    expect(errorCode(null)).toBeUndefined();
  });
});

describe('safeNext', () => {
  it.each([
    ['/ekorm/consumption?x=1', '/ekorm/consumption?x=1'],
    ['/ekorm', '/ekorm'],
    // R492: every module and the chooser are valid targets; anything else lands on the chooser (U3).
    ['/ekocm/data-entry', '/ekocm/data-entry'],
    ['/ekocm?x=1', '/ekocm?x=1'],
    ['/modules', '/modules'],
    [undefined, '/modules'],
    ['//evil.com', '/modules'],
    ['https://evil.com/ekorm', '/modules'],
    ['/auth/login', '/modules'],
    ['/ekormevil', '/modules'],
    ['/ekocmevil', '/modules'],
    ['/modulesx', '/modules'],
    ['/ekorm/\\evil.com', '/modules'],
    ['/ekocm/../auth/login', '/modules'],
    ['/ekorm/./x', '/modules'],
    ['/ekorm/a..b', '/ekorm/a..b'],
  ])('%s → %s', (input, want) => {
    expect(safeNext(input)).toBe(want);
  });
});

describe('authRedirectFor', () => {
  it('sends device mismatch to login with the reason', () => {
    expect(authRedirectFor('device_mismatch', '/ekorm/x')).toBe(
      '/auth/login?reason=device_mismatch',
    );
  });
  it.each(['session_revoked', 'token_invalid', 'token_expired', 'token_rotated', 'unauthorized'])(
    '%s returns to login with next',
    (code) => {
      expect(authRedirectFor(code, '/ekorm/consumption')).toBe(
        '/auth/login?next=%2Fekorm%2Fconsumption',
      );
    },
  );
  it('leaves other errors alone', () => {
    expect(authRedirectFor('invalid_credentials', '/ekorm')).toBeNull();
    expect(authRedirectFor('forbidden', '/ekorm')).toBeNull();
  });
});
