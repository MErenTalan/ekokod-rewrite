'use client';

import { Table2 } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useId, useState, type ReactNode } from 'react';

import { formatNumber } from '@/lib/format';

import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { EmptyState } from '../ui/empty-state';
import { Skeleton } from '../ui/skeleton';
import { ChartDataTable } from './_chart-data-table';
import { ChartLegend, type LegendExtra } from './_chart-legend';
import { DOWNSAMPLE_THRESHOLD, MAX_SERIES, type ChartSeries, type Datum } from './_theme';
import { downsampleLttb } from './downsample';

export type BaseChartProps = {
  title: string;
  description: string;
  data: Datum[];
  series: ChartSeries[];
  xLabel: string;
  formatX?: (x: string | number) => string;
  /** Plot height in px; the loading skeleton uses the same height (07 §5). */
  height?: number;
  loading?: boolean;
  empty: { title: string; description: string; action?: ReactNode };
  footnote?: string;
  dataTableDefaultOpen?: boolean;
};

type FrameProps = BaseChartProps & {
  children: (plot: Datum[]) => ReactNode;
  legendExtra?: LegendExtra[];
  /** Replaces the series legend (gauge, heatmap). */
  legend?: ReactNode;
  /** Replaces the series data table (gauge, heatmap). */
  table?: ReactNode;
  /** Overrides `data.length === 0` for charts whose data is not a Datum list. */
  isEmpty?: boolean;
  /** Hand-built SVGs scale with their width, so the plot box takes their natural height. */
  autoHeight?: boolean;
};

/**
 * Shared chart chrome: named figure, legend, plot at a fixed height, skeleton, empty state, the more-than-six-series
 * guard (D21), LTTB downsampling with a footnote, and the toggleable data table.
 */
export function ChartFrame({
  title,
  description,
  data,
  series,
  xLabel,
  formatX,
  height = 320,
  loading = false,
  empty,
  footnote,
  dataTableDefaultOpen = false,
  children,
  legendExtra,
  legend,
  table,
  isEmpty,
  autoHeight = false,
}: FrameProps) {
  const t = useTranslations('charts');
  const titleId = useId();
  const descriptionId = useId();
  const tableId = useId();
  const [tableOpen, setTableOpen] = useState(dataTableDefaultOpen);
  const tooMany = series.length > MAX_SERIES;
  const hasData = !(isEmpty ?? data.length === 0);
  const downsampled = series.length > 0 && data.length > DOWNSAMPLE_THRESHOLD;
  const plot = downsampled ? downsampleLttb(data, series[0].key, DOWNSAMPLE_THRESHOLD) : data;
  const ready = !loading && hasData && !tooMany;
  const notes = [
    downsampled ? t('downsampled', { shown: formatNumber(DOWNSAMPLE_THRESHOLD), total: formatNumber(data.length) }) : null,
    footnote ?? null,
  ].filter(Boolean);

  return (
    <figure aria-labelledby={titleId} aria-describedby={descriptionId} className="flex min-w-0 flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h3 id={titleId} className="text-foreground type-h3">
          {title}
        </h3>
        <p id={descriptionId} className="text-foreground-muted type-small">
          {description}
        </p>
      </div>
      {ready ? (legend ?? <ChartLegend series={series} extra={legendExtra} />) : null}
      {loading ? (
        <Skeleton className="w-full" style={{ height }} />
      ) : !hasData ? (
        <div className="flex items-center justify-center rounded-md border border-dashed border-border" style={{ minHeight: height }}>
          <EmptyState title={empty.title} description={empty.description} action={empty.action} />
        </div>
      ) : tooMany ? (
        <Alert tone="warning" title={t('tooManySeries')} />
      ) : (
        <div className="w-full" style={autoHeight ? undefined : { height }}>
          {children(plot)}
        </div>
      )}
      {ready && notes.length > 0 ? (
        <figcaption className="text-foreground-muted type-caption">{notes.join(' · ')}</figcaption>
      ) : null}
      {ready ? (
        <div className="flex flex-col gap-2">
          <Button
            variant="ghost"
            size="sm"
            iconStart={Table2}
            className="self-start"
            aria-expanded={tableOpen}
            aria-controls={tableId}
            onClick={() => setTableOpen((open) => !open)}
          >
            {tableOpen ? t('hideTable') : t('showTable')}
          </Button>
          <div id={tableId}>
            {tableOpen ? (table ?? <ChartDataTable caption={title} xLabel={xLabel} series={series} data={data} formatX={formatX} />) : null}
          </div>
        </div>
      ) : null}
    </figure>
  );
}
