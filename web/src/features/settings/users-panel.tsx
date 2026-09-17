'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import type { User } from '@/lib/api/types';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { UserFormView, emptyUser, toUserRequest, type UserDraft } from './user-form';
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

  const users = $api.useQuery('get', '/api/v1/users', { params: { query: { ...scope, limit: 500 } } });
  const create = useApiMutation('post', '/api/v1/users', { success: t('saved'), invalidate: ['/api/v1/users'] });
  const update = useApiMutation('patch', '/api/v1/users/{id}', { success: t('saved'), invalidate: ['/api/v1/users'] });
  const remove = useApiMutation('delete', '/api/v1/users/{id}', { success: t('deleted'), invalidate: ['/api/v1/users'] });

  return (
    <>
      <UsersTabView
        users={users.data?.items ?? []}
        selfId={me.id}
        canEdit={canEdit}
        loading={users.isLoading}
        onAdd={() => setDraft(emptyUser(me.role))}
        onEdit={(user) =>
          setDraft({
            id: user.id,
            name: user.name,
            email: user.email,
            phone: user.phone ?? '',
            role: user.role,
            password: '',
            isActive: user.is_active,
          })
        }
        onDelete={setPendingDelete}
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
                const patch = { name: body.name, email: body.email, phone: body.phone, role: body.role, is_active: body.is_active };
                update.mutate({ params: { path: { id: draft.id }, query: scope }, body: patch });
              } else {
                create.mutate({ params: { query: scope }, body });
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
              if (pendingDelete) remove.mutate({ params: { path: { id: pendingDelete.id }, query: scope } });
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
