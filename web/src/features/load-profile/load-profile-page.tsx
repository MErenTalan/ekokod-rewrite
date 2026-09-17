'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import type { ExportFormat } from '@/components/domain/export-menu';
import { DateRangePicker, type DateRange } from '@/components/ui/date-range-picker';
import { periodPresets } from '@/components/domain/period-filter-bar';
import { Button } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { Tabs } from '@/components/ui/tabs';
import { FilterBar } from '@/components/shell/filter-bar';
import { PageHeader } from '@/components/shell/page-header';
import { ScopePicker } from '@/features/scope/scope-picker';
import { downloadFile } from '@/lib/api/download';
import { $api } from '@/lib/api/query';
import { addDays, istanbulToday } from '@/lib/dates';
import { useScopeParams, useSelection } from '@/lib/selection/selection-store';

import { CompareTabView } from './compare-tab';
import { DailyTabView } from './daily-tab';
import { DetailsTabView } from './details-tab';
import { PROFILE_KEYS, type ProfileKey } from './profile-label';
import { SeasonalTabView } from './seasonal-tab';

const WEEKDAY_NAMES = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'] as const;

/** The load-profile screen of 01 §7.4; every profile comes from one request. */
export function LoadProfilePage() {
  const t = useTranslations('loadProfile');
  const config = useTranslations('loadProfile.config');
  const forms = useTranslations('forms');
  const common = useTranslations('common');
  const calendar = useTranslations('calendar.weekdays');
  const scope = useScopeParams();
  const { analyzerId } = useSelection();
  const today = istanbulToday();

  const [activeOnly, setActiveOnly] = useState(false);
  const [draft, setDraft] = useState<DateRange>({ from: addDays(today, -182), to: today });
  const [range, setRange] = useState(draft);
  const [compared, setCompared] = useState<ProfileKey[]>(['weekday', 'weekend']);
  const [busyFormat, setBusyFormat] = useState<ExportFormat | null>(null);

  const enabled = Boolean(analyzerId);
  const query = { ...scope, analyzer_id: analyzerId ?? '', from: range.from, to: range.to, profiles: [...PROFILE_KEYS] };
  const profiles = $api.useQuery('get', '/api/v1/load-profile', { params: { query } }, { enabled });
  const statistics = $api.useQuery('get', '/api/v1/load-profile/statistics', { params: { query } }, { enabled });

  const used = profiles.data?.config;
  const weekendDays = (used?.weekend_days ?? []).map((day) => calendar(WEEKDAY_NAMES[day])).join(', ');

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('title')} description={t('subtitle')} />
      <FilterBar>
        <ScopePicker activeOnly={activeOnly} onActiveOnlyChange={setActiveOnly} />
        <div className="w-80 max-w-full">
          <DateRangePicker
            label={forms('chooseDateRange')}
            value={draft}
            onValueChange={setDraft}
            max={today}
            presets={periodPresets(today).map((p) => ({ id: p.id, label: forms(p.id), range: p.range }))}
          />
        </div>
        <Button onClick={() => setRange(draft)} loading={profiles.isFetching}>
          {common('apply')}
        </Button>
      </FilterBar>

      {used ? (
        <p className="flex flex-wrap items-center gap-2 text-foreground-muted type-small">
          {config('weekendDays', {
            days: weekendDays,
            source: used.weekend_source === 'company' ? config('sourceCompany') : config('sourceDefault'),
            vacations: used.vacations,
          })}
          <Button asChild variant="ghost" size="sm">
            <Link href="/ekorm/calendar">{config('calendarLink')}</Link>
          </Button>
        </p>
      ) : null}

      {!enabled ? (
        <EmptyState title={t('selectAnalyzer')} description={t('selectAnalyzerHint')} />
      ) : (
        <Tabs
          items={[
            { value: 'daily', label: t('tabs.daily'), content: <DailyTabView data={profiles.data ?? null} loading={profiles.isFetching} /> },
            { value: 'seasonal', label: t('tabs.seasonal'), content: <SeasonalTabView data={profiles.data ?? null} loading={profiles.isFetching} /> },
            {
              value: 'compare',
              label: t('tabs.compare'),
              content: (
                <CompareTabView
                  data={profiles.data ?? null}
                  selected={compared}
                  onSelectedChange={setCompared}
                  loading={profiles.isFetching}
                />
              ),
            },
            {
              value: 'details',
              label: t('tabs.details'),
              content: (
                <DetailsTabView
                  statistics={statistics.data ?? null}
                  loading={statistics.isFetching}
                  busyFormat={busyFormat}
                  onExport={async (format) => {
                    setBusyFormat(format);
                    try {
                      await downloadFile(
                        '/api/v1/load-profile/export',
                        { ...scope, analyzer_id: analyzerId ?? '', from: range.from, to: range.to },
                        `yuk-profili-${range.from}-${range.to}.xlsx`,
                      );
                    } finally {
                      setBusyFormat(null);
                    }
                  }}
                />
              ),
            },
          ]}
        />
      )}
    </div>
  );
}
