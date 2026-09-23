'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import { Alert } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import { formatDate, formatDateTime } from '@/lib/format';
import type { BuildingTariffState, BulkTariffAssignment } from '@/lib/api/types';

export type BulkTabViewProps = {
  buildings: BuildingTariffState[];
  assignments: BulkTariffAssignment[];
  /** tariffs.edit (A/CA) gates the assignment; the two tables are read-only. */
  canEdit: boolean;
  onAssign: (buildingIDs: string[]) => void;
  loading?: boolean;
};

/**
 * The bulk tab of 01 §7.11: every building's current tariff, an assignment
 * over the ones the operator checks, and the history of what was assigned —
 * which the schema records only because R241 added the table for it.
 */
export function BulkTabView({ buildings, assignments, canEdit, onAssign, loading = false }: BulkTabViewProps) {
  const t = useTranslations('tariffs');
  const locale = useLocale() as Locale;
  const [selected, setSelected] = useState<string[]>([]);
  const [problem, setProblem] = useState<string | null>(null);

  const submit = () => {
    if (selected.length === 0) {
      setProblem(t('bulk.noSelection'));
      return;
    }
    setProblem(null);
    onAssign(selected);
  };

  if (loading) return <Skeleton className="h-64 w-full" />;

  return (
    <div className="flex flex-col gap-6">
      <section className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h3 className="type-h3">{t('bulk.current')}</h3>
            <p className="text-foreground-muted type-caption">{t('bulk.description')}</p>
          </div>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-2">
              {selected.length > 0 ? (
                <span role="status" className="text-foreground-muted type-caption">
                  {t('bulk.selected', { count: selected.length })}
                </span>
              ) : null}
              <Button variant="ghost" onClick={() => setSelected(buildings.map((b) => b.building_id))}>
                {t('bulk.selectAll')}
              </Button>
              <Button variant="ghost" onClick={() => setSelected([])}>{t('bulk.clearSelection')}</Button>
              <Button onClick={submit}>{t('bulk.apply')}</Button>
            </div>
          ) : null}
        </div>

        {problem ? <Alert tone="danger" title={problem} /> : null}

        <TableContainer label={t('bulk.current')}>
          <Table>
            <TableHeader>
              <TableRow>
                {canEdit ? <TableHead>{t('bulk.buildings')}</TableHead> : null}
                <TableHead>{t('columns.name')}</TableHead>
                <TableHead>{t('columns.effectiveFrom')}</TableHead>
                <TableHead>{t('columns.pricing')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {buildings.map((building) => (
                <TableRow key={building.building_id}>
                  {canEdit ? (
                    <TableCell>
                      <Checkbox
                        label={building.building_name}
                        checked={selected.includes(building.building_id)}
                        onCheckedChange={(checked) =>
                          setSelected((prev) =>
                            checked ? [...prev, building.building_id] : prev.filter((id) => id !== building.building_id),
                          )
                        }
                      />
                    </TableCell>
                  ) : null}
                  <TableCell className="font-medium">
                    {canEdit ? building.tariff_name ?? '—' : building.building_name}
                  </TableCell>
                  <TableCell>{building.effective_from ? formatDate(building.effective_from, locale) : '—'}</TableCell>
                  <TableCell>
                    {building.tariff_id ? (
                      <Badge tone={building.use_ptf_yekdem ? 'info' : 'neutral'}>
                        {building.use_ptf_yekdem ? t('ptfBadge') : t('fixedBadge')}
                      </Badge>
                    ) : (
                      <Badge tone="warning">{t('bulk.noTariff')}</Badge>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </section>

      <section className="flex flex-col gap-4">
        <h3 className="type-h3">{t('bulk.history')}</h3>
        {assignments.length === 0 ? (
          <EmptyState title={t('bulk.historyEmpty')} description={t('bulk.historyEmptyDescription')} />
        ) : (
          <TableContainer label={t('bulk.history')}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('bulk.columns.date')}</TableHead>
                  <TableHead>{t('bulk.columns.tariff')}</TableHead>
                  <TableHead>{t('bulk.columns.template')}</TableHead>
                  <TableHead>{t('bulk.columns.buildingCount')}</TableHead>
                  <TableHead>{t('columns.effectiveFrom')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {assignments.map((assignment) => (
                  <TableRow key={assignment.id}>
                    <TableCell>{formatDateTime(assignment.created_at, locale)}</TableCell>
                    <TableCell>{assignment.tariff_name ?? '—'}</TableCell>
                    <TableCell>
                      {assignment.template_id ? t('bulk.fromTemplate') : t('bulk.manual')}
                    </TableCell>
                    <TableCell className="type-data">{assignment.building_ids.length}</TableCell>
                    <TableCell>{formatDate(assignment.effective_from, locale)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </section>
    </div>
  );
}
