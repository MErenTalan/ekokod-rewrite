'use client';

import { useTranslations } from 'next-intl';

import { formatNumber } from '@/lib/format';

import { Button } from '../ui/button';
import { StatusBadge } from '../ui/status-badge';
import { Table, TableBody, TableCaption, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '../ui/table';
import type { MapMarker } from './map';

const coordinate = (v: number) => formatNumber(v, { minFractionDigits: 5, maxFractionDigits: 5 });

/** The map's accessible equivalent and its offline fallback (plan D23). */
export function MapMarkerList({ markers, label, onMarkerSelect }: { markers: MapMarker[]; label: string; onMarkerSelect?: (id: string) => void }) {
  const t = useTranslations('map');
  return (
    <TableContainer label={label} className="max-h-96">
      <Table>
        <TableCaption>{label}</TableCaption>
        <TableHeader>
          <TableRow>
            <TableHead>{t('name')}</TableHead>
            <TableHead>{t('status')}</TableHead>
            <TableHead>{t('coordinates')}</TableHead>
            {onMarkerSelect ? (
              <TableHead>
                <span className="sr-only">{t('select')}</span>
              </TableHead>
            ) : null}
          </TableRow>
        </TableHeader>
        <TableBody>
          {markers.map((m) => (
            <TableRow key={m.id}>
              <TableHead scope="row" className="text-foreground type-body">
                {m.name}
              </TableHead>
              <TableCell>
                <StatusBadge status={m.status === 'active' ? 'success' : 'neutral'} label={t(m.status)} />
              </TableCell>
              <TableCell className="type-data whitespace-nowrap">{`${coordinate(m.lat)}, ${coordinate(m.lng)}`}</TableCell>
              {onMarkerSelect ? (
                <TableCell>
                  <Button size="sm" variant="ghost" aria-label={t('selectMarker', { name: m.name })} onClick={() => onMarkerSelect(m.id)}>
                    {t('select')}
                  </Button>
                </TableCell>
              ) : null}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
