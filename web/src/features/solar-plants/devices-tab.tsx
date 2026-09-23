'use client';

import { useTranslations } from 'next-intl';

import { EmptyState } from '@/components/ui/empty-state';
import { SearchInput } from '@/components/ui/search-input';
import { StatusBadge, type Status } from '@/components/ui/status-badge';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { formatNumber } from '@/lib/format';
import type { PlantDeviceView } from '@/lib/api/types';

import { shortDateTime } from './format';

const STATUS: Record<string, Status> = { normal: 'success', alarm: 'warning', fault: 'danger', offline: 'neutral' };
const TYPES: Record<number, 'inverter' | 'storageInverter' | 'meteo' | 'meter'> = { 1: 'inverter', 14: 'storageInverter', 5: 'meteo', 7: 'meter' };

export type DevicesTabViewProps = {
  devices: PlantDeviceView[];
  query: string;
  onQueryChange: (q: string) => void;
};

/** §7.7's device table (R285); the search runs on the API's `q`. */
export function DevicesTabView({ devices, query, onQueryChange }: DevicesTabViewProps) {
  const t = useTranslations('solarPlants');
  const num = (v?: string) => (v ? formatNumber(v, { maxFractionDigits: 2 }) : t('noData'));
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="w-full max-w-sm">
          <SearchInput label={t('devices.search')} description={t('devices.searchDescription')} value={query} onValueChange={onQueryChange} />
        </div>
        <p className="text-foreground-muted type-small" aria-live="polite">{t('devices.count', { count: devices.length })}</p>
      </div>
      {devices.length === 0 ? (
        <EmptyState title={t('devices.empty')} description={t('devices.emptyDescription')} />
      ) : (
        <TableContainer label={t('tabs.devices')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('devices.name')}</TableHead>
                <TableHead>{t('devices.serial')}</TableHead>
                <TableHead>{t('devices.type')}</TableHead>
                <TableHead>{t('devices.status')}</TableHead>
                <TableHead>{t('devices.power')}</TableHead>
                <TableHead>{t('devices.today')}</TableHead>
                <TableHead>{t('devices.total')}</TableHead>
                <TableHead>{t('devices.lastUpdate')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {devices.map((d) => {
                const type = (d.device_type !== undefined && d.device_type !== null && TYPES[d.device_type]) || 'other';
                return (
                  <TableRow key={d.id}>
                    <TableCell>{d.device_name ?? '—'}</TableCell>
                    <TableCell className="type-data">{d.device_sn}</TableCell>
                    <TableCell>{t(`devices.types.${type}`)}</TableCell>
                    <TableCell>
                      <StatusBadge status={STATUS[d.status ?? ''] ?? 'neutral'} label={t(`devices.statuses.${(d.status ?? 'unknown') as 'normal'}`)} />
                    </TableCell>
                    <TableCell className="type-data">{num(d.active_power_kw)}</TableCell>
                    <TableCell className="type-data">{num(d.yield_today_kwh)}</TableCell>
                    <TableCell className="type-data">{num(d.yield_total_kwh)}</TableCell>
                    <TableCell>{d.last_update ? shortDateTime(d.last_update) : t('noData')}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}
