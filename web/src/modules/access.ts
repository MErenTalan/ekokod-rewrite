import type { MeResponse } from '@/lib/api/errors';

import { MODULE_IDS, type ModuleId } from './ids';

/**
 * The modules a user may open (R491). Frontend-only hook for now (U2): everyone gets every module.
 * A per-user grant later only changes this function (or reads a `modules` field from /auth/me).
 */
// eslint-disable-next-line @typescript-eslint/no-unused-vars -- a per-user grant will read it (U2)
export function modulesFor(me: MeResponse): ModuleId[] {
  return [...MODULE_IDS];
}
