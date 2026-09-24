'use client';

import { Download, Trash2 } from 'lucide-react';
import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { FileUpload } from '@/components/ui/file-upload';
import { IconButton } from '@/components/ui/icon-button';
import type { Locale } from '@/i18n/locale';
import type { ISOFile } from '@/lib/api/types';
import { formatBytes, formatDateTime } from '@/lib/format';

/** R336's allow-list as the picker's filter; the server sniffs the bytes regardless. */
export const ACCEPT = '.pdf,.xlsx,.docx,.xls,.csv,.png,.jpg,.jpeg';
export const MAX_BYTES = 30 * 1024 * 1024;

export type FilesViewProps = {
  files: ISOFile[];
  editable: boolean;
  uploading: boolean;
  /** The server's refusal, already translated. */
  error?: string;
  onUpload: (f: File) => void;
  onDownload: (f: ISOFile) => void;
  onDelete: (f: ISOFile) => void;
};

/** R346: a sub-clause's evidence, uploaded, listed, downloaded and deleted with confirmation. */
export function FilesView({ files, editable, uploading, error, onUpload, onDownload, onDelete }: FilesViewProps) {
  const t = useTranslations('iso');
  const locale = useLocale() as Locale;
  const [deleting, setDeleting] = useState<ISOFile | null>(null);
  return (
    <section className="flex flex-col gap-3">
      <h4 className="type-body font-semibold">{t('files.title')}</h4>
      {error ? <Alert tone="danger" title={error} /> : null}
      {editable ? (
        <FileUpload
          label={t('files.upload')}
          description={t('files.hint')}
          accept={ACCEPT}
          maxSizeBytes={MAX_BYTES}
          disabled={uploading}
          onFilesSelected={(list) => list[0] && onUpload(list[0])}
        />
      ) : null}
      {files.length === 0 ? (
        <p className="text-foreground-muted type-small">{t('files.none')}</p>
      ) : (
        <ul aria-label={t('files.label')} className="flex flex-col gap-2">
          {files.map((f) => (
            <li key={f.id} className="flex items-center justify-between gap-2 rounded-md border border-border p-2">
              <span className="flex min-w-0 flex-col">
                <span className="truncate type-small">{f.name}</span>
                <span className="text-foreground-muted type-caption">{`${formatBytes(f.size_bytes)} · ${formatDateTime(f.created_at, locale)}`}</span>
              </span>
              <span className="flex shrink-0 gap-1">
                <IconButton label={t('files.download', { name: f.name })} icon={Download} onClick={() => onDownload(f)} />
                {editable ? <IconButton label={t('files.delete', { name: f.name })} icon={Trash2} onClick={() => setDeleting(f)} /> : null}
              </span>
            </li>
          ))}
        </ul>
      )}
      <Dialog
        open={deleting !== null}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={t('files.deleteTitle')}
        footer={
          <Button variant="danger" onClick={() => { if (deleting) onDelete(deleting); setDeleting(null); }}>
            {t('notes.delete')}
          </Button>
        }
      >
        {deleting ? <p>{t('files.deleteConfirm', { name: deleting.name })}</p> : null}
      </Dialog>
    </section>
  );
}
