import { describe, expect, it } from 'vitest';

import { messages } from '../../../messages';

import { errorCodeOf, errorMessage, fieldErrors } from './problem';

const t = (key: string) => {
  const path = key.split('.');
  let node: unknown = messages.tr.forms;
  for (const part of path) node = (node as Record<string, unknown>)[part];
  return String(node);
};

describe('problem', () => {
  it('maps validation details to one message per field', () => {
    const body = {
      error: {
        code: 'validation_failed',
        message: 'Doğrulama başarısız',
        details: { name: ['required'], 'contacts[0].phone': ['max'], role: ['not_assignable'] },
      },
    };
    expect(fieldErrors(body, t)).toEqual({
      name: 'Zorunlu alan',
      'contacts[0].phone': 'Çok uzun',
      role: 'Bu rolü atayamazsınız',
    });
  });

  it('falls back to the generic message for a code it does not know', () => {
    expect(fieldErrors({ error: { details: { x: ['brand_new_server_code'] } } }, t)).toEqual({ x: 'Geçersiz değer' });
    expect(fieldErrors({ error: {} }, t)).toEqual({});
    expect(fieldErrors(null, t)).toEqual({});
  });

  it('prefers the API message and exposes the code', () => {
    expect(errorMessage({ error: { message: 'Bina silinemedi' } }, 'yedek')).toBe('Bina silinemedi');
    expect(errorMessage({ error: {} }, 'yedek')).toBe('yedek');
    expect(errorMessage(new Error('network'), 'yedek')).toBe('yedek');
    expect(errorCodeOf({ error: { code: 'building_has_analyzers' } })).toBe('building_has_analyzers');
    expect(errorCodeOf({})).toBeUndefined();
  });
});
