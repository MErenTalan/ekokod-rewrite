import { Check } from 'lucide-react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/cn';

import { RM_FEATURES } from './content';
import { PageHero } from './site-blocks';

const EXTRAS = ['customReports', 'multiBuilding', 'analytics', 'api'] as const;
const PACKAGES = [
  { key: 'standard', extras: false, popular: false },
  { key: 'premium', extras: true, popular: true },
  { key: 'enterprise', extras: true, popular: false },
] as const;

/** Pricing (01 §7.19, behind EKOKOD_FEATURE_PRICING_PAGE): packages, a "most popular" marker, contact CTA. */
export function PricingView() {
  const t = useTranslations('site');
  const features = RM_FEATURES.map((k) => t(`features.${k}`));
  const extras = EXTRAS.map((k) => t(`pricing.extras.${k}`));
  return (
    <>
      <PageHero title={t('pricing.title')} description={t('pricing.description')} />
      <div className="mx-auto grid max-w-7xl gap-6 px-4 py-14 sm:px-6 lg:grid-cols-3">
        {PACKAGES.map((p) => {
          const title = t(`pricing.packages.${p.key}.title`);
          return (
            <article
              key={p.key}
              aria-labelledby={`pkg-${p.key}`}
              aria-describedby={p.popular ? `pkg-${p.key}-popular` : undefined}
              className={cn('flex flex-col gap-5 rounded-lg border bg-surface p-6', p.popular ? 'border-primary ring-2 ring-primary' : 'border-border')}
            >
              <div className="flex flex-col gap-2">
                {p.popular ? (
                  <span id={`pkg-${p.key}-popular`} className="self-start">
                    <Badge tone="success">{t('pricing.popular')}</Badge>
                  </span>
                ) : null}
                <h2 id={`pkg-${p.key}`} className="text-foreground type-h1">{title}</h2>
                <p className="text-foreground-muted">{t(`pricing.packages.${p.key}.description`)}</p>
                <p className="text-foreground type-h2">{t('pricing.price')}</p>
              </div>
              <div className="flex flex-1 flex-col gap-3">
                <h3 className="text-foreground type-h3">{t('pricing.includes')}</h3>
                <ul className="flex flex-col gap-2">
                  {[...features, ...(p.extras ? extras : [])].map((f) => (
                    <li key={f} className="flex items-start gap-2 text-foreground">
                      <Check aria-hidden className="mt-0.5 size-4 shrink-0 stroke-brand" />
                      <span>{f}</span>
                    </li>
                  ))}
                </ul>
                {p.key === 'enterprise' ? <p className="text-foreground-muted">{t('pricing.packages.enterprise.custom')}</p> : null}
              </div>
              <Button asChild size="lg" variant={p.popular ? 'primary' : 'secondary'}>
                <Link href={`/contact?subject=${encodeURIComponent(`${t('pricing.title')}: ${title}`)}`} aria-label={`${t('pricing.action')}: ${title}`}>
                  {t('pricing.action')}
                </Link>
              </Button>
            </article>
          );
        })}
      </div>
    </>
  );
}
