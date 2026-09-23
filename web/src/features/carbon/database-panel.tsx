'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { EmissionFactorView } from '@/lib/api/types';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { FactorDialog } from './factor-dialog';
import { FactorTable } from './factor-table';

const INVALIDATE = ['/api/v1/carbon/emission-factors'];

/** R326's data: the company catalogue, overrides and reset (company-level, no building). */
export function DatabasePanel() {
  const t = useTranslations('carbon');
  const { can } = useSession();
  const scope = useScopeParams();
  const [query, setQuery] = useState('');
  const [main, setMain] = useState('all');
  const [editing, setEditing] = useState<EmissionFactorView | null>(null);
  const [resetting, setResetting] = useState(false);
  // The previous result stays while a new search loads, so the search box keeps focus.
  const list = $api.useQuery(
    'get',
    '/api/v1/carbon/emission-factors',
    { params: { query: { ...scope, q: query.trim() || undefined, main_category: main === 'all' ? undefined : main } } },
    { placeholderData: (previous) => previous },
  );
  const override = useApiMutation('patch', '/api/v1/carbon/emission-factors/{id}', { success: t('database.saved'), invalidate: INVALIDATE });
  const reset = useApiMutation('post', '/api/v1/carbon/emission-factors/reset', { success: t('database.resetDone'), invalidate: INVALIDATE });

  return (
    <>
      {list.data ? (
        <FactorTable
          rows={list.data.items}
          editable={can('carbon.edit')}
          query={query}
          onQuery={setQuery}
          main={main}
          onMain={setMain}
          onOverride={setEditing}
          onReset={() => setResetting(true)}
        />
      ) : (
        <Skeleton className="h-64 w-full" />
      )}
      {editing ? (
        <FactorDialog
          factor={editing}
          saving={override.isPending}
          errors={override.fieldErrors}
          onClose={() => setEditing(null)}
          onSubmit={(body) =>
            override.mutate({ params: { path: { id: editing.id }, query: scope }, body }, { onSuccess: () => setEditing(null) })
          }
        />
      ) : null}
      <Dialog
        open={resetting}
        onOpenChange={setResetting}
        title={t('database.resetTitle')}
        footer={
          <Button
            variant="danger"
            onClick={() => {
              reset.mutate({ params: { query: scope } });
              setResetting(false);
            }}
          >
            {t('database.reset')}
          </Button>
        }
      >
        <p>{t('database.resetConfirm')}</p>
      </Dialog>
    </>
  );
}
