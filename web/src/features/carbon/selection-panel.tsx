'use client';

import { useTranslations } from 'next-intl';

import { Skeleton } from '@/components/ui/skeleton';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { SelectionView } from './selection-view';

/** R323's data: the catalogue and the building's declaration. */
export function SelectionPanel({ buildingId }: { buildingId: string }) {
  const t = useTranslations('carbon');
  const { can } = useSession();
  const scope = useScopeParams();
  const catalogue = $api.useQuery('get', '/api/v1/carbon/activity-catalogue', { params: { query: scope } });
  const selected = $api.useQuery('get', '/api/v1/carbon/selected-activities', { params: { query: { ...scope, building_id: buildingId } } });
  const save = useApiMutation('put', '/api/v1/carbon/selected-activities', {
    success: t('selection.saved'),
    invalidate: ['/api/v1/carbon/selected-activities', '/api/v1/carbon/overview'],
  });
  if (!catalogue.data || !selected.data) return <Skeleton className="h-64 w-full" />;
  return (
    <SelectionView
      key={`${buildingId}:${selected.data.activity_keys.join(',')}`}
      catalogue={catalogue.data}
      selected={selected.data.activity_keys}
      editable={can('carbon.edit')}
      saving={save.isPending}
      onSave={(keys) => save.mutate({ params: { query: { ...scope, building_id: buildingId } }, body: { activity_keys: keys } })}
    />
  );
}
