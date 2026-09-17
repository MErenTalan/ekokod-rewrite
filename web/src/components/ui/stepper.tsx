'use client';

import { Check } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { cn } from '@/lib/cn';

export type StepperProps = { steps: { id: string; label: string; description?: string }[]; currentIndex: number };

export function Stepper({ steps, currentIndex }: StepperProps) {
  const t = useTranslations('feedback');
  return (
    <div className="flex flex-col gap-2">
      <p className="text-foreground-muted type-caption">{t('stepOf', { current: currentIndex + 1, total: steps.length })}</p>
      <ol className="flex flex-col gap-3 sm:flex-row sm:gap-6">
        {steps.map((step, i) => {
          const done = i < currentIndex;
          const current = i === currentIndex;
          return (
            <li key={step.id} aria-current={current ? 'step' : undefined} className="flex min-w-0 items-start gap-2">
              <span
                className={cn(
                  'flex size-6 shrink-0 items-center justify-center rounded-full border type-caption',
                  done && 'border-primary bg-primary text-on-primary',
                  current && 'border-primary text-primary',
                  !done && !current && 'border-border-control text-foreground-muted',
                )}
              >
                {done ? <Check aria-hidden className="size-3.5" /> : i + 1}
              </span>
              <span className="flex min-w-0 flex-col">
                <span className={cn('type-small font-semibold', current ? 'text-foreground' : 'text-foreground-muted')}>{step.label}</span>
                {step.description ? <span className="text-foreground-muted type-caption">{step.description}</span> : null}
                {done ? <span className="sr-only">{t('stepCompleted')}</span> : null}
              </span>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
