'use client';

import { useTranslations } from 'next-intl';

import { Alert } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { MultiSelect } from '@/components/ui/multi-select';
import { Select } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import type { Analyzer } from '@/lib/api/types';

import { ALARM_KINDS, typeLabelKey } from './alarm-labels';
import { clearForType, draftErrors, type AlarmDraft, type AlarmKind, type PeriodUnit, type SettingsKey } from './alarm-draft';

export type AlarmDialogViewProps = {
  open: boolean;
  draft: AlarmDraft;
  analyzers: Analyzer[];
  onDraftChange: (draft: AlarmDraft) => void;
  onSubmit: () => void;
  onClose: () => void;
  saving?: boolean;
};

/** One threshold with the period it is measured over (R230's "group"). */
type Group = {
  threshold: SettingsKey;
  value: SettingsKey;
  unit: SettingsKey;
  labelKey: 'fields.inductiveRatio' | 'fields.capacitiveRatio' | 'fields.activeConsumptionMax' | 'fields.activeConsumptionMin';
};

const REACTIVE_FIELDS: Group[] = [
  { threshold: 'inductive_ratio_threshold', value: 'inductive_period_value', unit: 'inductive_period_unit', labelKey: 'fields.inductiveRatio' },
  { threshold: 'capacitive_ratio_threshold', value: 'capacitive_period_value', unit: 'capacitive_period_unit', labelKey: 'fields.capacitiveRatio' },
  { threshold: 'active_consumption_max', value: 'active_consumption_max_period_value', unit: 'active_consumption_max_period_unit', labelKey: 'fields.activeConsumptionMax' },
  { threshold: 'active_consumption_min', value: 'active_consumption_min_period_value', unit: 'active_consumption_min_period_unit', labelKey: 'fields.activeConsumptionMin' },
];

/**
 * The one rule dialog of 01 §7.12, with a type switch rather than four routes
 * (R229). Switching the type clears the abandoned type's fields, so a
 * threshold typed under one type can never be submitted under another.
 */
