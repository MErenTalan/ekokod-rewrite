'use client';

import { useTranslations } from 'next-intl';
import type { FormEvent } from 'react';

import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { FormErrorSummary } from '@/components/ui/form-error-summary';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';

import { isRequired, LEAD_FIELDS, type LeadField, type LeadKind, type LeadValues } from './lead-validation';

export type LeadStatus = 'idle' | 'sent' | 'failed' | 'notConfigured' | 'tooMany';
export type LeadFormProps = {
  kind: LeadKind;
  errors: Partial<Record<LeadField, string>>;
  status: LeadStatus;
  sending: boolean;
  defaultSubject?: string;
  onSubmit: (values: LeadValues, website: string) => void;
  onReset: () => void;
};

const TYPES: Partial<Record<LeadField, { type: string; autoComplete: string }>> = {
  name: { type: 'text', autoComplete: 'name' },
  email: { type: 'email', autoComplete: 'email' },
  phone: { type: 'tel', autoComplete: 'tel' },
  company: { type: 'text', autoComplete: 'organization' },
  role: { type: 'text', autoComplete: 'organization-title' },
  subject: { type: 'text', autoComplete: 'off' },
};

/** Contact and demo-request form (01 §7.19, R353): labelled fields, an error summary, a honeypot. */
export function LeadForm({ kind, errors, status, sending, defaultSubject, onSubmit, onReset }: LeadFormProps) {
  const t = useTranslations('site.forms');
  const email = useTranslations('site.contactInfo')('emailValue');
  if (status === 'sent') {
    return (
      <Alert tone="success" title={t('sent')} action={<Button variant="secondary" onClick={onReset}>{t('sendAnother')}</Button>} />
    );
  }
  const fields = LEAD_FIELDS[kind];
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const values = Object.fromEntries(['name', 'email', 'phone', 'subject', 'company', 'role', 'message'].map((f) => [f, String(data.get(f) ?? '')])) as LeadValues;
    onSubmit(values, String(data.get('website') ?? ''));
  };
  const summary = fields.filter((f) => errors[f]).map((f) => ({ fieldId: `lead-${f}`, message: `${t(f)}: ${errors[f]}` }));
  const refusal = status === 'failed' ? t('failed', { email }) : status === 'notConfigured' ? t('notConfigured', { email }) : status === 'tooMany' ? t('tooMany') : null;
  return (
    <form noValidate onSubmit={submit} aria-label={kind === 'demo' ? t('demoTitle') : t('contactTitle')} className="flex flex-col gap-4">
      <FormErrorSummary title={t('errorSummary')} errors={summary} />
      <div className="grid gap-4 sm:grid-cols-2">
        {fields.map((f) =>
          f === 'message' ? (
            <div key={f} className="sm:col-span-2">
              <Textarea id={`lead-${f}`} name={f} label={t(f)} required={isRequired(kind, f)} error={errors[f]} rows={6} maxLength={kind === 'demo' ? 2000 : 5000} />
            </div>
          ) : (
            <Input key={f} id={`lead-${f}`} name={f} label={t(f)} required={isRequired(kind, f)} error={errors[f]}
              defaultValue={f === 'subject' ? defaultSubject : undefined} {...TYPES[f]} />
          ),
        )}
      </div>
      {/* Honeypot (R353): invisible and unreachable for people; bots fill it and get a silent 202. */}
      <div aria-hidden="true" className="absolute -start-[9999px] size-px overflow-hidden">
        <input type="text" name="website" tabIndex={-1} autoComplete="off" defaultValue="" />
      </div>
      {refusal ? <Alert tone={status === 'tooMany' ? 'warning' : 'danger'} title={refusal} /> : null}
      <Button type="submit" size="lg" loading={sending} className="self-start">
        {sending ? t('sending') : t('send')}
      </Button>
    </form>
  );
}
