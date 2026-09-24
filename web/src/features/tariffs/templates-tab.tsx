'use client';

import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import { Alert } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog } from '@/components/ui/dialog';
import { EmptyState } from '@/components/ui/empty-state';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { Locale } from '@/i18n/locale';
import { formatDate } from '@/lib/format';
import type { BuildingTariffState, TariffTemplate } from '@/lib/api/types';

export type TemplateApply = { templateID: string; buildingIDs: string[]; effectiveFrom: string };

export type TemplatesTabViewProps = {
  templates: TariffTemplate[];
  buildings: BuildingTariffState[];
  /** tariffs.edit (A/CA) gates create, edit, delete and apply. */
  canEdit: boolean;
  onCreate: () => void;
  onEdit: (template: TariffTemplate) => void;
  onDelete: (template: TariffTemplate) => void;
  onApply: (apply: TemplateApply) => void;
  loading?: boolean;
};

/** The template tab of 01 §7.11: named definitions, applied to many buildings. */
export function TemplatesTabView({
  templates, buildings, canEdit, onCreate, onEdit, onDelete, onApply, loading = false,
}: TemplatesTabViewProps) {
  const t = useTranslations('tariffs');
  const common = useTranslations('common');
  const locale = useLocale() as Locale;

  const [applying, setApplying] = useState<TariffTemplate | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [effectiveFrom, setEffectiveFrom] = useState('');
  const [problem, setProblem] = useState<string | null>(null);

  const close = () => {
    setApplying(null);
    setSelected([]);
    setEffectiveFrom('');
    setProblem(null);
  };

  const submit = () => {
    if (!applying) return;
    if (selected.length === 0) {
      setProblem(t('bulk.noSelection'));
      return;
    }
    onApply({ templateID: applying.id, buildingIDs: selected, effectiveFrom });
    close();
  };

  if (loading) return <Skeleton className="h-64 w-full" />;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h3 className="type-h3">{t('templates.title')}</h3>
          <p className="text-foreground-muted type-caption">{t('templates.description')}</p>
        </div>
        {canEdit ? <Button onClick={onCreate}>{t('templates.new')}</Button> : null}
      </div>

      {templates.length === 0 ? (
        <EmptyState title={t('templates.empty')} description={t('templates.emptyDescription')} />
      ) : (
        <TableContainer label={t('templates.title')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('templates.name')}</TableHead>
                <TableHead>{t('templates.descriptionField')}</TableHead>
                <TableHead>{t('columns.pricing')}</TableHead>
                <TableHead>{common('actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {templates.map((template) => (
                <TableRow key={template.id}>
                  <TableCell className="font-medium">
                    <div className="flex flex-wrap items-center gap-2">
                      {template.name}
                      {template.is_default ? <Badge tone="success">{t('templates.default')}</Badge> : null}
                    </div>
                  </TableCell>
                  <TableCell>{template.description ?? '—'}</TableCell>
                  <TableCell>
                    <Badge tone={template.tariff.use_ptf_yekdem ? 'info' : 'neutral'}>
                      {template.tariff.use_ptf_yekdem ? t('ptfBadge') : t('fixedBadge')}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {canEdit ? (
                      <div className="flex flex-wrap gap-2">
                        <Button variant="ghost" size="sm" onClick={() => setApplying(template)}>{t('templates.apply')}</Button>
                        <Button variant="ghost" size="sm" onClick={() => onEdit(template)}>{common('edit')}</Button>
                        <Button variant="ghost" size="sm" onClick={() => onDelete(template)}>{common('delete')}</Button>
                      </div>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <Dialog
        open={applying !== null}
        onOpenChange={(next) => !next && close()}
        title={t('templates.applyTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={close}>{common('cancel')}</Button>
            <Button onClick={submit}>{t('bulk.apply')}</Button>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          <Input
            label={t('bulk.effectiveFrom')}
            required
            type="date"
            value={effectiveFrom}
            onChange={(e) => setEffectiveFrom(e.target.value)}
          />
          {problem ? <Alert tone="danger" title={problem} /> : null}
          <fieldset className="flex flex-col gap-2">
            <legend className="text-foreground type-body">{t('bulk.buildings')}</legend>
            {buildings.map((building) => (
              <Checkbox
                key={building.building_id}
                label={`${building.building_name} — ${building.tariff_name ?? t('bulk.noTariff')}`}
                checked={selected.includes(building.building_id)}
                onCheckedChange={(checked) =>
                  setSelected((prev) =>
                    checked ? [...prev, building.building_id] : prev.filter((id) => id !== building.building_id),
                  )
                }
              />
            ))}
          </fieldset>
          {applying?.tariff.effective_from ? (
            <p className="text-foreground-muted type-caption">
              {t('columns.effectiveFrom')}: {formatDate(applying.tariff.effective_from, locale)}
            </p>
          ) : null}
        </div>
      </Dialog>
    </div>
  );
}
