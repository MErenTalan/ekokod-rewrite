'use client';

import { AlertCircle, CheckCircle2, FileText, Loader2, X } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { formatBytes } from '@/lib/format';

import { IconButton } from './icon-button';

export type FileListItem = {
  id: string;
  name: string;
  sizeBytes: number;
  status: 'uploading' | 'done' | 'error';
  progress?: number;
  error?: string;
};

export function FileList({ files, onRemove }: { files: FileListItem[]; onRemove: (id: string) => void }) {
  const t = useTranslations('forms');
  if (files.length === 0) return null;
  return (
    <ul className="flex flex-col divide-y divide-border rounded-md border border-border bg-surface">
      {files.map((file) => (
        <li key={file.id} className="flex items-center gap-3 px-3 py-2">
          <FileText aria-hidden className="size-5 shrink-0 text-foreground-muted" />
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="truncate text-foreground type-body" title={file.name}>
              {file.name}
            </span>
            <span className="flex flex-wrap items-center gap-x-2 text-foreground-muted type-caption">
              <span className="type-data">{formatBytes(file.sizeBytes)}</span>
              {file.status === 'done' ? (
                <span className="inline-flex items-center gap-1 text-success">
                  <CheckCircle2 aria-hidden className="size-3.5" />
                  {t('uploaded')}
                </span>
              ) : null}
              {file.status === 'uploading' ? (
                <span className="inline-flex items-center gap-1">
                  <Loader2 aria-hidden className="size-3.5 animate-spin" />
                  {t('uploading')}
                </span>
              ) : null}
            </span>
            {file.status === 'uploading' ? (
              <div
                role="progressbar"
                aria-label={t('uploadProgress', { name: file.name })}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={file.progress}
                className="mt-1 h-1.5 overflow-hidden rounded-full bg-surface-sunken"
              >
                <div className="h-full rounded-full bg-primary" style={{ width: `${file.progress ?? 0}%` }} />
              </div>
            ) : null}
            {file.status === 'error' ? (
              <span className="flex items-start gap-1 text-danger type-small">
                <AlertCircle aria-hidden className="mt-0.5 size-3.5 shrink-0" />
                {file.error ?? t('uploadFailed')}
              </span>
            ) : null}
          </div>
          <IconButton label={t('removeFile', { name: file.name })} icon={X} size="sm" onClick={() => onRemove(file.id)} />
        </li>
      ))}
    </ul>
  );
}
