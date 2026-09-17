import type { MeResponse } from '@/lib/api/errors';

export type Permission = MeResponse['permissions'][number];
export type Role = NonNullable<MeResponse['role']>;
