'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { Alert } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { FileUpload } from '@/components/ui/file-upload';
import { Input } from '@/components/ui/input';
import { Stepper } from '@/components/ui/stepper';
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { BuildingTariffState, IcmalAnalysis, IcmalCoefficient, IcmalImport } from '@/lib/api/types';

const MAX_UPLOAD_BYTES = 10 * 1024 * 1024;

export type IcmalConfirmation = { building_id: string; etso_code: string; effective_from: string };

export type IcmalTabViewProps = {
  /** The analysis of the current upload, or null before one exists. */
  result: IcmalImport | null;
  buildings: BuildingTariffState[];
  onUpload: (file: File) => void;
  onApply: (confirmations: IcmalConfirmation[]) => void;
  failure?: 'unreadable';
  uploading?: boolean;
  applying?: boolean;
};

/**
 * The icmal import of 01 §7.11 as three steps — upload, review, confirm.
 * 02 §8.4 is the reason for the third: a derived coefficient is written only
 * after the operator confirms the building it belongs to (R245).
 */
export function IcmalTabView({
  result, buildings, onUpload, onApply, failure, uploading = false, applying = false,
}: IcmalTabViewProps) {
  const t = useTranslations('tariffs');
  const [confirmed, setConfirmed] = useState<string[]>([]);
  const [effectiveFrom, setEffectiveFrom] = useState('');

  const steps = [
    { id: 'upload', label: t('icmal.steps.upload') },
    { id: 'review', label: t('icmal.steps.review') },
    { id: 'confirm', label: t('icmal.steps.confirm') },
  ];
  const currentIndex = result === null ? 0 : confirmed.length === 0 ? 1 : 2;
  const buildingName = (id?: string) => buildings.find((b) => b.building_id === id)?.building_name ?? id ?? '—';

  const submit = () => {
    if (!result || confirmed.length === 0) return;
    onApply(
      result.analyses
        .filter((a) => a.building_id && confirmed.includes(a.etso_code))
        .map((a) => ({ building_id: a.building_id as string, etso_code: a.etso_code, effective_from: effectiveFrom })),
    );
  };

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="type-h3">{t('icmal.title')}</h3>
        <p className="text-foreground-muted type-caption">{t('icmal.description')}</p>
      </div>

      <Stepper steps={steps} currentIndex={currentIndex} />

      {failure ? <Alert tone="danger" title={t('icmal.unreadable')} /> : null}

      <FileUpload
        label={t('icmal.file')}
        accept=".csv,.xlsx,.xls"
        maxSizeBytes={MAX_UPLOAD_BYTES}
        disabled={uploading}
        onFilesSelected={(files) => files[0] && onUpload(files[0])}
      />

      {result ? (
        <>
          {result.unmatched.length > 0 ? (
            <Card className="flex flex-col gap-2">
              <h4 className="type-h3">{t('icmal.unmatched')}</h4>
              <p className="text-foreground-muted type-caption">{t('icmal.unmatchedDescription')}</p>
              <div className="flex flex-wrap gap-2">
                {result.unmatched.map((etso) => (
                  <Badge key={etso} tone="warning">{etso}</Badge>
                ))}
              </div>
            </Card>
          ) : null}

          <div className="flex flex-col gap-4">
            {result.analyses.map((analysis) => (
              <AnalysisCard
                key={analysis.etso_code}
                analysis={analysis}
                buildingName={buildingName(analysis.building_id)}
                checked={confirmed.includes(analysis.etso_code)}
                onCheckedChange={(checked) =>
                  setConfirmed((prev) =>
                    checked ? [...prev, analysis.etso_code] : prev.filter((code) => code !== analysis.etso_code),
                  )
                }
              />
            ))}
          </div>

          <Card className="flex flex-col gap-4">
            <p className="text-foreground type-body">{t('icmal.confirmDescription')}</p>
            <Input
              label={t('icmal.effectiveFrom')}
              required
              type="date"
              value={effectiveFrom}
              onChange={(e) => setEffectiveFrom(e.target.value)}
            />
            <div>
              <Button disabled={confirmed.length === 0 || effectiveFrom === ''} loading={applying} onClick={submit}>
                {t('icmal.confirm')}
              </Button>
            </div>
          </Card>
        </>
      ) : null}
    </div>
  );
}

function AnalysisCard({
  analysis, buildingName, checked, onCheckedChange,
}: {
  analysis: IcmalAnalysis;
  buildingName: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const t = useTranslations('tariffs');
  const rows: { label: string; value: IcmalCoefficient }[] = [
    { label: t('icmal.energyKbk'), value: analysis.energy_kbk },
    { label: t('icmal.distribution'), value: analysis.distribution_tl_per_kwh },
    { label: t('icmal.powerPrice'), value: analysis.power_unit_price },
    { label: t('icmal.reactivePrice'), value: analysis.reactive_unit_price },
    { label: t('icmal.vatRate'), value: analysis.vat_rate },
  ];

  return (
    <Card className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Checkbox
          label={`${analysis.etso_code} — ${buildingName}`}
          // A derivation that matched no building has nothing to write to.
          disabled={!analysis.building_id}
          checked={checked}
          onCheckedChange={onCheckedChange}
        />
        <Badge tone={analysis.within_tolerance ? 'success' : 'warning'}>
          {analysis.within_tolerance ? t('icmal.withinTolerance') : t('icmal.outOfTolerance')}
        </Badge>
      </div>

      <TableContainer label={analysis.etso_code}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('icmal.coefficient')}</TableHead>
              <TableHead>{t('icmal.value')}</TableHead>
              <TableHead>{t('columns.effectiveFrom')}</TableHead>
              <TableHead>{t('icmal.backCalcError')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.label}>
                <TableCell>{row.label}</TableCell>
                <TableCell className="type-data">{row.value.value ?? '—'}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-foreground-muted type-caption">
                      {t('icmal.samples', { count: row.value.samples })}
                    </span>
                    <Badge tone={row.value.stable === true ? 'success' : row.value.stable === false ? 'warning' : 'neutral'}>
                      {row.value.stable === true
                        ? t('icmal.stable')
                        : row.value.stable === false
                          ? t('icmal.unstable')
                          : t('icmal.unknownStability')}
                    </Badge>
                  </div>
                </TableCell>
                <TableCell className="type-data">{row.value.back_calc_error_pct ?? '—'}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      {analysis.warnings.length > 0 ? (
        <div className="flex flex-col gap-1">
          <p className="text-foreground type-body">{t('icmal.warnings')}</p>
          {analysis.warnings.map((warning, i) => (
            <p key={i} role="status" className="text-foreground-muted type-caption">{warning.text}</p>
          ))}
        </div>
      ) : null}
    </Card>
  );
}
