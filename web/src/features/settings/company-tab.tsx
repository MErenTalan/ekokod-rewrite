'use client';

import { useTranslations } from 'next-intl';
import { useState, type ReactNode } from 'react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { NumberInput } from '@/components/ui/number-input';
import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Textarea } from '@/components/ui/textarea';
import type { CompanyDetail, CompanyUpdateRequest } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

export type CompanyTabViewProps = {
  company: CompanyDetail | null;
  canEdit: boolean;
  onSave: (values: CompanyUpdateRequest) => void;
  fieldErrors?: Record<string, string>;
  saving?: boolean;
  /** The admin company list and the credentials section live under this tab. */
  children?: ReactNode;
};

/** The company details of 01 §7.15, with analyzers grouped by integration. */
export function CompanyTabView({ company, canEdit, onSave, fieldErrors = {}, saving = false, children }: CompanyTabViewProps) {
  const t = useTranslations('settings.company');
  const common = useTranslations('common');
  const [values, setValues] = useState<Record<string, string>>({
    name: company?.name ?? '',
    address: company?.address ?? '',
    sector: company?.sector ?? '',
    total_area_m2: company?.total_area_m2 ?? '',
    personnel_count: company?.personnel_count == null ? '' : String(company.personnel_count),
    contact_name: company?.contact_name ?? '',
    contact_phone: company?.contact_phone ?? '',
  });

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle>{t('title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="flex max-w-xl flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              onSave({
                name: values.name,
                address: values.address || null,
                sector: values.sector || null,
                // A decimal field is omitted when empty; the API has no null for it.
                total_area_m2: values.total_area_m2 || undefined,
                personnel_count: values.personnel_count === '' ? null : Number(values.personnel_count),
                contact_name: values.contact_name || null,
                contact_phone: values.contact_phone || null,
              });
            }}
          >
            <Input label={t('name')} value={values.name} disabled={!canEdit} error={fieldErrors.name} onChange={(e) => setValues({ ...values, name: e.target.value })} required />
            <Textarea label={t('address')} value={values.address} disabled={!canEdit} rows={2} onChange={(e) => setValues({ ...values, address: e.target.value })} />
            <Input label={t('sector')} value={values.sector} disabled={!canEdit} error={fieldErrors.sector} onChange={(e) => setValues({ ...values, sector: e.target.value })} />
            <NumberInput label={t('totalArea')} value={values.total_area_m2} disabled={!canEdit} onValueChange={(v) => setValues({ ...values, total_area_m2: v ?? '' })} fractionDigits={2} />
            <NumberInput label={t('personnel')} value={values.personnel_count} disabled={!canEdit} onValueChange={(v) => setValues({ ...values, personnel_count: v ?? '' })} fractionDigits={0} />
            <Input label={t('contactName')} value={values.contact_name} disabled={!canEdit} onChange={(e) => setValues({ ...values, contact_name: e.target.value })} />
            <Input label={t('contactPhone')} value={values.contact_phone} disabled={!canEdit} onChange={(e) => setValues({ ...values, contact_phone: e.target.value })} />
            {canEdit ? (
              <Button type="submit" loading={saving} className="self-start">
                {common('save')}
              </Button>
            ) : null}
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('analyzerCounts')}</CardTitle>
        </CardHeader>
        <CardContent>
          {company && company.analyzer_counts.length > 0 ? (
            <TableContainer label={t('analyzerCounts')}>
              <Table>
                <TableCaption>{t('analyzerCounts')}</TableCaption>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('provider')}</TableHead>
                    <TableHead numeric>{t('count')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {company.analyzer_counts.map((count) => (
                    <TableRow key={`${count.provider}-${count.subtype}`}>
                      <TableCell>{`${count.provider} / ${count.subtype}`}</TableCell>
                      <TableCell numeric>{formatNumber(count.count)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          ) : (
            <p className="text-foreground-muted type-small">{t('noAnalyzers')}</p>
          )}
        </CardContent>
      </Card>

      {children}
    </div>
  );
}
