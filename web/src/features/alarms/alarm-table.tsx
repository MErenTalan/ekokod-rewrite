'use client';

import { useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { RadioGroup } from '@/components/ui/radio-group';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Alarm, AlarmAnalyzer } from '@/lib/api/types';

import { typeLabelKey } from './alarm-labels';

/** Active/passive filter of 01 §7.12; 'all' is the default. */
export type AlarmStateFilter = 'all' | 'active' | 'passive';

export type AlarmTableViewProps = {
  alarms: Alarm[];
  state: AlarmStateFilter;
  onStateChange: (state: AlarmStateFilter) => void;
  /** alarms.edit (A/CA/BA) gates create, edit, delete and the toggle. */
  canEdit: boolean;
  /** alarms.evaluate (A/CA) gates the dry run. */
  canEvaluate: boolean;
  onCreate: () => void;
  onEdit: (alarm: Alarm) => void;
  onDelete: (alarm: Alarm) => void;
  onToggle: (alarm: Alarm, enabled: boolean) => void;
  onDetails: (alarm: Alarm) => void;
  onEvaluate: (alarm: Alarm) => void;
  loading?: boolean;
};

/**
 * The rule list of 01 §7.12: name, type, applied analyzers, the enable toggle
 * and the per-row actions.
 *
 * "View details" and "view logs" are the SAME dialog, as they were in legacy —
 * one "Detaylar" button that opens the rule's settings above its history.
 */
export function AlarmTableView({
  alarms, state, onStateChange, canEdit, canEvaluate,
  onCreate, onEdit, onDelete, onToggle, onDetails, onEvaluate, loading = false,
}: AlarmTableViewProps) {
  const t = useTranslations('alarms');
  const common = useTranslations('common');

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <RadioGroup
          label={t('state')}
          value={state}
          onValueChange={(value) => onStateChange(value as AlarmStateFilter)}
          options={[
            { value: 'all', label: t('all') },
            { value: 'active', label: t('active') },
            { value: 'passive', label: t('passive') },
          ]}
        />
        {canEdit ? <Button onClick={onCreate}>{t('addNew')}</Button> : null}
      </div>

      {loading ? (
        <Skeleton className="h-64 w-full" />
      ) : alarms.length === 0 ? (
        <EmptyState
          title={state === 'all' ? t('empty') : t('emptyFiltered')}
          description={state === 'all' ? t('emptyDescription') : t('emptyFilteredDescription')}
        />
      ) : (
        <TableContainer label={t('title')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('name')}</TableHead>
                <TableHead>{t('type')}</TableHead>
                <TableHead>{t('appliedAnalyzers')}</TableHead>
                <TableHead>{t('state')}</TableHead>
                <TableHead>{t('actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {alarms.map((alarm) => (
                <TableRow key={alarm.id}>
                  <TableCell className="font-medium">{alarm.name}</TableCell>
                  <TableCell>{t(typeLabelKey(alarm.type))}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {(alarm.analyzers ?? []).map((a: AlarmAnalyzer) => (
                        <Badge key={a.id} tone="neutral">{a.installation_number}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={alarm.is_enabled}
                      disabled={!canEdit}
                      label={alarm.is_enabled ? t('active') : t('passive')}
                      onCheckedChange={(checked) => onToggle(alarm, checked)}
                    />
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-2">
                      <Button variant="ghost" size="sm" onClick={() => onDetails(alarm)}>
                        {t('details')}
                      </Button>
                      {canEvaluate ? (
                        <Button variant="ghost" size="sm" onClick={() => onEvaluate(alarm)}>
                          {t('evaluate.action')}
                        </Button>
                      ) : null}
                      {canEdit ? (
                        <>
                          <Button variant="ghost" size="sm" onClick={() => onEdit(alarm)}>
                            {common('edit')}
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => onDelete(alarm)}>
                            {common('delete')}
                          </Button>
                        </>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}
