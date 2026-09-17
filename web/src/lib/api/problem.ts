// The error envelope (05 §1) read back into what a form needs (R199).
import type { Translator } from './types';

type Envelope = { error?: { code?: unknown; message?: unknown; details?: unknown } };

function envelope(body: unknown): Envelope['error'] | undefined {
  const error = (body as Envelope | null)?.error;
  return error && typeof error === 'object' ? error : undefined;
}

/** The API's own localised message, or the caller's fallback. */
export function errorMessage(body: unknown, fallback: string): string {
  const message = envelope(body)?.message;
  return typeof message === 'string' && message ? message : fallback;
}

/** The stable machine code, for the few branches that need one. */
export function errorCodeOf(body: unknown): string | undefined {
  const code = envelope(body)?.code;
  return typeof code === 'string' ? code : undefined;
}

/**
 * The validation codes the API emits that have their own message. Anything
 * else falls back to the generic one: next-intl throws on a missing key, so a
 * new server code must never be looked up blindly. Message keys are camelCase
 * (the i18n parity check enforces it), the codes are the API's snake_case.
 */
const CODE_KEYS: Record<string, string> = {
  required: 'required',
  invalid: 'invalid',
  email: 'email',
  min: 'min',
  max: 'max',
  oneof: 'oneof',
  gtefield: 'gtefield',
  url: 'url',
  uuid: 'uuid',
  numeric: 'numeric',
  not_assignable: 'notAssignable',
  too_many_analyzers: 'tooManyAnalyzers',
  range_too_wide: 'rangeTooWide',
  exactly_one_of_analyzer_id_building_id: 'subjectExclusive',
};

/**
 * `details` mapped to one message per field: the API sends validation codes
 * (`["required"]`), which become `forms.errors.<code>`, with the generic
 * message for anything not in KNOWN_CODES.
 */
export function fieldErrors(body: unknown, t: Translator): Record<string, string> {
  const details = envelope(body)?.details;
  if (!details || typeof details !== 'object') return {};
  const out: Record<string, string> = {};
  for (const [field, codes] of Object.entries(details as Record<string, unknown>)) {
    const code = Array.isArray(codes) ? codes[0] : codes;
    if (typeof code !== 'string') continue;
    out[field] = translateCode(t, code);
  }
  return out;
}

function translateCode(t: Translator, code: string): string {
  return t(`errors.${CODE_KEYS[code] ?? 'invalid'}`);
}
