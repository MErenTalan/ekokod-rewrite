import { MODULE_PREFIXES } from '@/modules/ids';

import type { components } from './schema';

export type ApiError = components['schemas']['Error'];
export type MeResponse = components['schemas']['Me'];

export const LOGIN_PATH = '/auth/login';
/** The module chooser (R495), also where a sign-in without a valid `next` lands (U3). */
export const CHOOSER_PATH = '/modules';
const APP_ROOTS: readonly string[] = [CHOOSER_PATH, ...MODULE_PREFIXES];

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
  if (!next || /[\\\s]|\/\.\.?(?:\/|\?|$)/.test(next)) return CHOOSER_PATH;
  // R492: only the chooser or a module root, followed by nothing, `/` or `?`.
  const inApp = APP_ROOTS.some((root) => next === root || next.startsWith(`${root}/`) || next.startsWith(`${root}?`));
  return inApp ? next : CHOOSER_PATH;
}

/** Where an ended session sends the browser, or null when the error is not about the session. */
export function authRedirectFor(code: string, next: string): string | null {
  if (code === 'device_mismatch') return `${LOGIN_PATH}?reason=device_mismatch`;
  if (SESSION_ENDED_CODES.has(code))
    return `${LOGIN_PATH}?next=${encodeURIComponent(safeNext(next))}`;
  return null;
}
