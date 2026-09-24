'use client';

import { useTranslations } from 'next-intl';
import { useRef, useState } from 'react';

import { errorCodeOf, fieldErrors } from '@/lib/api/problem';
import { $api } from '@/lib/api/query';
import type { Translator } from '@/lib/api/types';

import { LeadForm, type LeadStatus } from './lead-form';
import { validateLead, type LeadField, type LeadKind, type LeadValues } from './lead-validation';

const REFUSALS: Record<string, LeadStatus> = { forms_not_configured: 'notConfigured', rate_limited: 'tooMany' };

/** Sends the lead to F12a's public routes; `elapsed_ms` counts from the form's first render (R353). */
export function LeadFormPanel({ kind, defaultSubject }: { kind: LeadKind; defaultSubject?: string }) {
  const forms = useTranslations('forms') as unknown as Translator;
  const site = useTranslations('site.forms');
  const opened = useRef(Date.now());
  const [status, setStatus] = useState<LeadStatus>('idle');
  const [errors, setErrors] = useState<Partial<Record<LeadField, string>>>({});
  const contact = $api.useMutation('post', '/api/v1/public/contact');
  const demo = $api.useMutation('post', '/api/v1/public/demo-request');
  const mutation = kind === 'demo' ? demo : contact;

  const onSubmit = (v: LeadValues, website: string) => {
    const codes = validateLead(kind, v);
    const local = Object.fromEntries(Object.entries(codes).map(([f, c]) => [f, c === 'tooShort' ? site('messageShort') : forms(`errors.${c}`)]));
    setErrors(local);
    if (Object.keys(local).length) return;
    setStatus('idle');
    const common = { name: v.name.trim(), email: v.email.trim(), website, elapsed_ms: Date.now() - opened.current };
    const handlers = {
      onSuccess: () => setStatus('sent'),
      onError: (error: unknown) => {
        const fields = fieldErrors(error, forms);
        setErrors(fields);
        if (Object.keys(fields).length === 0) setStatus(REFUSALS[errorCodeOf(error) ?? ''] ?? 'failed');
      },
    };
    if (kind === 'demo')
      demo.mutate({ body: { ...common, phone: v.phone.trim(), company: v.company.trim(), role: v.role.trim() || undefined, message: v.message.trim() || undefined } }, handlers);
    else
      contact.mutate({ body: { ...common, phone: v.phone.trim() || undefined, subject: v.subject.trim() || undefined, message: v.message.trim() } }, handlers);
  };

  return (
    <LeadForm
      kind={kind}
      errors={errors}
      status={status}
      sending={mutation.isPending}
      defaultSubject={defaultSubject}
      onSubmit={onSubmit}
      onReset={() => {
        opened.current = Date.now();
        setErrors({});
        setStatus('idle');
      }}
    />
  );
}
