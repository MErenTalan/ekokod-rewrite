'use client';

import { UploadCloud } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useRef, useState } from 'react';

import { cn } from '@/lib/cn';
import { formatBytes } from '@/lib/format';

import { Button } from './button';
import { Field, type FieldProps } from './field';

export type FileUploadProps = FieldProps & {
  accept: string;
  maxSizeBytes: number;
  multiple?: boolean;
  onFilesSelected: (files: File[]) => void;
};

/** `accept` tokens: `.ext`, `type/subtype` or `type/*`. */
export function matchesAccept(file: File, accept: string): boolean {
  const tokens = accept.split(',').map((t) => t.trim().toLowerCase()).filter(Boolean);
  if (tokens.length === 0) return true;
  const name = file.name.toLowerCase();
  const type = file.type.toLowerCase();
  return tokens.some((token) =>
    token.startsWith('.') ? name.endsWith(token) : token.endsWith('/*') ? type.startsWith(token.slice(0, -1)) : type === token,
  );
}

export function FileUpload({ label, description, error, required, id, disabled, accept, maxSizeBytes, multiple = false, onFilesSelected }: FileUploadProps) {
  const t = useTranslations('forms');
  const input = useRef<HTMLInputElement>(null);
  const [problems, setProblems] = useState<string[]>([]);
  const [dragging, setDragging] = useState(false);
  const max = formatBytes(maxSizeBytes);

  const handle = (list: FileList | null) => {
    const files = [...(list ?? [])].slice(0, multiple ? undefined : 1);
    const found: string[] = [];
    const valid = files.filter((file) => {
      if (!matchesAccept(file, accept)) found.push(t('fileTypeNotAllowed', { name: file.name }));
      else if (file.size > maxSizeBytes) found.push(t('fileTooLarge', { name: file.name, max }));
      else return true;
      return false;
    });
    setProblems(found);
    if (valid.length > 0) onFilesSelected(valid);
  };

  return (
    <Field label={label} description={description} error={error ?? (problems.join(' · ') || undefined)} required={required} id={id} disabled={disabled}>
      {({ controlId, describedBy, invalid }) => (
        <div
          onDragOver={(e) => {
            e.preventDefault();
            if (!disabled) setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragging(false);
            if (!disabled) handle(e.dataTransfer.files);
          }}
          className={cn(
            'flex flex-col items-center gap-2 rounded-md border border-dashed border-border-control bg-surface-sunken p-4 text-center transition-colors duration-(--duration-hover)',
            dragging && 'border-primary',
            invalid && 'border-danger',
          )}
        >
          <UploadCloud aria-hidden className="size-6 text-foreground-muted" />
          <p className="text-foreground-muted type-small">{t('dropFiles')}</p>
          <Button variant="secondary" size="sm" disabled={disabled} aria-describedby={`${controlId}-hint`} onClick={() => input.current?.click()}>
            {t('browse')}
          </Button>
          <p id={`${controlId}-hint`} className="text-foreground-muted type-caption">
            {t('acceptedTypes', { types: accept.replaceAll(',', ', '), max })}
          </p>
          <input
            ref={input}
            id={controlId}
            type="file"
            accept={accept}
            multiple={multiple}
            disabled={disabled}
            tabIndex={-1}
            aria-describedby={describedBy}
            aria-invalid={invalid || undefined}
            className="sr-only"
            onChange={(e) => {
              handle(e.target.files);
              e.target.value = '';
            }}
          />
        </div>
      )}
    </Field>
  );
}
