'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { formatNumber, unitSymbol, type Unit } from '@/lib/format';
import { cn } from '@/lib/cn';
import { compareDecimal } from '@/lib/decimal';

import { controlClasses, Field, type FieldProps } from './field';

export type NumberInputProps = FieldProps & {
  value: string | null;
  onValueChange: (value: string | null) => void;
  min?: string;
  max?: string;
  fractionDigits?: number;
  unit?: Unit;
  placeholder?: string;
};

const ALLOWED = /^-?[\d.,]*$/;

/**
 * `1.234,56` → `'1234.56'` as a string, never through a float (plan D9, D11). Undefined when not a number.
 * Without a comma, a single dot not followed by exactly three digits is a decimal point
 * (`41.0082`, `12.5`); `1.234` stays Turkish thousands. Coordinates typed with a dot were
 * silently multiplied (41.0082 → 410082).
 */
export function parseTurkishDecimal(text: string): string | null | undefined {
  const trimmed = text.replace(/\s/g, '');
  if (trimmed === '') return null;
  const dotIsDecimal = !trimmed.includes(',') && /^-?\d*\.\d+$/.test(trimmed) && !/\.\d{3}$/.test(trimmed);
  const normalised = dotIsDecimal ? trimmed : trimmed.replace(/\./g, '').replace(',', '.');
  return /^-?\d+(\.\d+)?$/.test(normalised) ? normalised : undefined;
}

type Problem = 'invalid' | 'integer' | 'outOfRange';

const display = (value: string | null, fractionDigits?: number) =>
  value === null || value === ''
    ? ''
    : formatNumber(value, fractionDigits === undefined ? {} : { minFractionDigits: fractionDigits, maxFractionDigits: fractionDigits });

export function NumberInput({ label, description, error, required, id, disabled, value, onValueChange, min, max, fractionDigits, unit, placeholder }: NumberInputProps) {
  const [text, setText] = useState(() => display(value, fractionDigits));
  const [focused, setFocused] = useState(false);
  const [problem, setProblem] = useState<Problem | null>(null);
  const forms = useTranslations('forms');
  const shown = focused || problem ? text : display(value, fractionDigits);
  const shownError = error ?? (problem ? forms(`errors.${problem}`) : undefined);
  return (
    <Field label={label} description={description} error={shownError} required={required} id={id} disabled={disabled}>
      {({ controlId, describedBy, invalid }) => (
        <div className="relative">
          <input
            id={controlId}
            type="text"
            inputMode="decimal"
            autoComplete="off"
            aria-describedby={describedBy}
            aria-invalid={invalid || undefined}
            required={required}
            disabled={disabled}
            placeholder={placeholder}
            value={shown}
            onFocus={() => {
              setText(display(value, fractionDigits));
              setFocused(true);
            }}
            onChange={(e) => {
              if (ALLOWED.test(e.target.value)) setText(e.target.value);
            }}
            onBlur={() => {
              setFocused(false);
              const parsed = parseTurkishDecimal(text);
              // Bad input is kept and named for correction, never silently dropped or clamped.
              const next: Problem | null =
                parsed === undefined
                  ? 'invalid'
                  : parsed !== null && fractionDigits === 0 && parsed.includes('.')
                    ? 'integer'
                    : parsed !== null && ((min !== undefined && compareDecimal(parsed, min) < 0) || (max !== undefined && compareDecimal(parsed, max) > 0))
                      ? 'outOfRange'
                      : null;
              setProblem(next);
              if (!next && parsed !== undefined && parsed !== value) onValueChange(parsed);
            }}
            className={cn(controlClasses, 'text-end type-data', unit && 'pe-14')}
          />
          {unit ? (
            <span className="pointer-events-none absolute inset-y-0 end-3 flex items-center text-foreground-muted type-small">
              {unitSymbol(unit)}
            </span>
          ) : null}
        </div>
      )}
    </Field>
  );
}
