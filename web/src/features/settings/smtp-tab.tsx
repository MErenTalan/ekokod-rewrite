'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { NumberInput } from '@/components/ui/number-input';
import { Switch } from '@/components/ui/switch';
import type { SMTPPutRequest, SMTPSettings } from '@/lib/api/types';

export type SmtpTabViewProps = {
  settings: SMTPSettings | null;
  onSave: (values: SMTPPutRequest) => void;
  onTest: (to: string) => void;
  fieldErrors?: Record<string, string>;
  saving?: boolean;
  testing?: boolean;
};

/** Per-company outbound mail (01 §7.15, R172): the password is write-only. */
export function SmtpTabView({ settings, onSave, onTest, fieldErrors = {}, saving = false, testing = false }: SmtpTabViewProps) {
  const t = useTranslations('settings.smtp');
  const common = useTranslations('common');
  const [values, setValues] = useState({
    host: settings?.host ?? '',
    port: String(settings?.port ?? 587),
    secure: settings?.secure ?? false,
    username: settings?.username ?? '',
    password: '',
    from_address: settings?.from_address ?? '',
  });
  const [testOpen, setTestOpen] = useState(false);
  const [testTo, setTestTo] = useState('');

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-foreground-muted type-small">{t('emptyHint')}</p>
      </CardHeader>
      <CardContent>
        <form
          className="flex max-w-xl flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            onSave({
              host: values.host,
              port: Number(values.port),
              secure: values.secure,
              username: values.username,
              from_address: values.from_address,
              // An empty password keeps the stored one (R172).
              ...(values.password ? { password: values.password } : {}),
            });
          }}
        >
          <Input label={t('host')} value={values.host} error={fieldErrors.host} onChange={(e) => setValues({ ...values, host: e.target.value })} required />
          <NumberInput label={t('port')} value={values.port} error={fieldErrors.port} onValueChange={(v) => setValues({ ...values, port: v ?? '' })} min="1" max="65535" fractionDigits={0} />
          <Switch label={t('secure')} checked={values.secure} onCheckedChange={(secure) => setValues({ ...values, secure })} />
          <Input label={t('username')} value={values.username} error={fieldErrors.username} onChange={(e) => setValues({ ...values, username: e.target.value })} />
          <Input
            label={t('password')}
            type="password"
            autoComplete="new-password"
            value={values.password}
            description={settings?.has_password ? t('passwordKept') : undefined}
            error={fieldErrors.password}
            onChange={(e) => setValues({ ...values, password: e.target.value })}
          />
          <Input label={t('fromAddress')} type="email" value={values.from_address} error={fieldErrors.from_address} onChange={(e) => setValues({ ...values, from_address: e.target.value })} required />
          <div className="flex flex-wrap gap-2">
            <Button type="submit" loading={saving}>
              {common('save')}
            </Button>
            <Button type="button" variant="secondary" disabled={!settings} onClick={() => setTestOpen(true)}>
              {t('test')}
            </Button>
          </div>
        </form>
      </CardContent>

      <Dialog
        open={testOpen}
        onOpenChange={setTestOpen}
        title={t('test')}
        footer={
          <Button
            loading={testing}
            onClick={() => {
              onTest(testTo);
              setTestOpen(false);
            }}
          >
            {t('test')}
          </Button>
        }
      >
        <Input label={t('testTo')} type="email" value={testTo} onChange={(e) => setTestTo(e.target.value)} required />
      </Dialog>
    </Card>
  );
}
