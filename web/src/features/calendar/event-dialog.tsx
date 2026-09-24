'use client';

import { useTranslations } from 'next-intl';

import { Button } from '@/components/ui/button';
import { DatePicker } from '@/components/ui/date-picker';
import { Dialog } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { RadioGroup } from '@/components/ui/radio-group';
import { Switch } from '@/components/ui/switch';
import { EVENT_COLOURS } from '@/styles/event-palette';

import { eventRangeValid, type EventDraft } from './event-draft';

export type EventDialogViewProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: EventDraft | null;
  onChange: (draft: EventDraft) => void;
  onSubmit: () => void;
  onDelete?: () => void;
  readOnly?: boolean;
  saving?: boolean;
  fieldErrors?: Record<string, string>;
};

/** Event create and edit (01 §7.18): title, all-day, range and colour. */
export function EventDialogView({
  open,
  onOpenChange,
  value,
  onChange,
  onSubmit,
  onDelete,
  readOnly = false,
  saving = false,
  fieldErrors = {},
}: EventDialogViewProps) {
  const t = useTranslations('calendar');
  const common = useTranslations('common');
  const colours = useTranslations('calendar.colours');
  const valid = value ? eventRangeValid(value) : false;
  const set = (patch: Partial<EventDraft>) => value && onChange({ ...value, ...patch });

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={value?.id ? t('editEvent') : t('addEvent')}
      footer={
        readOnly ? undefined : (
          <div className="flex flex-wrap gap-2">
            {value?.id && onDelete ? (
              <Button variant="danger" onClick={onDelete}>
                {t('deleteEvent')}
              </Button>
            ) : null}
            <Button loading={saving} disabled={!valid || !value?.title} onClick={onSubmit}>
              {common('save')}
            </Button>
          </div>
        )
      }
    >
      {value ? (
        <div className="flex flex-col gap-4">
          <Input
            label={t('eventTitle')}
            value={value.title}
            disabled={readOnly}
            error={fieldErrors.title}
            onChange={(e) => set({ title: e.target.value })}
            required
          />
          <Switch label={t('allDay')} checked={value.allDay} disabled={readOnly} onCheckedChange={(allDay) => set({ allDay })} />
          <div className="flex flex-wrap gap-4">
            <DatePicker label={t('start')} value={value.startDate} disabled={readOnly} onValueChange={(date) => set({ startDate: date ?? value.startDate })} />
            {!value.allDay ? (
              <Input label={t('startTime')} type="time" value={value.startTime} disabled={readOnly} onChange={(e) => set({ startTime: e.target.value })} />
            ) : null}
          </div>
          <div className="flex flex-wrap gap-4">
            <DatePicker label={t('end')} value={value.endDate} disabled={readOnly} onValueChange={(date) => set({ endDate: date ?? value.endDate })} />
            {!value.allDay ? (
              <Input label={t('endTime')} type="time" value={value.endTime} disabled={readOnly} onChange={(e) => set({ endTime: e.target.value })} />
            ) : null}
          </div>
          {!valid ? <p className="text-danger type-small">{t('rangeInvalid')}</p> : null}
          <RadioGroup
            label={t('colour')}
            orientation="horizontal"
            options={EVENT_COLOURS.map((colour) => ({ value: colour.hex, label: colours(colour.id) }))}
            value={value.colour}
            disabled={readOnly}
            onValueChange={(colour) => set({ colour })}
          />
        </div>
      ) : null}
    </Dialog>
  );
}
