'use client';

import { useTranslations } from 'next-intl';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import type { FinancialMonth } from '@/lib/api/types';
import { formatQuantity } from '@/lib/format';

/** R294's offset: production − consumption, split into grid purchase and sale, labelled as computed. */
export function OffsetCard({ figures }: { figures: FinancialMonth }) {
  const t = useTranslations('financial.offset');
  const rows = [
    { label: t('offset'), value: figures.offset_kwh },
    { label: t('purchase'), value: figures.grid_purchase_kwh },
    { label: t('sale'), value: figures.grid_sale_kwh },
  ];
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <CardDescription>{t('hint')}</CardDescription>
      </CardHeader>
      <CardContent>
        {figures.offset_kwh ? (
          <dl className="grid gap-3 sm:grid-cols-3">
            {rows.map((r) => (
              <div key={r.label} className="flex flex-col gap-0.5">
                <dt className="text-foreground-muted type-caption">{r.label}</dt>
                <dd className="text-foreground type-data">
                  {formatQuantity(r.value, 'kWh', { maxFractionDigits: 2 })}
                </dd>
              </div>
            ))}
          </dl>
        ) : (
          <p className="text-foreground-muted type-body">{t('unavailable')}</p>
        )}
      </CardContent>
    </Card>
  );
}
