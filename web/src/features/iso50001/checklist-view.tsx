'use client';

import { FileSpreadsheet } from 'lucide-react';
import { useTranslations } from 'next-intl';
import type { ReactNode } from 'react';

import { Accordion } from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import type { ISOClauses, ISOSubClause, ISOTemplate } from '@/lib/api/types';

export type ChecklistViewProps = {
  clauses: ISOClauses;
  templates: ISOTemplate[];
  onTemplate: (t: ISOTemplate) => void;
  /** The sub-clause's notes and files; mounted only while its clause is open. */
  renderSub: (sub: ISOSubClause) => ReactNode;
};

/** R345: clauses 5–9 as an accordion; every sub-clause with its text, template, notes and files. */
export function ChecklistView({ clauses, templates, onTemplate, renderSub }: ChecklistViewProps) {
  const t = useTranslations('iso');
  const byId = new Map(templates.map((tpl) => [tpl.id, tpl]));
  return (
    <Accordion
      type="multiple"
      items={clauses.items.map((main) => ({
        value: main.id,
        title: main.title,
        content: (
          <div className="flex flex-col gap-6 pb-4">
            {main.subs.map((sub) => {
              const tpl = sub.template ? byId.get(sub.template) : undefined;
              return (
                <article key={sub.id} className="flex flex-col gap-3 rounded-lg border border-border bg-surface-raised p-4">
                  <h4 className="type-h3">{sub.title}</h4>
                  <p className="max-w-prose text-foreground type-body">{sub.description}</p>
                  {tpl ? (
                    <div className="flex flex-col gap-1 rounded-md bg-surface-sunken p-3">
                      <p className="type-small font-semibold">{t('checklist.template')}</p>
                      <p className="text-foreground-muted type-small">{tpl.description}</p>
                      <Button variant="secondary" size="sm" className="self-start" onClick={() => onTemplate(tpl)}
                        aria-label={t('checklist.downloadTemplate', { name: tpl.file_name })}>
                        <FileSpreadsheet aria-hidden className="size-4" />
                        {tpl.file_name}
                      </Button>
                    </div>
                  ) : null}
                  {renderSub(sub)}
                </article>
              );
            })}
          </div>
        ),
      }))}
    />
  );
}
