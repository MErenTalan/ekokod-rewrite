'use client';

import { useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { User } from '@/lib/api/types';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import {
  UserFormView,
  emptyUser,
  isBuildingRole,
  toUserRequest,
  type UserDraft,
} from './user-form';
import { UsersTabView } from './users-tab';

/** The Users tab's data (01 §7.15, R174). */
export function UsersPanel() {
  const t = useTranslations('settings.users');
  const settings = useTranslations('settings');
  const common = useTranslations('common');
  const { me, can } = useSession();
  const scope = useScopeParams();
  const [draft, setDraft] = useState<UserDraft | null>(null);
  const [pendingDelete, setPendingDelete] = useState<User | null>(null);
  const canEdit = can('write') && can('settings.users');

  const users = $api.useQuery('get', '/api/v1/users', {
    params: { query: { ...scope, limit: 500 } },
  });
  const isAdmin = me.role === 'admin';
  const companies = $api.useQuery(
    'get',
    '/api/v1/companies',
    { params: { query: { limit: 500 } } },
    { enabled: isAdmin },
  );
  const companyNames = useMemo(
    () => Object.fromEntries((companies.data?.items ?? []).map((c) => [c.id, c.name])),
    [companies.data],
  );
  // A new user's buildings come from the company chosen in the form, not the header's.
  const formCompany =
    draft?.companyId && draft.companyId !== scope.company_id
      ? { company_id: draft.companyId }
      : scope;
  const buildings = $api.useQuery(
    'get',
    '/api/v1/buildings',
    { params: { query: { ...formCompany, limit: 500 } } },
    { enabled: draft !== null },
  );
  const create = useApiMutation('post', '/api/v1/users', {
    success: t('saved'),
    invalidate: ['/api/v1/users'],
  });
  const update = useApiMutation('patch', '/api/v1/users/{id}', {
    success: t('saved'),
    invalidate: ['/api/v1/users'],
  });
  const remove = useApiMutation('delete', '/api/v1/users/{id}', {
    success: t('deleted'),
    invalidate: ['/api/v1/users'],
  });

  return (
    <>
      <UsersTabView
        users={users.data?.items ?? []}
        selfId={me.id}
        canEdit={canEdit}
        loading={users.isLoading}
        onAdd={() => setDraft(emptyUser(me.role, scope.company_id ?? me.company.id))}
        onEdit={(user) =>
          setDraft({
            id: user.id,
            name: user.name,
            email: user.email,
            phone: user.phone ?? '',
            role: user.role,
            password: '',
            isActive: user.is_active,
            companyId: user.company_id,
            buildingIds: user.building_ids ?? [],
          })
        }
        onDelete={setPendingDelete}
        companyNames={isAdmin ? companyNames : undefined}
      />

      <Dialog
        open={draft !== null}
        onOpenChange={(open) => !open && setDraft(null)}
        title={draft?.id ? t('edit') : t('add')}
        footer={
          <Button
            loading={create.isPending || update.isPending}
            onClick={() => {
              if (!draft) return;
              const body = toUserRequest(draft);
              if (draft.id) {
                // A password is only ever set at creation; an update never carries one.
                const patch = {
                  name: body.name,
                  email: body.email,
                  phone: body.phone,
                  role: body.role,
                  is_active: body.is_active,
                  ...(isBuildingRole(draft.role) ? { building_ids: draft.buildingIds } : {}),
                };
                update.mutate({ params: { path: { id: draft.id }, query: scope }, body: patch });
              } else {
                create.mutate({
                  params: { query: draft.companyId ? { company_id: draft.companyId } : scope },
                  body,
                });
              }
              setDraft(null);
            }}
          >
            {common('save')}
          </Button>
        }
      >
        {draft ? (
          <UserFormView
            value={draft}
            onChange={setDraft}
            actorRole={me.role}
            isSelf={draft.id === me.id}
            fieldErrors={{ ...create.fieldErrors, ...update.fieldErrors }}
            companies={
              isAdmin
                ? (companies.data?.items ?? []).map((c) => ({ id: c.id, name: c.name }))
                : undefined
            }
            buildings={(buildings.data?.items ?? []).map((b) => ({ id: b.id, name: b.name }))}
          />
        ) : null}
      </Dialog>

      <Dialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        title={settings('confirmDelete')}
        footer={
          <Button
            variant="danger"
            onClick={() => {
              if (pendingDelete)
                remove.mutate({ params: { path: { id: pendingDelete.id }, query: scope } });
              setPendingDelete(null);
            }}
          >
            {settings('deleteAction')}
          </Button>
        }
      >
        {pendingDelete ? <p>{t('deleteConfirm', { name: pendingDelete.name })}</p> : null}
      </Dialog>
    </>
  );
}