export function AlarmDialogView({
  open, draft, analyzers, onDraftChange, onSubmit, onClose, saving = false,
}: AlarmDialogViewProps) {
  const t = useTranslations('alarms');
  const common = useTranslations('common');
  const errors = draftErrors(draft);

  const setSetting = (key: SettingsKey, value: string) =>
    onDraftChange({ ...draft, settings: { ...draft.settings, [key]: value } });
  const errorText = (key: string) => {
    const code = errors[key];
    if (!code) return undefined;
    // draftErrors only ever emits these three codes; the map keeps the key
    // literal so next-intl can type-check it.
    const messages = { required: t('errors.required'), invalid: t('errors.invalid'), atLeastOne: t('errors.atLeastOne') };
    return messages[code as keyof typeof messages];
  };

  const unitOptions = [
    { value: 'hours', label: t('fields.hours') },
    { value: 'days', label: t('fields.days') },
  ];

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => !next && onClose()}
      title={draft.id ? t('editAlarm') : t('addNew')}
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>{common('cancel')}</Button>
          <Button loading={saving} disabled={Object.keys(errors).length > 0} onClick={onSubmit}>
            {common('save')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Input
          label={t('name')}
          required
          value={draft.name}
          error={errorText('name')}
          onChange={(e) => onDraftChange({ ...draft, name: e.target.value })}
        />

        <MultiSelect
          label={t('selectAnalyzer')}
          required
          error={errorText('analyzerIds')}
          value={draft.analyzerIds}
          onValueChange={(values) => onDraftChange({ ...draft, analyzerIds: values })}
          options={analyzers.map((a) => ({ value: a.id, label: a.installation_number }))}
          searchPlaceholder={common('search')}
          emptyText={t('analyzersEmpty')}
        />

        <Select
          label={t('type')}
          required
          value={draft.type}
          onValueChange={(value) => onDraftChange(clearForType(draft, value as AlarmKind))}
          options={ALARM_KINDS.map((kind) => ({ value: kind, label: t(typeLabelKey(kind)) }))}
        />

        {draft.type === 'reactive_limit' ? (
          <div className="flex flex-col gap-4">
            {errors.settings ? <Alert tone="warning" title={t('errors.atLeastOne')} /> : null}
            {REACTIVE_FIELDS.map((group) => (
              <div key={group.threshold} className="grid gap-3 sm:grid-cols-3">
                <Input
                  label={t(group.labelKey)}
                  inputMode="decimal"
                  value={draft.settings[group.threshold] ?? ''}
                  onChange={(e) => setSetting(group.threshold, e.target.value)}
                />
                <Input
                  label={t('fields.inLast')}
                  inputMode="numeric"
                  value={draft.settings[group.value] ?? ''}
                  onChange={(e) => setSetting(group.value, e.target.value)}
                />
                <Select
                  label={t('fields.hours')}
                  value={draft.settings[group.unit] ?? 'hours'}
                  onValueChange={(value) => setSetting(group.unit, value)}
                  options={unitOptions}
                />
              </div>
            ))}
          </div>
        ) : null}

        {draft.type === 'data_communication' ? (
          <Input
            label={t('fields.communicationThresholdHours')}
            required
            inputMode="numeric"
            value={draft.settings.communication_threshold_hours ?? ''}
            error={errorText('communication_threshold_hours')}
            onChange={(e) => setSetting('communication_threshold_hours', e.target.value)}
          />
        ) : null}

        {draft.type === 'current_voltage_power' ? (
          <div className="flex flex-col gap-3">
            {errors.settings ? <Alert tone="warning" title={t('errors.atLeastOne')} /> : null}
            <p className="text-foreground-muted type-caption">{t('powerDescription')}</p>
            <div className="grid gap-3 sm:grid-cols-2">
              <Input
                label={t('fields.powerMax')}
                inputMode="decimal"
                value={draft.settings.power_max ?? ''}
                onChange={(e) => setSetting('power_max', e.target.value)}
              />
              <Input
                label={t('fields.powerMin')}
                inputMode="decimal"
                value={draft.settings.power_min ?? ''}
                onChange={(e) => setSetting('power_min', e.target.value)}
              />
            </div>
          </div>
        ) : null}

        {draft.type === 'invoice_increase' ? (
          <Input
            label={t('fields.invoiceThresholdPct')}
            required
            inputMode="decimal"
            description={t('invoiceThresholdDescription')}
            value={draft.settings.invoice_threshold_pct ?? ''}
            error={errorText('invoice_threshold_pct')}
            onChange={(e) => setSetting('invoice_threshold_pct', e.target.value)}
          />
        ) : null}

        <div className="grid gap-3 sm:grid-cols-2">
          <Input
            label={t('notification.frequency')}
            inputMode="numeric"
            value={draft.frequencyValue}
            onChange={(e) => onDraftChange({ ...draft, frequencyValue: e.target.value })}
          />
          <Select
            label={t('fields.inLast')}
            value={draft.frequencyUnit || 'hours'}
            error={errorText('frequencyUnit')}
            onValueChange={(value) => onDraftChange({ ...draft, frequencyUnit: value as PeriodUnit })}
            options={unitOptions}
          />
        </div>

        <fieldset className="flex flex-col gap-3">
          <legend className="text-foreground type-label">{t('notification.channel')}</legend>
          <Checkbox
            label={t('notification.email')}
            checked={draft.channels.email}
            onCheckedChange={(checked) =>
              onDraftChange({ ...draft, channels: { ...draft.channels, email: checked } })}
          />
          {draft.channels.email ? (
            <Input
              label={t('notification.emailRecipients')}
              value={draft.emailTargets}
              error={errorText('emailTargets')}
              onChange={(e) => onDraftChange({ ...draft, emailTargets: e.target.value })}
            />
          ) : null}

          <Checkbox
            label={t('notification.sms')}
            checked={draft.channels.sms}
            onCheckedChange={(checked) =>
              onDraftChange({ ...draft, channels: { ...draft.channels, sms: checked } })}
          />
          {draft.channels.sms ? (
            <div className="flex flex-col gap-2">
              {/* R211/D-5: the numbers are stored so a gateway can be wired
                  later, and the caption says plainly that nothing is sent. */}
              <Alert tone="info" title={t('notification.smsInactive')} />
              <Input
                label={t('notification.smsNumbers')}
                value={draft.smsTargets}
                error={errorText('smsTargets')}
                onChange={(e) => onDraftChange({ ...draft, smsTargets: e.target.value })}
              />
            </div>
          ) : null}
        </fieldset>

        <Switch
          label={draft.isEnabled ? t('active') : t('passive')}
          checked={draft.isEnabled}
          onCheckedChange={(checked) => onDraftChange({ ...draft, isEnabled: checked })}
        />
      </div>
    </Dialog>
  );
}
