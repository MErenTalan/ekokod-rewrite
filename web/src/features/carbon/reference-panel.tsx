'use client';

import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';

import { ReferenceView, type ReferenceKind } from './reference-view';

/** R327: the reference pages read the live catalogue for their mapping table. */
export function ReferencePanel({ kind }: { kind: ReferenceKind }) {
  const scope = useScopeParams();
  const catalogue = $api.useQuery('get', '/api/v1/carbon/activity-catalogue', { params: { query: scope } }, { enabled: kind !== 'standards' });
  return <ReferenceView kind={kind} catalogue={catalogue.data} />;
}
