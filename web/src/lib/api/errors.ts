import type { components } from './schema';

export type ApiError = components['schemas']['Error'];
export type MeResponse = components['schemas']['Me'];

export const HOME_PATH = '/ekorm';
export const LOGIN_PATH = '/auth/login';

/** Codes a single-flight refresh may cure (R168); an expired access cookie is simply absent → `unauthorized`. */
export const REFRESHABLE_CODES: ReadonlySet<string> = new Set([
  'token_expired',
  'token_rotated',
  'unauthorized',
]);
const SESSION_ENDED_CODES: ReadonlySet<string> = new Set([
  ...REFRESHABLE_CODES,
  'session_revoked',
  'token_invalid',
]);

export function errorCode(body: unknown): string | undefined {
  const code = (body as { error?: { code?: unknown } } | null)?.error?.code;
  return typeof code === 'string' ? code : undefined;
}

/** Only an in-app path is a valid post-login target (open-redirect guard). */
export function safeNext(next: string | null | undefined): string {
  if (!next || /[\\\s]/.test(next)) return HOME_PATH;
  if (next !== HOME_PATH && !next.startsWith(`${HOME_PATH}/`) && !next.startsWith(`${HOME_PATH}?`))
    return HOME_PATH;
  return next;
}

/** Where an ended session sends the browser, or null when the error is not about the session. */
export function authRedirectFor(code: string, next: string): string | null {
  if (code === 'device_mismatch') return `${LOGIN_PATH}?reason=device_mismatch`;
  if (SESSION_ENDED_CODES.has(code))
    return `${LOGIN_PATH}?next=${encodeURIComponent(safeNext(next))}`;
  return null;
}
