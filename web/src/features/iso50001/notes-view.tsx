'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import type { Locale } from '@/i18n/locale';
import type { ISONote } from '@/lib/api/types';
import { formatDateTime } from '@/lib/format';

export type NoteDraft = { title: string; body: string };

export type NotesViewProps = {
  notes: ISONote[];
  editable: boolean;
  saving: boolean;
  onAdd: (d: NoteDraft) => void;
  onUpdate: (n: ISONote, d: NoteDraft) => void;
  onDelete: (n: ISONote) => void;
};

/** R346: a sub-clause's titled notes with add, edit in place, cancel and a confirmed delete. */
export function NotesView({ notes, editable, saving, onAdd, onUpdate, onDelete }: NotesViewProps) {
  const t = useTranslations('iso');
  const locale = useLocale() as Locale;
  const [draft, setDraft] = useState<NoteDraft>({ title: '', body: '' });
  const [editing, setEditing] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<ISONote | null>(null);
  const editingNote = notes.find((n) => n.id === editing);
  const submit = () => {
    const d = { title: draft.title.trim(), body: draft.body.trim() };
    if (editingNote) onUpdate(editingNote, d);
    else onAdd(d);
    setDraft({ title: '', body: '' });
    setEditing(null);
  };
  return (
    <section className="flex flex-col gap-3">
      <h4 className="type-body font-semibold">{t('notes.title')}</h4>
      {editable ? (
        <div className="flex flex-col gap-2">
          <Input label={t('notes.noteTitle')} value={draft.title} maxLength={200} onChange={(e) => setDraft({ ...draft, title: e.target.value })} />
          <Textarea label={t('notes.body')} value={draft.body} maxLength={10000} rows={3} onChange={(e) => setDraft({ ...draft, body: e.target.value })} />
          <div className="flex gap-2">
            <Button size="sm" loading={saving} disabled={!draft.body.trim()} onClick={submit}>
              {editingNote ? t('notes.update') : t('notes.add')}
            </Button>
            {editingNote ? (
              <Button size="sm" variant="secondary" onClick={() => { setEditing(null); setDraft({ title: '', body: '' }); }}>
                {t('notes.cancel')}
              </Button>
            ) : null}
          </div>
        </div>
      ) : null}
      <h5 className="text-foreground-muted type-small">{t('notes.existing')}</h5>
      {notes.length === 0 ? (
        <p className="text-foreground-muted type-small">{t('notes.none')}</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {notes.map((n) => (
            <li key={n.id} className="flex flex-col gap-1 rounded-md border border-border p-3">
              <p className="type-small font-semibold">{n.title ?? t('notes.untitled')}</p>
              <p className="whitespace-pre-wrap text-foreground type-small">{n.body}</p>
              <p className="text-foreground-muted type-caption">{formatDateTime(n.updated_at, locale)}</p>
              {editable ? (
                <div className="flex gap-2">
                  <Button size="sm" variant="ghost" onClick={() => { setEditing(n.id); setDraft({ title: n.title ?? '', body: n.body }); }}>
                    {t('notes.edit')}
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setDeleting(n)}>
                    {t('notes.delete')}
                  </Button>
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      <Dialog
        open={deleting !== null}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={t('notes.deleteTitle')}
        footer={
          <Button variant="danger" onClick={() => { if (deleting) onDelete(deleting); setDeleting(null); }}>
            {t('notes.delete')}
          </Button>
        }
      >
        {deleting ? <p>{t('notes.deleteConfirm', { title: deleting.title ?? t('notes.untitled') })}</p> : null}
      </Dialog>
    </section>
  );
}
