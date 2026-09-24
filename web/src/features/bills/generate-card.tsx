'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { MultiSelect } from '@/components/ui/multi-select';
import type { Analyzer } from '@/lib/api/types';

import { validateSelection, type BillsMessageKey, type GenerationScope } from './generation-status';

export type GenerateRequest = {
  scope: GenerationScope;
  analyzerIds: string[];
  buildingId: string | null;
  period: string;
};

export type GenerateCardViewProps = {
  analyzers: Analyzer[];
  period: string | null;
  buildingId: string | null;
  onGenerate: (request: GenerateRequest) => void;
  /** Bill ids the finished job produced, ready to download. */
  readyBillIds?: string[];
  onDownload?: (billID: string) => void;
  /** The API's own failure, mapped to a §7.10 sentence (R238). */
  failure?: BillsMessageKey;
  running?: boolean;
};

/**
 * Mode B of 01 §7.10: pick meters and a month, then generate the analyzer,
 * building or company invoice. The selection guards run before anything is
 * enqueued, so every §7.10 validation message is reachable.
 */
export function GenerateCardView({
  analyzers, period, buildingId, onGenerate, readyBillIds = [], onDownload, failure, running = false,
}: GenerateCardViewProps) {
  const t = useTranslations('bills');
  const common = useTranslations('common');
  const [analyzerIds, setAnalyzerIds] = useState<string[]>([]);
  const [problem, setProblem] = useState<BillsMessageKey | null>(null);

  const start = (scope: GenerationScope) => {
    const invalid = validateSelection({ analyzerIds, buildingId, period, scope });
    setProblem(invalid);
    if (invalid || !period) return;
    onGenerate({ scope, analyzerIds, buildingId, period });
  };

  const shown = problem ?? failure ?? null;

  return (
    <Card className="p-4 flex flex-col gap-4">
      <div>
        <h3 className="type-h3">{t('generate.title')}</h3>
        <p className="text-foreground-muted type-caption">{t('generate.description')}</p>
      </div>

      <MultiSelect
        label={t('generate.analyzers')}
        value={analyzerIds}
        onValueChange={setAnalyzerIds}
        options={analyzers.map((a) => ({
          value: a.id,
          label: [a.installation_number, a.customer_name, [a.province, a.district].filter(Boolean).join('/')]
            .filter(Boolean)
            .join(' · '),
        }))}
        searchPlaceholder={common('search')}
        emptyText={t('noData')}
      />

      {shown ? <Alert tone="danger" title={t(`errors.${shown}`)} /> : null}

      <div className="flex flex-wrap gap-2">
        <Button loading={running} onClick={() => start('analyzer')}>{t('generate.analyzerBill')}</Button>
        <Button variant="secondary" loading={running} onClick={() => start('building')}>{t('generate.buildingBill')}</Button>
        <Button variant="secondary" loading={running} onClick={() => start('company')}>{t('generate.companyBill')}</Button>
      </div>

      {readyBillIds.length > 0 && onDownload ? (
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-foreground type-body">{t('generate.ready')}</p>
          {readyBillIds.map((id) => (
            <Button key={id} variant="ghost" onClick={() => onDownload(id)}>{t('generate.downloadReady')}</Button>
          ))}
        </div>
      ) : null}
    </Card>
  );
}
