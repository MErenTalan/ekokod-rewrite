// Client mirror of internal/auth/policy.go (R146): the checks that need no server context.
// Personal fragments and history are server-only; the server stays the authority.
export const PASSWORD_MIN = 10;
export const PASSWORD_MAX = 128;
const SPECIAL = ' !"#$%&\'()*+,-./:;<=>?@[\\]^_`{|}~';
const COMMON = ['1234', 'abcd', 'qwer', 'asdf', 'zxcv', 'password', 'passw0rd', '1111', '0000'];

export type PasswordRule = 'minLength' | 'lower' | 'upper' | 'digit' | 'special' | 'noRepeat' | 'noSequence';

export function passwordChecks(password: string): { rule: PasswordRule; met: boolean }[] {
  const chars = [...password];
  let run = 0;
  let longest = 0;
  chars.forEach((c, i) => {
    run = i > 0 && c === chars[i - 1] ? run + 1 : 1;
    longest = Math.max(longest, run);
  });
  const lower = password.toLocaleLowerCase('tr');
  return [
    { rule: 'minLength', met: chars.length >= PASSWORD_MIN && chars.length <= PASSWORD_MAX },
    { rule: 'lower', met: /\p{Ll}/u.test(password) },
    { rule: 'upper', met: /\p{Lu}/u.test(password) },
    { rule: 'digit', met: /[0-9]/.test(password) },
    { rule: 'special', met: chars.some((c) => SPECIAL.includes(c)) },
    { rule: 'noRepeat', met: longest < 4 },
    { rule: 'noSequence', met: !COMMON.some((s) => lower.includes(s)) },
  ];
}

const VIOLATIONS = {
  password_too_short: 'passwordTooShort',
  password_too_long: 'passwordTooLong',
  password_complexity: 'passwordComplexity',
  password_repeat: 'passwordRepeat',
  password_common: 'passwordCommon',
  password_personal: 'passwordPersonal',
  password_reused: 'passwordReused',
} as const;
export type ViolationKey = (typeof VIOLATIONS)[keyof typeof VIOLATIONS];

/** R146 violation code → auth.violations message key. */
export function violationKey(code: string): ViolationKey | null {
  return Object.hasOwn(VIOLATIONS, code) ? VIOLATIONS[code as keyof typeof VIOLATIONS] : null;
}
