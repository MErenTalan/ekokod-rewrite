'use client';

import { Slider as RadixSlider } from 'radix-ui';

import { Field, type FieldProps } from './field';

export type SliderProps = FieldProps & {
  value: number;
  onValueChange: (value: number) => void;
  min: number;
  max: number;
  step: number;
  formatValue?: (value: number) => string;
};

export function Slider({ label, description, error, required, id, disabled, value, onValueChange, min, max, step, formatValue = String }: SliderProps) {
  return (
    <Field as="span" label={label} description={description} error={error} required={required} id={id} disabled={disabled}>
      {({ controlId, labelId, describedBy }) => (
        <div className="flex items-center gap-3">
          <RadixSlider.Root
            id={controlId}
            value={[value]}
            onValueChange={([v]) => onValueChange(v)}
            min={min}
            max={max}
            step={step}
            disabled={disabled}
            className="relative flex h-5 grow touch-none items-center select-none pointer-coarse:h-11 data-[disabled]:opacity-60"
          >
            <RadixSlider.Track className="relative h-1.5 grow rounded-full bg-surface-sunken">
              <RadixSlider.Range className="absolute h-full rounded-full bg-primary" />
            </RadixSlider.Track>
            <RadixSlider.Thumb
              aria-labelledby={labelId}
              aria-describedby={describedBy}
              aria-valuetext={formatValue(value)}
              className="block size-4 rounded-full border-2 border-primary bg-surface shadow-sm transition-colors duration-(--duration-hover) pointer-coarse:size-6"
            />
          </RadixSlider.Root>
          <output htmlFor={controlId} className="min-w-10 text-end text-foreground type-data">
            {formatValue(value)}
          </output>
        </div>
      )}
    </Field>
  );
}
