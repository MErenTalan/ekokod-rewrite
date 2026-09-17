import { describe, expect, it } from 'vitest';

import { passwordChecks, violationKey } from './password-policy';

const unmet = (pw: string) => passwordChecks(pw).filter((c) => !c.met).map((c) => c.rule);

// Same inputs as internal/auth/policy_test.go where the rule needs no server context.
describe('passwordChecks', () => {
  it.each([
    ['Kisa!1a', ['minLength']],
    ['abcdefghijK1', ['special', 'noSequence']],
    ['Aaaaa!2345xy', ['noRepeat']],
    ['Qwer!9876zz', ['noSequence']],
    ['Guvenli!Sifre-42', []],
    ['Guvenli Sifre42', []],
    ['ŞİFRE!çok-güçlü9', []],
    ['A'.repeat(0) + 'Xy!1'.repeat(33), ['minLength']],
  ])('%s', (pw, want) => {
    expect(unmet(pw)).toEqual(want);
  });

  it('counts four identical characters as a repeat, three as fine', () => {
    expect(unmet('Guvenli!Sifre-4222')).toEqual([]);
    expect(unmet('Guvenli!Sifre-42222')).toEqual(['noRepeat']);
  });
});

describe('violationKey', () => {
  it('maps every R146 code and nothing else', () => {
    expect(violationKey('password_too_short')).toBe('passwordTooShort');
    expect(violationKey('password_reused')).toBe('passwordReused');
    expect(violationKey('password_personal')).toBe('passwordPersonal');
    expect(violationKey('nope')).toBeNull();
  });
});
