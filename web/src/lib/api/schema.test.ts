import { describe, expectTypeOf, it } from 'vitest';

import type { MeResponse } from './errors';
import type { paths } from './schema';

describe('generated schema', () => {
  it('describes the auth endpoints the web relies on', () => {
    expectTypeOf<paths['/api/v1/auth/me']['get']>().not.toBeNever();
    expectTypeOf<paths['/api/v1/auth/refresh']['post']>().not.toBeNever();
    expectTypeOf<MeResponse['permissions']>().toBeArray();
  });
});
