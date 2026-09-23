'use client';

import { useTranslations } from 'next-intl';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import type { FinancialTariffs } from '@/lib/api/types';

import { money } from './money-lines';

/** R294's tariff panel: each building's purchase tariff and each plant's feed-in in force today. */
export function TariffPanel({ tariffs }: { tariffs: FinancialTariffs }) {
  const t = useTranslations('financial.tariffs');
  const price = (v: string | undefined | null, currency: string) =>
    v ? `${money({ currency: currency as 'TRY', amount: v })}${t('perKwh')}` : null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-6 md:grid-cols-2">
        <section className="flex flex-col gap-2" aria-labelledby="fin-purchase">
          <h4 id="fin-purchase" className="text-foreground type-h4">
            {t('purchase')}
          </h4>
          {tariffs.purchase_missing ? (
            <p className="text-foreground-muted type-body">{t('purchaseMissing')}</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {tariffs.purchase.map((b) => (
                <li key={b.building_id} className="flex flex-col">
                  <span className="text-foreground type-body">{b.building_name}</span>
                  <span className="text-foreground-muted type-small">
                    {b.price_type === 'single_time'
                      ? `${t('single')}: ${price(b.single, b.currency) ?? '—'}`
                      : (['t1', 't2', 't3'] as const)
                          .map((k) => `${t(k)}: ${price(b[k], b.currency) ?? '—'}`)
                          .join(' · ')}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
        <section className="flex flex-col gap-2" aria-labelledby="fin-sale">
          <h4 id="fin-sale" className="text-foreground type-h4">
            {t('sale')}
          </h4>
          {tariffs.sale_missing ? (
            <p className="text-foreground-muted type-body">{t('saleMissing')}</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {tariffs.sale.map((p) => (
                <li key={p.plant_id} className="flex flex-col">
                  <span className="text-foreground type-body">{p.plant_name}</span>
                  <span className="text-foreground-muted type-small">
                    {price(p.price, p.currency)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </CardContent>
    </Card>
  );
}
