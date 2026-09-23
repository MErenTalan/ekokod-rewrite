'use client';

import { Pencil } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { FilterBar } from '@/components/shell/filter-bar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { IconButton } from '@/components/ui/icon-button';
import { SearchInput } from '@/components/ui/search-input';
import { Select } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { EmissionFactorView } from '@/lib/api/types';
import { formatNumber } from '@/lib/format';

import { mainKey } from './labels';

const MAINS = ['cat_stationary', 'cat_mobility', 'cat_logistics', 'cat_gas_emissions', 'cat_electricity', 'cat_external_energy', 'cat_products', 'cat_waste'];

export type FactorTableProps = {
  rows: EmissionFactorView[];
  editable: boolean;
  query: string;
  onQuery: (q: string) => void;
  main: string;
  onMain: (main: string) => void;
  onOverride: (f: EmissionFactorView) => void;
  onReset: () => void;
};

/** R326: the effective catalogue; A CA override a value or restore the platform list. */
export function FactorTable({ rows, editable, query, onQuery, main, onMain, onOverride, onReset }: FactorTableProps) {
  const t = useTranslations('carbon');
  return (
    <div className="flex flex-col gap-4">
      <p className="text-foreground-muted type-body">{editable ? t('database.intro') : t('database.readOnly')}</p>
      <FilterBar>
        <SearchInput label={t('database.search')} value={query} onValueChange={onQuery} />
        <Select
          label={t('database.mainFilter')}
          value={main}
          onValueChange={onMain}
          options={[{ value: 'all', label: t('database.allMains') }, ...MAINS.map((m) => ({ value: m, label: t(mainKey(m)) }))]}
        />
        {editable ? (
          <Button variant="secondary" onClick={onReset}>
            {t('database.reset')}
          </Button>
        ) : null}
      </FilterBar>
      {rows.length === 0 ? (
        <EmptyState title={t('database.empty')} description={t('database.emptyHint')} />
      ) : (
        <TableContainer label={t('database.label')}>
          <Table aria-label={t('database.label')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t('database.name')}</TableHead>
                <TableHead>{t('database.main')}</TableHead>
                <TableHead numeric>{t('database.factor')}</TableHead>
                <TableHead>{t('database.source')}</TableHead>
                {editable ? <TableHead>{t('database.override')}</TableHead> : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((f) => (
                <TableRow key={f.id}>
                  <TableCell>
                    <span className="block">{f.label}</span>
                    <span className="block text-foreground-muted type-caption">{f.key}</span>
                  </TableCell>
                  <TableCell>{t(mainKey(f.main_category))}</TableCell>
                  <TableCell numeric className="whitespace-nowrap">
                    <span className="block">{`${formatNumber(f.base_factor)} / ${f.base_unit}`}</span>
                    {f.overridden ? (
                      <span className="flex flex-col items-end gap-1">
                        <Badge tone="info">{t('database.overridden')}</Badge>
                        {f.platform_base_factor ? (
                          <span className="text-foreground-muted type-caption">{t('database.platformValue', { value: formatNumber(f.platform_base_factor) })}</span>
                        ) : null}
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell>{[f.source, f.source_year].filter(Boolean).join(', ') || '—'}</TableCell>
                  {editable ? (
                    <TableCell>
                      <IconButton label={t('database.overrideName', { name: f.label })} icon={Pencil} onClick={() => onOverride(f)} />
                    </TableCell>
                  ) : null}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}
