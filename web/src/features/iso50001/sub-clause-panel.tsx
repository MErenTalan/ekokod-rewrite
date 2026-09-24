'use client';

import { useTranslations } from 'next-intl';

import { downloadFile } from '@/lib/api/download';
import { useApiMutation } from '@/lib/api/mutation';
import { $api } from '@/lib/api/query';
import { useScopeParams } from '@/lib/selection/selection-store';
import { useSession } from '@/lib/session/session-provider';

import { FilesView } from './files-view';
import { NotesView } from './notes-view';

const INVALIDATE = ['/api/v1/iso50001/{building_id}', '/api/v1/iso50001/{building_id}/clauses/{clause}/notes', '/api/v1/iso50001/{building_id}/clauses/{clause}/files'];

/** R346's data for one opened sub-clause. */
export function SubClausePanel({ buildingId, clause }: { buildingId: string; clause: string }) {
  const t = useTranslations('iso');
  const { can } = useSession();
  const scope = useScopeParams();
  const path = { params: { path: { building_id: buildingId, clause }, query: scope } };
  const notes = $api.useQuery('get', '/api/v1/iso50001/{building_id}/clauses/{clause}/notes', path);
  const files = $api.useQuery('get', '/api/v1/iso50001/{building_id}/clauses/{clause}/files', path);
  const add = useApiMutation('post', '/api/v1/iso50001/{building_id}/clauses/{clause}/notes', { success: t('notes.saved'), invalidate: INVALIDATE });
  const update = useApiMutation('patch', '/api/v1/iso50001/notes/{id}', { success: t('notes.saved'), invalidate: INVALIDATE });
  const removeNote = useApiMutation('delete', '/api/v1/iso50001/notes/{id}', { success: t('notes.deleted'), invalidate: INVALIDATE });
  const upload = useApiMutation('post', '/api/v1/iso50001/{building_id}/clauses/{clause}/files', { success: t('files.uploaded'), invalidate: INVALIDATE });
  const removeFile = useApiMutation('delete', '/api/v1/iso50001/files/{id}', { success: t('files.deleted'), invalidate: INVALIDATE });
  const editable = can('iso50001.edit');
  const orNull = (s: string) => (s ? s : undefined);

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <NotesView
        notes={notes.data?.items ?? []}
        editable={editable}
        saving={add.isPending || update.isPending}
        onAdd={(d) => add.mutate({ ...path, body: { title: orNull(d.title), body: d.body } })}
        onUpdate={(n, d) => update.mutate({ params: { path: { id: n.id }, query: scope }, body: { clause_id: clause, title: orNull(d.title), body: d.body } })}
        onDelete={(n) => removeNote.mutate({ params: { path: { id: n.id }, query: scope } })}
      />
      <FilesView
        files={files.data?.items ?? []}
        editable={editable}
        uploading={upload.isPending}
        error={upload.fieldErrors.file}
        onUpload={(f) => {
          const body = new FormData();
          body.append('file', f);
          upload.mutate({ ...path, body: body as never, bodySerializer: (b: unknown) => b } as never);
        }}
        onDownload={(f) => void downloadFile(`/api/v1/iso50001/files/${f.id}`, scope, f.name)}
        onDelete={(f) => removeFile.mutate({ params: { path: { id: f.id }, query: scope } })}
      />
    </div>
  );
}
