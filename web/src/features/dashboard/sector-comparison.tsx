'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { ExportMenu } from '@/components/domain/export-menu';
import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { BuildingComparison, ComparisonMetric, Translator } from '@/lib/api/types';
import { toCsv } from '@/lib/csv';
import { formatNumber } from '@/lib/format';

/** The five compared figures, in the order 01 §7.2 lists them. */
const METRICS = ['daily_consumption', 'monthly_consumption', 'co2_emission_kg', 'consumption_per_capita', 'consumption_per_area'] as const;
type MetricKey = (typeof METRICS)[number];

const LABEL = {
  daily_consumption: 'daily',
  monthly_consumption: 'monthly',
  co2_emission_kg: 'co2',
  consumption_per_capita: 'perCapita',
  consumption_per_area: 'perArea',
} as const;
/** The three figures legacy ranked a building on. */
const RANKED: MetricKey[] = ['consumption_per_capita', 'consumption_per_area', 'monthly_consumption'];

/** The table as CSV rows: the building, the sector average, then its ranks (R205). */
export function sectorCsvRows(comparison: BuildingComparison, buildingName: string, t: Translator): string[][] {
  const header = [t('building'), ...METRICS.map((key) => t(LABEL[key]))];
  const values = (pick: (m: ComparisonMetric) => string | undefined) =>
    METRICS.map((key) => formatNumber(pick(comparison[key]) ?? null));
  return [
    header,
    [buildingName, ...values((m) => m.value)],
    [t('average'), ...values((m) => m.average)],
    ...RANKED.map((key) => [
      t(LABEL[key]),
      comparison[key].ranked > 0
        ? t('rankLine', { rank: comparison[key].rank, peers: comparison[key].ranked })
        : t('unranked'),
      '',
      '',
      '',
      '',
    ]),
  ];
}

export type SectorComparisonViewProps = {
  buildingName: string | null;
  comparison: BuildingComparison | null;
  /** 409 building_sector_missing: the building has no sector yet. */
  sectorMissing?: boolean;
  canEditBuildings: boolean;
  onExport: () => void;
  loading?: boolean;
};

/**
 * The sectoral comparison table 10 §13 restored: the building against the
 * average of its sector, with the ranks legacy showed and a CSV download.
 */
export function SectorComparisonView({
  buildingName,
  comparison,
  sectorMissing = false,
  canEditBuildings,
  onExport,
  loading = false,
}: SectorComparisonViewProps) {
  const t = useTranslations('dashboard.sector');
  const body = () => {
    if (loading) return <Skeleton className="h-40 w-full" />;
    if (!buildingName) return <EmptyState title={t('noBuilding')} description={t('noBuildingHint')} />;
    if (sectorMissing) {
      return (
        <Alert tone="warning" title={t('missingSector')}>
          {canEditBuildings ? (
            <Button asChild size="sm" variant="secondary">
              <Link href="/ekorm/settings?tab=buildings">{t('missingSectorAction')}</Link>
            </Button>
          ) : null}
        </Alert>
      );
    }
    if (!comparison) return <EmptyState title={t('noBuilding')} description={t('noBuildingHint')} />;
    if (!comparison.available) return <Alert tone="info" title={t('tooSmall')} />;
    return (
      <div className="flex flex-col gap-3">
        <p className="text-foreground-muted type-small">
          {t('header', { sector: comparison.sector, count: comparison.peers })}
        </p>
        <TableContainer label={t('title')}>
          <Table>
            <TableCaption>{t('title')}</TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead>{t('building')}</TableHead>
                {METRICS.map((key) => (
                  <TableHead key={key} numeric>
                    {t(LABEL[key])}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell>{buildingName}</TableCell>
                {METRICS.map((key) => (
                  <TableCell key={key} numeric>
                    {formatNumber(comparison[key].value ?? null)}
                  </TableCell>
                ))}
              </TableRow>
              <TableRow>
                <TableCell>{t('average')}</TableCell>
                {METRICS.map((key) => (
                  <TableCell key={key} numeric>
                    {formatNumber(comparison[key].average ?? null)}
                  </TableCell>
                ))}
              </TableRow>
            </TableBody>
          </Table>
        </TableContainer>
        <ul className="flex flex-col gap-1 type-small">
          {RANKED.map((key) => (
            <li key={key}>
              {`${t(LABEL[key])}: `}
              {comparison[key].ranked > 0
                ? t('rankLine', { rank: comparison[key].rank, peers: comparison[key].ranked })
                : t('unranked')}
            </li>
          ))}
        </ul>
      </div>
    );
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <CardTitle>{t('title')}</CardTitle>
        {comparison?.available ? <ExportMenu formats={['csv']} onExport={onExport} /> : null}
      </CardHeader>
      <CardContent>{body()}</CardContent>
    </Card>
  );
}

/** Builds the CSV file the export menu saves (D22 formatting). */
export function sectorCsvFile(comparison: BuildingComparison, buildingName: string, t: Translator): { name: string; content: string } {
  return { name: `${buildingName}-sektor-karsilastirma.csv`, content: toCsv(sectorCsvRows(comparison, buildingName, t)) };
}
