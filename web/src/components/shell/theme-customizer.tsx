'use client';

import { RotateCcw } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { formatNumber } from '@/lib/format';
import { defaultUiPreferences, type UiPreferences } from '@/lib/ui-preferences';
import { useUiPreferences } from '@/lib/ui-preferences-provider';

import { Button } from '../ui/button';
import { Drawer } from '../ui/drawer';
import { RadioGroup } from '../ui/radio-group';
import { Slider } from '../ui/slider';
import { Switch } from '../ui/switch';

/** 01 §6 customiser minus RTL and theme colour (plan D15, Q1/Q2). */
export function ThemeCustomizer({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const t = useTranslations('shell');
  const { prefs, update } = useUiPreferences();
  const choose = <K extends keyof UiPreferences>(key: K) => (value: string) => update({ [key]: value } as Partial<UiPreferences>);
  return (
    <Drawer
      open={open}
      onOpenChange={onOpenChange}
      side="end"
      title={t('customizer.title')}
      footer={
        <Button variant="secondary" iconStart={RotateCcw} onClick={() => update(defaultUiPreferences)}>
          {t('customizer.reset')}
        </Button>
      }
    >
      <div className="flex flex-col gap-6">
        <RadioGroup
          label={t('theme.label')}
          orientation="horizontal"
          value={prefs.theme}
          onValueChange={choose('theme')}
          options={(['light', 'dark', 'system'] as const).map((v) => ({ value: v, label: t(`theme.${v}`) }))}
        />
        <RadioGroup
          label={t('customizer.layout')}
          orientation="horizontal"
          value={prefs.layout}
          onValueChange={choose('layout')}
          options={(['vertical', 'horizontal'] as const).map((v) => ({ value: v, label: t(`customizer.${v}`) }))}
        />
        <RadioGroup
          label={t('customizer.container')}
          orientation="horizontal"
          value={prefs.container}
          onValueChange={choose('container')}
          options={(['full', 'boxed'] as const).map((v) => ({ value: v, label: t(`customizer.${v}`) }))}
        />
        <Switch label={t('customizer.sidebarCollapsed')} checked={prefs.sidebar === 'collapsed'} onCheckedChange={(v) => update({ sidebar: v ? 'collapsed' : 'expanded' })} />
        <RadioGroup
          label={t('customizer.card')}
          orientation="horizontal"
          value={prefs.card}
          onValueChange={choose('card')}
          options={(['border', 'shadow'] as const).map((v) => ({ value: v, label: t(`customizer.${v}`) }))}
        />
        <Slider
          label={t('customizer.radius')}
          min={0.5}
          max={1.5}
          step={0.25}
          value={prefs.radiusScale}
          onValueChange={(v) => update({ radiusScale: v })}
          formatValue={(v) => `${formatNumber(v)}×`}
        />
      </div>
    </Drawer>
  );
}
