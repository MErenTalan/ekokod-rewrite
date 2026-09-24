'use client';

import { AlertTriangle, ShieldAlert } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { formatQuantity } from '@/lib/format';

import { Badge } from '../ui/badge';
import { Tooltip } from '../ui/tooltip';

export type DataQuality = { state: 'complete' } | { state: 'estimated' | 'incomplete' | 'suspect'; reason: string; coverage?: string };

/** Any figure derived from incomplete data is visibly marked (07 §6); the reason is keyboard reachable. */
export function DataQualityBadge({ quality }: { quality: DataQuality }) {
  const t = useTranslations('domain.quality');
  if (quality.state === 'complete') return null;
  const suspect = quality.state === 'suspect';
  const Icon = suspect ? ShieldAlert : AlertTriangle;
  return (
    <Tooltip
      content={
        <span className="flex flex-col gap-0.5">
          <span>{quality.reason}</span>
          {quality.coverage ? <span className="text-foreground-muted">{t('coverage', { value: formatQuantity(quality.coverage, 'percent') })}</span> : null}
        </span>
      }
    >
      <Badge data-quality-badge tone={suspect ? 'danger' : 'warning'} tabIndex={0} className="cursor-help pointer-coarse:min-h-11">
        <Icon aria-hidden className="size-3.5 shrink-0" />
        {t(quality.state)}
      </Badge>
    </Tooltip>
  );
}
