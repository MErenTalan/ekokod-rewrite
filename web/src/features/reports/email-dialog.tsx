'use client';

import { useTranslations } from 'next-intl';
import { useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Dialog } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';

export type EmailDialogProps = {
  open: boolean;
  summary: { period: string; building: string };
  sending: boolean;
  /** A server refusal of the address, shown on the field. */
  error?: string;
  onSend: (to: string) => void;
  onClose: () => void;
};

/** §7.14's send-by-e-mail: the recipient, with the period and building shown before sending. */
export function EmailDialog({ open, summary, sending, error, onSend, onClose }: EmailDialogProps) {
  const t = useTranslations('reports.email');
  const [to, setTo] = useState('');
  const [invalid, setInvalid] = useState(false);
  useEffect(() => {
    if (!open) {
      setTo('');
      setInvalid(false);
    }
  }, [open]);

  const submit = () => {
    // The server validates the address properly (R266); this only stops an obvious slip.
    if (!/^[^@\s]+@[^@\s]+$/.test(to.trim())) {
      setInvalid(true);
      return;
    }
    onSend(to.trim());
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => (o ? undefined : onClose())}
      title={t('title')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>{t('cancel')}</Button>
          <Button onClick={submit} loading={sending}>{sending ? t('sending') : t('send')}</Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Input
          label={t('address')}
          type="email"
          autoComplete="email"
          value={to}
          onChange={(e) => {
            setTo(e.target.value);
            setInvalid(false);
          }}
          error={invalid ? t('invalid') : error}
          required
        />
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 type-small">
          <dt className="text-foreground-muted">{t('summaryPeriod')}</dt>
          <dd>{summary.period}</dd>
          <dt className="text-foreground-muted">{t('summaryBuilding')}</dt>
          <dd>{summary.building}</dd>
        </dl>
      </form>
    </Dialog>
  );
}
